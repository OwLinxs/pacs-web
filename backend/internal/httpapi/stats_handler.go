package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
	"github.com/pmfb-saude/pacs-web/backend/internal/statsjob"
)

type StatsSource interface {
	StudyMoments(context.Context, orthanc.Config, time.Time, time.Time) ([]stats.Moment, bool, error)
	ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error)
}

type UsageReader interface {
	Usage(context.Context, stats.Period, stats.Period) (stats.Usage, error)
	UnitAccess(context.Context, stats.Period) ([]stats.UnitAccess, error)
}

// StatsStore lê os resumos diários gravados pelo job (stats_daily).
type StatsStore interface {
	Days(ctx context.Context, from, to time.Time) ([]stats.Daily, error)
	ArchiveHistory(ctx context.Context, from, to time.Time) ([]stats.ArchivePoint, error)
}

// Cada seção é opcional: falha no PACS não esconde o uso, e vice-versa.
type statsResponse struct {
	Period         string               `json:"period"`
	From           string               `json:"from"`
	To             string               `json:"to"`
	PreviousFrom   string               `json:"previousFrom"`
	PreviousTo     string               `json:"previousTo"`
	Exams          *stats.Exams         `json:"exams"`
	ExamsError     string               `json:"examsError,omitempty"`
	Archive        *stats.Archive       `json:"archive"`
	ArchiveHistory []stats.ArchivePoint `json:"archiveHistory"`
	Usage          *stats.Usage         `json:"usage"`
	Units          []stats.UnitAccess   `json:"units"`
}

// maxLiveRanges limita as consultas ao vivo por período; acima disso, uma só.
const maxLiveRanges = 3

type dayRange struct{ from, to time.Time }

// liveRanges devolve as sequências de dias do período, até hoje, ainda não
// gravadas. O dia corrente nunca é gravado e sempre vem do PACS.
func liveRanges(p stats.Period, today time.Time, stored map[time.Time]bool) []dayRange {
	var ranges []dayRange
	for d := p.From; !d.After(p.To) && !d.After(today); d = d.AddDate(0, 0, 1) {
		if stored[d] && !d.Equal(today) {
			continue
		}
		if n := len(ranges); n > 0 && ranges[n-1].to.Equal(d.AddDate(0, 0, -1)) {
			ranges[n-1].to = d
		} else {
			ranges = append(ranges, dayRange{d, d})
		}
	}
	if len(ranges) > maxLiveRanges {
		ranges = []dayRange{{ranges[0].from, ranges[len(ranges)-1].to}}
	}
	return ranges
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	kind, date := "day", ""
	for key, list := range values {
		if len(list) != 1 || (key != "period" && key != "date") {
			err = stats.ErrInvalid
		} else if key == "period" {
			kind = list[0]
		} else {
			date = list[0]
		}
	}
	var period stats.Period
	if err == nil && len(r.URL.RawQuery) <= 256 {
		period, err = stats.ParsePeriod(kind, date, s.now())
	} else {
		err = stats.ErrInvalid
	}
	if err != nil {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, stats.ErrInvalid.Error())
		return
	}
	previous := period.Previous()
	today := stats.Today(s.now())
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	out := statsResponse{Period: period.Kind, From: day(period.From), To: day(period.To), PreviousFrom: day(previous.From), PreviousTo: day(previous.To),
		ExamsError: "Estatísticas de exames indisponíveis. Verifique a conexão com o PACS."}

	// Dias já calculados pelo job; os demais vêm do PACS ao vivo.
	var all []stats.Daily
	stored := map[time.Time]bool{}
	if s.statsStore != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		end := period.To
		if end.After(today) {
			end = today
		}
		// O dia corrente nunca está gravado: lê até ontem, no máximo.
		storedTo := period.To
		if yesterday := today.AddDate(0, 0, -1); storedTo.After(yesterday) {
			storedTo = yesterday
		}
		if days, err := s.statsStore.Days(ctx, previous.From, storedTo); err == nil {
			for _, d := range days {
				stored[d.Day] = true
			}
			all = days
		}
		historyFrom := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -11, 0)
		if history, err := s.statsStore.ArchiveHistory(ctx, historyFrom, end); err == nil {
			out.ArchiveHistory = history
		}
		cancel()
	}
	type gap struct {
		from, to time.Time
		days     []stats.Daily
		err      error
	}
	var gaps []*gap
	for _, p := range []stats.Period{period, previous} {
		for _, lr := range liveRanges(p, today, stored) {
			gaps = append(gaps, &gap{from: lr.from, to: lr.to})
		}
	}

	// Configuração do PACS resolvida antes de qualquer goroutine.
	var cfg *orthanc.Config
	if s.stats != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		snapshot, err := s.settings.OrthancConnection(ctx)
		cancel()
		if err == nil {
			if c, ok := statsjob.ConfigFrom(snapshot); ok {
				cfg = &c
				if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(c.Timeout + 10*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
					writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Estatísticas indisponíveis.")
					return
				}
			}
		}
	}
	var wg sync.WaitGroup
	if s.usage != nil {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			if u, err := s.usage.Usage(ctx, period, previous); err == nil {
				out.Usage = &u
			}
			if units, err := s.usage.UnitAccess(ctx, period); err == nil {
				out.Units = units
			}
		})
	}
	if cfg != nil {
		// Leituras independentes em paralelo, cada uma com o timeout configurado.
		for _, g := range gaps {
			wg.Go(func() {
				moments, truncated, err := s.stats.StudyMoments(r.Context(), *cfg, g.from, g.to)
				g.days, g.err = stats.Days(g.from, g.to, moments), err
				for i := range g.days {
					g.days[i].Truncated = truncated
				}
			})
		}
		wg.Go(func() {
			if a, err := s.stats.ArchiveStatistics(r.Context(), *cfg); err == nil {
				out.Archive = &a
			}
		})
	}
	wg.Wait()
	ready := cfg != nil || len(gaps) == 0
	timedOut := false
	for _, g := range gaps {
		ready = ready && g.err == nil
		timedOut = timedOut || errors.Is(g.err, orthanc.Timeout)
	}
	if ready {
		days := []stats.Daily{}
		for _, d := range all {
			live := false
			for _, g := range gaps {
				live = live || (!d.Day.Before(g.from) && !d.Day.After(g.to))
			}
			if !live {
				days = append(days, d)
			}
		}
		for _, g := range gaps {
			days = append(days, g.days...)
		}
		exams := stats.Combine(period, days)
		prior := stats.Combine(previous, days)
		exams.PreviousTotal = prior.Total
		exams.Truncated = exams.Truncated || prior.Truncated
		out.Exams, out.ExamsError = &exams, ""
	} else if timedOut {
		out.ExamsError = "A consulta de exames excedeu o tempo disponível. Tente um período menor."
	}
	// Somente agregados: nenhum identificador de estudo ou paciente sai daqui.
	writeJSON(w, s.log, http.StatusOK, out)
}
