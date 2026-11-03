// Package stats agrega contagens operacionais por período. Nunca recebe nem
// devolve dados de paciente: somente datas/horários DICOM e totais.
package stats

import (
	"errors"
	"strconv"
	"time"
)

var ErrInvalid = errors.New("Período de estatísticas inválido.")

// MaxStudies limita os estudos lidos do Orthanc por período (sentinela +1).
const MaxStudies = 10000

// Period usa datas civis de São Paulo, inclusivas, representadas em UTC 00:00.
type Period struct {
	Kind string    // day | week | month | year
	From time.Time // primeiro dia
	To   time.Time // último dia, inclusive
}

func civil(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// Today devolve a data civil corrente em São Paulo.
func Today(now time.Time) time.Time {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*3600)
	}
	return civil(now.In(loc))
}

// ParsePeriod: semana de segunda a domingo; mês e ano civis. Datas futuras são recusadas.
func ParsePeriod(kind, date string, now time.Time) (Period, error) {
	today := Today(now)
	ref := today
	if date != "" {
		d, err := time.Parse("2006-01-02", date)
		if err != nil || d.Year() < 1900 || d.After(today) {
			return Period{}, ErrInvalid
		}
		ref = d
	}
	switch kind {
	case "day":
		return Period{Kind: kind, From: ref, To: ref}, nil
	case "week":
		from := ref.AddDate(0, 0, -((int(ref.Weekday()) + 6) % 7))
		return Period{Kind: kind, From: from, To: from.AddDate(0, 0, 6)}, nil
	case "month":
		from := time.Date(ref.Year(), ref.Month(), 1, 0, 0, 0, 0, time.UTC)
		return Period{Kind: kind, From: from, To: from.AddDate(0, 1, -1)}, nil
	case "year":
		from := time.Date(ref.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return Period{Kind: kind, From: from, To: from.AddDate(1, 0, -1)}, nil
	}
	return Period{}, ErrInvalid
}

// Previous é o período imediatamente anterior, do mesmo tipo.
func (p Period) Previous() Period {
	switch p.Kind {
	case "week":
		return Period{Kind: p.Kind, From: p.From.AddDate(0, 0, -7), To: p.To.AddDate(0, 0, -7)}
	case "month":
		from := p.From.AddDate(0, -1, 0)
		return Period{Kind: p.Kind, From: from, To: from.AddDate(0, 1, -1)}
	case "year":
		from := p.From.AddDate(-1, 0, 0)
		return Period{Kind: p.Kind, From: from, To: from.AddDate(1, 0, -1)}
	}
	return Period{Kind: p.Kind, From: p.From.AddDate(0, 0, -1), To: p.To.AddDate(0, 0, -1)}
}

// Moment é o par StudyDate/StudyTime de um estudo, sem identificadores.
type Moment struct{ Date, Time string }

type Bucket struct {
	Label string `json:"label"` // HH (dia), YYYY-MM-DD (semana/mês) ou YYYY-MM (ano)
	Count int    `json:"count"`
}

type Exams struct {
	Total         int      `json:"total"`
	PreviousTotal int      `json:"previousTotal"`
	Truncated     bool     `json:"truncated"`
	WithoutTime   int      `json:"withoutTime"`
	Buckets       []Bucket `json:"buckets"`
	ByHour        []int    `json:"byHour"`
}

func hour(t string) (int, bool) {
	if len(t) < 2 {
		return 0, false
	}
	h, err := strconv.Atoi(t[:2])
	return h, err == nil && h >= 0 && h <= 23
}

// Daily é o resumo de um dia civil: base da tabela stats_daily e da agregação.
type Daily struct {
	Day         time.Time
	Exams       int
	WithoutTime int
	ByHour      [24]int
	Truncated   bool
}

// Days resume os momentos por dia, incluindo dias sem exames, de from a to.
// Datas fora do intervalo ou inválidas são ignoradas; horários inválidos contam
// no total e em WithoutTime.
func Days(from, to time.Time, moments []Moment) []Daily {
	days := []Daily{}
	index := map[string]int{}
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		index[d.Format("20060102")] = len(days)
		days = append(days, Daily{Day: d})
	}
	for _, m := range moments {
		i, ok := index[m.Date]
		if !ok {
			continue
		}
		days[i].Exams++
		if h, ok := hour(m.Time); ok {
			days[i].ByHour[h]++
		} else {
			days[i].WithoutTime++
		}
	}
	return days
}

// Combine monta as séries do período a partir de resumos diários: por hora
// (dia), por dia (semana/mês) ou por mês (ano). Dias fora do período são
// ignorados; dias ausentes contam zero.
func Combine(p Period, days []Daily) Exams {
	e := Exams{Buckets: []Bucket{}, ByHour: make([]int, 24)}
	index := map[string]int{}
	key := func(d time.Time) string { return d.Format("2006-01-02") }
	switch p.Kind {
	case "day":
		for h := 0; h < 24; h++ {
			e.Buckets = append(e.Buckets, Bucket{Label: strconv.Itoa(100 + h)[1:]})
		}
	case "year":
		key = func(d time.Time) string { return d.Format("2006-01") }
		for d := p.From; !d.After(p.To); d = d.AddDate(0, 1, 0) {
			index[key(d)] = len(e.Buckets)
			e.Buckets = append(e.Buckets, Bucket{Label: key(d)})
		}
	default:
		for d := p.From; !d.After(p.To); d = d.AddDate(0, 0, 1) {
			index[key(d)] = len(e.Buckets)
			e.Buckets = append(e.Buckets, Bucket{Label: key(d)})
		}
	}
	for _, d := range days {
		if d.Day.Before(p.From) || d.Day.After(p.To) {
			continue
		}
		e.Total += d.Exams
		e.WithoutTime += d.WithoutTime
		e.Truncated = e.Truncated || d.Truncated
		for h, n := range d.ByHour {
			e.ByHour[h] += n
			if p.Kind == "day" {
				e.Buckets[h].Count += n
			}
		}
		if p.Kind != "day" {
			e.Buckets[index[key(d.Day)]].Count += d.Exams
		}
	}
	return e
}

// Aggregate conta os momentos do período (ver Days e Combine).
func Aggregate(p Period, moments []Moment) Exams {
	return Combine(p, Days(p.From, p.To, moments))
}

// Usage resume eventos de auditoria no período; nada identifica usuários.
type Usage struct {
	Logins              int `json:"logins"`
	LoginFailures       int `json:"loginFailures"`
	ActiveUsers         int `json:"activeUsers"`
	Exports             int `json:"exports"`
	PreviousLogins      int `json:"previousLogins"`
	PreviousActiveUsers int `json:"previousActiveUsers"`
	PreviousExports     int `json:"previousExports"`
}

// UnitAccess conta logins de usuários vinculados à unidade. Quem pertence a
// mais de uma unidade conta em cada uma.
type UnitAccess struct {
	Name        string `json:"name"`
	ActiveUsers int    `json:"activeUsers"`
	Logins      int    `json:"logins"`
}

// Archive vem de GET /statistics do Orthanc: totais do acervo, não do período.
type Archive struct {
	Studies   int64 `json:"studies"`
	Series    int64 `json:"series"`
	Instances int64 `json:"instances"`
	DiskMB    int64 `json:"diskMB"`
}

// ArchivePoint é o último retrato do acervo registrado no mês.
type ArchivePoint struct {
	Month   string `json:"month"` // YYYY-MM
	Studies int64  `json:"studies"`
	DiskMB  int64  `json:"diskMB"`
}
