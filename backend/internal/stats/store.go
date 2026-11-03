package stats

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrStore encobre falhas do banco; detalhes não saem para a API.
var ErrStore = errors.New("estatísticas armazenadas indisponíveis")

// Store persiste resumos diários (stats_daily) e retratos do acervo
// (stats_archive_daily). Só contagens: nada identifica estudo ou pessoa.
type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Days devolve os dias já calculados no intervalo, em ordem.
func (s *Store) Days(ctx context.Context, from, to time.Time) ([]Daily, error) {
	rows, err := s.pool.Query(ctx, `SELECT day, exams, without_time, by_hour, truncated FROM stats_daily WHERE day BETWEEN $1 AND $2 ORDER BY day`, from, to)
	if err != nil {
		return nil, ErrStore
	}
	defer rows.Close()
	days := []Daily{}
	for rows.Next() {
		var d Daily
		var byHour []int32
		if rows.Scan(&d.Day, &d.Exams, &d.WithoutTime, &byHour, &d.Truncated) != nil || len(byHour) != 24 {
			return nil, ErrStore
		}
		for h, n := range byHour {
			d.ByHour[h] = int(n)
		}
		d.Day = civil(d.Day)
		days = append(days, d)
	}
	if rows.Err() != nil {
		return nil, ErrStore
	}
	return days, nil
}

// SaveDays grava (ou recalcula) os dias numa única transação.
func (s *Store) SaveDays(ctx context.Context, days []Daily) error {
	if len(days) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, d := range days {
		batch.Queue(`INSERT INTO stats_daily(day, exams, without_time, by_hour, truncated, computed_at) VALUES($1,$2,$3,$4,$5,now())
ON CONFLICT (day) DO UPDATE SET exams=EXCLUDED.exams, without_time=EXCLUDED.without_time, by_hour=EXCLUDED.by_hour, truncated=EXCLUDED.truncated, computed_at=now()`,
			d.Day, d.Exams, d.WithoutTime, d.ByHour[:], d.Truncated)
	}
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error { return tx.SendBatch(ctx, batch).Close() })
	if err != nil {
		return ErrStore
	}
	return nil
}

// MissingDays lista, do mais recente ao mais antigo, os dias do intervalo
// ainda não calculados, até limit.
func (s *Store) MissingDays(ctx context.Context, from, to time.Time, limit int) ([]time.Time, error) {
	rows, err := s.pool.Query(ctx, `SELECT d::date FROM generate_series($1::date, $2::date, interval '1 day') d
WHERE NOT EXISTS (SELECT 1 FROM stats_daily s WHERE s.day = d::date) ORDER BY d DESC LIMIT $3`, from, to, limit)
	if err != nil {
		return nil, ErrStore
	}
	defer rows.Close()
	days := []time.Time{}
	for rows.Next() {
		var d time.Time
		if rows.Scan(&d) != nil {
			return nil, ErrStore
		}
		days = append(days, civil(d))
	}
	if rows.Err() != nil {
		return nil, ErrStore
	}
	return days, nil
}

// SaveArchive registra o retrato do acervo do dia; o último do dia prevalece.
func (s *Store) SaveArchive(ctx context.Context, day time.Time, a Archive) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO stats_archive_daily(day, studies, series, instances, disk_mb, captured_at) VALUES($1,$2,$3,$4,$5,now())
ON CONFLICT (day) DO UPDATE SET studies=EXCLUDED.studies, series=EXCLUDED.series, instances=EXCLUDED.instances, disk_mb=EXCLUDED.disk_mb, captured_at=now()`,
		day, a.Studies, a.Series, a.Instances, a.DiskMB)
	if err != nil {
		return ErrStore
	}
	return nil
}

// ArchiveHistory devolve o último retrato de cada mês no intervalo.
func (s *Store) ArchiveHistory(ctx context.Context, from, to time.Time) ([]ArchivePoint, error) {
	rows, err := s.pool.Query(ctx, `SELECT DISTINCT ON (date_trunc('month', day)) to_char(day, 'YYYY-MM'), studies, disk_mb
FROM stats_archive_daily WHERE day BETWEEN $1 AND $2 ORDER BY date_trunc('month', day), day DESC`, from, to)
	if err != nil {
		return nil, ErrStore
	}
	defer rows.Close()
	points := []ArchivePoint{}
	for rows.Next() {
		var p ArchivePoint
		if rows.Scan(&p.Month, &p.Studies, &p.DiskMB) != nil {
			return nil, ErrStore
		}
		points = append(points, p)
	}
	if rows.Err() != nil {
		return nil, ErrStore
	}
	return points, nil
}
