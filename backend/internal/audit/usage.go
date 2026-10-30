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
