package audit

import (
	"context"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

// Usage conta logins, usuários ativos, falhas e exportações no período e no
// anterior, com limites de dia civil de São Paulo. Devolve somente agregados.
func (r *Recorder) Usage(ctx context.Context, current, previous stats.Period) (stats.Usage, error) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return stats.Usage{}, ErrUnavailable
	}
	from, until := localDayStart(current.From, loc), localDayStart(current.To.AddDate(0, 0, 1), loc)
	prevFrom, prevUntil := localDayStart(previous.From, loc), localDayStart(previous.To.AddDate(0, 0, 1), loc)
	var u stats.Usage
	err = r.pool.QueryRow(ctx, `SELECT
 count(*) FILTER (WHERE cur AND event=$5),
 count(*) FILTER (WHERE cur AND event=$6),
 count(DISTINCT actor_user_id) FILTER (WHERE cur AND event=$5),
 count(*) FILTER (WHERE cur AND event=$7),
 count(*) FILTER (WHERE NOT cur AND event=$5),
 count(DISTINCT actor_user_id) FILTER (WHERE NOT cur AND event=$5),
 count(*) FILTER (WHERE NOT cur AND event=$7)
 FROM (SELECT event,actor_user_id,occurred_at>=$1 AND occurred_at<$2 AS cur FROM audit_events
  WHERE ((occurred_at>=$1 AND occurred_at<$2) OR (occurred_at>=$3 AND occurred_at<$4)) AND event IN ($5,$6,$7)) e`,
		from, until, prevFrom, prevUntil, string(EventLoginSuccess), string(EventLoginFailure), string(EventViewerImageExported)).
		Scan(&u.Logins, &u.LoginFailures, &u.ActiveUsers, &u.Exports, &u.PreviousLogins, &u.PreviousActiveUsers, &u.PreviousExports)
	if err != nil {
		return stats.Usage{}, ErrUnavailable
	}
	return u, nil
}

// UnitAccess conta logins e usuários ativos por unidade vinculada ao usuário no
// período. Usuário com várias unidades conta em cada uma; sem vínculo, em nenhuma.
func (r *Recorder) UnitAccess(ctx context.Context, p stats.Period) ([]stats.UnitAccess, error) {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return nil, ErrUnavailable
	}
	rows, err := r.pool.Query(ctx, `SELECT u.name, count(DISTINCT e.actor_user_id), count(*)
 FROM audit_events e JOIN user_units uu ON uu.user_id=e.actor_user_id JOIN units u ON u.id=uu.unit_id
 WHERE e.event=$3 AND e.occurred_at>=$1 AND e.occurred_at<$2
 GROUP BY u.id, u.name ORDER BY count(*) DESC, u.name LIMIT 100`,
		localDayStart(p.From, loc), localDayStart(p.To.AddDate(0, 0, 1), loc), string(EventLoginSuccess))
	if err != nil {
		return nil, ErrUnavailable
	}
	defer rows.Close()
	list := []stats.UnitAccess{}
	for rows.Next() {
		var a stats.UnitAccess
		if rows.Scan(&a.Name, &a.ActiveUsers, &a.Logins) != nil {
			return nil, ErrUnavailable
		}
		list = append(list, a)
	}
	if rows.Err() != nil {
		return nil, ErrUnavailable
	}
	return list, nil
}
