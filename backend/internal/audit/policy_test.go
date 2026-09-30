package audit

import (
	"context"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"strings"
	"testing"
)

type capture struct {
	args  []any
	calls int
}

func (c *capture) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	c.args = args
	c.calls++
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func TestPolicyClosedVocabulary(t *testing.T) {
	for _, event := range Events {
		for _, detail := range []string{`{"password":"synthetic"}`, "password_hash=$argon2id$synthetic", "session_id=synthetic", "token=synthetic", "PatientName=synthetic", "PatientID=synthetic", "AccessionNumber=synthetic", "StudyInstanceUID=synthetic", "Authorization=synthetic", "PACS_MASTER_KEY=synthetic", "target_user_id=not-a-uuid"} {
			if SafeDetail(event, detail) != "" {
				t.Fatal("arbitrary detail accepted", event)
			}
			c := &capture{}
			if insert(context.Background(), c, Entry{Event: event, Detail: detail}) == nil || c.calls != 0 {
				t.Fatal("unsafe insert")
			}
		}
	}
	for _, event := range Events {
		if !Known(event) || Category(event) == "unknown" {
			t.Fatal("inventory")
		}
	}
	if len(Events) != 18 {
		t.Fatal("inventory changed")
	}
}
func TestAnonymousAttemptAndOriginNeverPersistArbitraryText(t *testing.T) {
	c := &capture{}
	err := insert(context.Background(), c, Entry{Event: EventLoginFailure, ActorUsername: "synthetic-secret-pasted", Origin: "session-token", Detail: "usuário inexistente"})
	if err != nil || c.calls != 1 || c.args[2].(*string) != nil || c.args[5].(*string) != nil {
		t.Fatal("untrusted data persisted")
	}
	id := uuid.New()
	c = &capture{}
	err = insert(context.Background(), c, Entry{Event: EventUserCreated, ActorUserID: &id, ActorUsername: "synthetic_admin", Detail: "target_user_id=" + id.String(), Origin: "127.0.0.1"})
	if err != nil || c.calls != 1 {
		t.Fatal("valid event rejected")
	}
}
func TestParseQuery(t *testing.T) {
	id := uuid.New().String()
	q, err := ParseQuery("dateFrom=2026-09-01&dateTo=2026-09-29&actorId=" + id + "&targetUserId=" + id + "&event=USER_CREATED&category=users&limit=100&offset=50")
	if err != nil || q.Limit != 100 || q.Offset != 50 || q.From.Format("2006-01-02T15:04:05Z07:00") != "2026-09-01T00:00:00-03:00" || q.Until.Day() != 30 {
		t.Fatal("valid query", err)
	}
	for _, raw := range []string{"limit=0", "limit=101", "offset=-1", "offset=10001", "limit=1&limit=2", "unknown=x", "event=invalid", "event=USER_CREATED%27%3BDELETE", "actorId=invalid", "targetUserId=invalid", "category=clinical", "dateFrom=2026-02-30", "dateTo=invalid", "dateFrom=2026-09-30&dateTo=2026-09-29", "dateFrom=0001-01-01", "event=", "x=" + strings.Repeat("a", 1100)} {
		if _, e := ParseQuery(raw); e == nil {
			t.Fatal("accepted", raw)
		}
	}
	if q, e := ParseQuery(""); e != nil || q.Limit != 50 {
		t.Fatal("default")
	}
}

func TestHistoricalSaoPauloDayBoundary(t *testing.T) {
	q, e := ParseQuery("dateFrom=2018-11-04&dateTo=2018-11-04")
	if e != nil || q.From.Format("2006-01-02T15:04:05Z07:00") != "2018-11-04T01:00:00-02:00" || q.Until.Format("2006-01-02T15:04:05Z07:00") != "2018-11-05T00:00:00-02:00" {
		t.Fatal("DST civil day boundary", e)
	}
}
