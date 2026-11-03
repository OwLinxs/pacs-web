package stats

import (
	"testing"
	"time"
)

func TestParsePeriodAndPrevious(t *testing.T) {
	now := time.Date(2026, 10, 30, 2, 0, 0, 0, time.UTC) // 29/10 23:00 em São Paulo
	cases := []struct{ kind, date, from, to, prevFrom, prevTo string }{
		{"day", "", "2026-10-29", "2026-10-29", "2026-10-28", "2026-10-28"},
		{"week", "2026-10-29", "2026-10-26", "2026-11-01", "2026-10-19", "2026-10-25"},
		{"week", "2026-10-25", "2026-10-19", "2026-10-25", "2026-10-12", "2026-10-18"},
		{"month", "2026-03-31", "2026-03-01", "2026-03-31", "2026-02-01", "2026-02-28"},
		{"month", "2026-01-15", "2026-01-01", "2026-01-31", "2025-12-01", "2025-12-31"},
		{"year", "", "2026-01-01", "2026-12-31", "2025-01-01", "2025-12-31"},
		{"year", "2024-02-29", "2024-01-01", "2024-12-31", "2023-01-01", "2023-12-31"},
	}
	f := func(x time.Time) string { return x.Format("2006-01-02") }
	for _, c := range cases {
		p, err := ParsePeriod(c.kind, c.date, now)
		if err != nil {
			t.Fatal(c, err)
		}
		prev := p.Previous()
		if f(p.From) != c.from || f(p.To) != c.to || f(prev.From) != c.prevFrom || f(prev.To) != c.prevTo {
			t.Fatalf("%v: %s..%s / %s..%s", c, f(p.From), f(p.To), f(prev.From), f(prev.To))
		}
	}
	for _, bad := range [][2]string{{"quarter", ""}, {"day", "2026-10-30"}, {"day", "29/10/2026"}, {"day", "1800-01-01"}} {
		if _, err := ParsePeriod(bad[0], bad[1], now); err == nil {
			t.Fatalf("aceitou %v", bad)
		}
	}
}

func TestAggregate(t *testing.T) {
	week := Period{Kind: "week", From: time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC), To: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)}
	e := Aggregate(week, []Moment{
		{"20261026", "235959"}, {"20261101", "000000.5"}, {"20261101", "2400"}, // hora inválida
		{"20261025", "100000"}, {"20261102", "100000"}, {"20260231", "100000"}, {"", ""}, // fora/ inválidas
	})
	if e.Total != 3 || e.WithoutTime != 1 || len(e.Buckets) != 7 || e.Buckets[0].Count != 1 || e.Buckets[6].Count != 2 || e.ByHour[23] != 1 || e.ByHour[0] != 1 {
		t.Fatalf("semana: %+v", e)
	}
	day := Period{Kind: "day", From: week.From, To: week.From}
	e = Aggregate(day, []Moment{{"20261026", "0815"}, {"20261026", "08"}, {"20261026", "x"}})
	if e.Total != 3 || len(e.Buckets) != 24 || e.Buckets[8].Count != 2 || e.Buckets[8].Label != "08" || e.WithoutTime != 1 {
		t.Fatalf("dia: %+v", e)
	}
}

func TestDaysAndCombine(t *testing.T) {
	d := func(m time.Month, day int) time.Time { return time.Date(2026, m, day, 0, 0, 0, 0, time.UTC) }
	days := Days(d(10, 30), d(11, 1), []Moment{{"20261030", "0815"}, {"20261101", "x"}, {"20261029", "1000"}})
	if len(days) != 3 || days[0].Exams != 1 || days[0].ByHour[8] != 1 || days[1].Exams != 0 || days[2].WithoutTime != 1 {
		t.Fatalf("dias: %+v", days)
	}
	days[2].Truncated = true
	year := Period{Kind: "year", From: d(1, 1), To: d(12, 31)}
	e := Combine(year, append(days, Daily{Day: time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC), Exams: 9}))
	if e.Total != 2 || len(e.Buckets) != 12 || e.Buckets[9].Label != "2026-10" || e.Buckets[9].Count != 1 || e.Buckets[10].Count != 1 || !e.Truncated || e.WithoutTime != 1 {
		t.Fatalf("ano: %+v", e)
	}
	day := Period{Kind: "day", From: d(10, 30), To: d(10, 30)}
	if e := Combine(day, days); e.Total != 1 || e.Buckets[8].Count != 1 || e.Truncated {
		t.Fatalf("dia: %+v", e)
	}
}
