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
	Kind string    // day | week | month
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

// ParsePeriod: semana de segunda a domingo; mês civil. Datas futuras são recusadas.
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
	}
	return Period{Kind: p.Kind, From: p.From.AddDate(0, 0, -1), To: p.To.AddDate(0, 0, -1)}
}

// Moment é o par StudyDate/StudyTime de um estudo, sem identificadores.
type Moment struct{ Date, Time string }

type Bucket struct {
	Label string `json:"label"` // YYYY-MM-DD, ou HH no período diário
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

// Aggregate conta por dia (semana/mês) ou por hora (dia). Estudos fora do
// período ou com data inválida são ignorados; horários inválidos só não entram
// na distribuição por hora.
func Aggregate(p Period, moments []Moment) Exams {
	e := Exams{Buckets: []Bucket{}, ByHour: make([]int, 24)}
	index := map[string]int{}
	if p.Kind == "day" {
		for h := 0; h < 24; h++ {
			e.Buckets = append(e.Buckets, Bucket{Label: strconv.Itoa(100 + h)[1:]})
		}
	} else {
		for d := p.From; !d.After(p.To); d = d.AddDate(0, 0, 1) {
			index[d.Format("20060102")] = len(e.Buckets)
			e.Buckets = append(e.Buckets, Bucket{Label: d.Format("2006-01-02")})
		}
	}
	from, to := p.From.Format("20060102"), p.To.Format("20060102")
	for _, m := range moments {
		if len(m.Date) != 8 || m.Date < from || m.Date > to {
			continue
		}
		if _, err := time.Parse("20060102", m.Date); err != nil {
			continue
		}
		e.Total++
		h, ok := hour(m.Time)
		if ok {
			e.ByHour[h]++
		} else {
			e.WithoutTime++
		}
		if p.Kind == "day" {
			if ok {
				e.Buckets[h].Count++
			}
		} else {
			e.Buckets[index[m.Date]].Count++
		}
	}
	return e
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

// Archive vem de GET /statistics do Orthanc: totais do acervo, não do período.
type Archive struct {
	Studies   int64 `json:"studies"`
	Series    int64 `json:"series"`
	Instances int64 `json:"instances"`
	DiskMB    int64 `json:"diskMB"`
}
