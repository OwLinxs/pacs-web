package audit

import (
	"context"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/google/uuid"
)

type Query struct {
	Limit, Offset     int
	From, Until       *time.Time
	ActorID, TargetID *uuid.UUID
	Event             Event
	Category          string
}

func ParseQuery(raw string) (Query, error) {
	q := Query{Limit: 50}
	if len(raw) > 1024 {
		return q, ErrInvalid
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return q, ErrInvalid
	}
	loc, _ := time.LoadLocation("America/Sao_Paulo")
	for key, vs := range values {
		if len(vs) != 1 || vs[0] == "" {
			return q, ErrInvalid
		}
		v := vs[0]
		switch key {
		case "limit", "offset":
			n, e := strconv.Atoi(v)
			if e != nil {
				return q, ErrInvalid
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		case "dateFrom", "dateTo":
			d, e := time.Parse("2006-01-02", v)
			if e != nil || d.Year() < 1900 || d.Year() > 9998 {
				return q, ErrInvalid
			}
			if key == "dateFrom" {
				start := localDayStart(d, loc)
				q.From = &start
			} else {
				end := localDayStart(d.AddDate(0, 0, 1), loc)
				q.Until = &end
			}
		case "actorId", "targetUserId":
			id, e := uuid.Parse(v)
			if e != nil || id == uuid.Nil || id.String() != v {
				return q, ErrInvalid
			}
			if key == "actorId" {
				q.ActorID = &id
			} else {
				q.TargetID = &id
			}
		case "event":
			q.Event = Event(v)
			if !Known(q.Event) {
				return q, ErrInvalid
			}
		case "category":
			switch v {
			case "users", "units", "auth", "settings":
				q.Category = v
			default:
				return q, ErrInvalid
			}
		default:
			return q, ErrInvalid
		}
	}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 10000 || (q.From != nil && q.Until != nil && !q.From.Before(*q.Until)) {
		return q, ErrInvalid
	}
	return q, nil
}

// Sao Paulo historically skipped midnight at DST start. Resolve each boundary
// independently so an inclusive civil date neither includes the previous day
// nor adds an extra hour to the following day.
func localDayStart(day time.Time, loc *time.Location) time.Time {
	start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, loc)
	wanted := day.Format("2006-01-02")
	for start.Format("2006-01-02") < wanted {
		start = start.Add(time.Minute)
	}
	return start
}

type Reference struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Username string `json:"username"`
	Kind     string `json:"kind"`
}
type Record struct {
	ID         string     `json:"id"`
	OccurredAt time.Time  `json:"occurredAt"`
	Event      Event      `json:"event"`
	Category   string     `json:"category"`
	Actor      *Reference `json:"actor"`
	Target     *Reference `json:"target"`
	Result     string     `json:"result"`
	// Fixed operational text, never raw JSON or arbitrary database detail.
	Detail string `json:"detail"`
	Origin string `json:"origin"`
}
type Page struct {
	Items      []Record `json:"items"`
	Limit      int      `json:"limit"`
	Offset     int      `json:"offset"`
	HasMore    bool     `json:"hasMore"`
	NextOffset *int     `json:"nextOffset"`
}

// LEFT JOINs do not drop events for disabled/missing actors or targets.
// Target cast is guarded before JOIN, so arbitrary legacy detail is never parsed as SQL/UUID.
func (r *Recorder) List(ctx context.Context, q Query) (Page, error) {
	page := Page{Items: []Record{}, Limit: q.Limit, Offset: q.Offset}
	if q.Limit < 1 || q.Limit > 100 || q.Offset < 0 || q.Offset > 10000 {
		return page, ErrInvalid
	}
	events := []string{}
	for _, e := range Events {
		if q.Category == "" || Category(e) == q.Category {
			events = append(events, string(e))
		}
	}
	target := ""
	if q.TargetID != nil {
		target = "target_user_id=" + q.TargetID.String()
	}
	rows, err := r.pool.Query(ctx, `SELECT a.id::text,a.occurred_at,a.event,a.actor_user_id,COALESCE(actor.name,''),COALESCE(actor.username,a.actor_username,''),a.unit_id,COALESCE(un.name,''),COALESCE(a.detail,''),COALESCE(a.origin,''),target.id,COALESCE(target.name,''),COALESCE(target.username,'')
 FROM audit_events a
 LEFT JOIN users actor ON actor.id=a.actor_user_id
 LEFT JOIN units un ON un.id=a.unit_id
 LEFT JOIN users target ON target.id=CASE WHEN a.event=ANY($9::text[]) AND a.detail ~ '^target_user_id=[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$' THEN substring(a.detail from 16)::uuid END
 WHERE ($1::timestamptz IS NULL OR a.occurred_at >=$1) AND ($2::timestamptz IS NULL OR a.occurred_at <$2)
 AND ($3::uuid IS NULL OR a.actor_user_id=$3) AND ($4='' OR a.event=$4)
 AND ($5='' OR (a.detail=$5 AND a.detail LIKE 'target_user_id=%' AND a.event=ANY($9::text[])))
 AND ($6::boolean OR a.event=ANY($7::text[]))
 ORDER BY a.occurred_at DESC,a.id DESC LIMIT $8 OFFSET $10`, q.From, q.Until, q.ActorID, string(q.Event), target, q.Category == "", events, q.Limit+1, userEvents(), q.Offset)
	if err != nil {
		return page, ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var v Record
		var actorID, unitID, targetID *uuid.UUID
		var actorName, actorUsername, unitName, detail, origin, targetName, targetUsername string
		if rows.Scan(&v.ID, &v.OccurredAt, &v.Event, &actorID, &actorName, &actorUsername, &unitID, &unitName, &detail, &origin, &targetID, &targetName, &targetUsername) != nil {
			return page, ErrUnavailable
		}
		v.Category = Category(v.Event)
		v.Result = "unknown"
		if !Known(v.Event) {
			v.Event = "UNKNOWN"
		} else {
			safe := SafeDetail(v.Event, detail)
			v.Detail = safe
			switch {
			case v.Event == EventLoginFailure:
				v.Result = "failure"
			case v.Event == EventOrthancConnectionTested:
				if safe == "PACS/Orthanc: connected" {
					v.Result = "success"
				} else if safe != "" {
					v.Result = "failure"
				}
			default:
				v.Result = "success"
			}
			if v.Category == "users" && safe != "" {
				v.Target = &Reference{ID: strings.TrimPrefix(safe, "target_user_id="), Name: targetName, Username: targetUsername, Kind: "user"}
				v.Detail = "" // Target is structured, not a free-form metadata blob.
			} else if v.Category == "units" && unitID != nil {
				v.Target = &Reference{ID: unitID.String(), Name: unitName, Kind: "unit"}
			}
			if ip := net.ParseIP(origin); ip != nil {
				v.Origin = ip.String()
			}
		}
		if actorID != nil {
			if !usernamePattern.MatchString(actorUsername) {
				actorUsername = ""
			}
			v.Actor = &Reference{ID: actorID.String(), Name: actorName, Username: actorUsername, Kind: "user"}
		}
		page.Items = append(page.Items, v)
	}
	if rows.Err() != nil {
		return page, ErrUnavailable
	}
	if len(page.Items) > q.Limit {
		page.HasMore = true
		page.Items = page.Items[:q.Limit]
		if n := q.Offset + q.Limit; n <= 10000 {
			page.NextOffset = &n
		}
	}
	return page, nil
}
func userEvents() []string {
	out := []string{}
	for _, e := range Events {
		if Category(e) == "users" {
			out = append(out, string(e))
		}
	}
	return out
}
