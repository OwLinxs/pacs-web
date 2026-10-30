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
)

type StatsSource interface {
	StudyMoments(context.Context, orthanc.Config, time.Time, time.Time) ([]stats.Moment, bool, error)
	ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error)
}

type UsageReader interface {
	Usage(context.Context, stats.Period, stats.Period) (stats.Usage, error)
}

// Cada seção é opcional: falha no PACS não esconde o uso, e vice-versa.
type statsResponse struct {
	Period       string         `json:"period"`
	From         string         `json:"from"`
	To           string         `json:"to"`
	PreviousFrom string         `json:"previousFrom"`
	PreviousTo   string         `json:"previousTo"`
	Exams        *stats.Exams   `json:"exams"`
	ExamsError   string         `json:"examsError,omitempty"`
	Archive      *stats.Archive `json:"archive"`
	Usage        *stats.Usage   `json:"usage"`
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
	day := func(t time.Time) string { return t.Format("2006-01-02") }
	out := statsResponse{Period: period.Kind, From: day(period.From), To: day(period.To), PreviousFrom: day(previous.From), PreviousTo: day(previous.To),
		ExamsError: "Estatísticas de exames indisponíveis. Verifique a conexão com o PACS."}

	// Configuração do PACS resolvida antes de qualquer goroutine.
	var cfg *orthanc.Config
	if s.stats != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		snapshot, err := s.settings.OrthancConnection(ctx)
		cancel()
		if err == nil && snapshot.Config.TimeoutSeconds >= 1 && snapshot.Config.TimeoutSeconds <= 120 {
			timeout := time.Duration(snapshot.Config.TimeoutSeconds) * time.Second
			cfg = &orthanc.Config{BaseURL: snapshot.Config.BaseURL, Username: snapshot.Config.Username, Credential: snapshot.Credential, Timeout: timeout, VerifyTLS: snapshot.Config.VerifyTLS}
			if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 10*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
				writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Estatísticas indisponíveis.")
				return
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
		})
	}
	var current, prior []stats.Moment
	var truncated, priorTruncated bool
	var currentErr, priorErr error
	if cfg != nil {
		// Leituras independentes em paralelo, cada uma com o timeout configurado.
		wg.Go(func() {
			current, truncated, currentErr = s.stats.StudyMoments(r.Context(), *cfg, period.From, period.To)
		})
		wg.Go(func() {
			prior, priorTruncated, priorErr = s.stats.StudyMoments(r.Context(), *cfg, previous.From, previous.To)
		})
		wg.Go(func() {
			if a, err := s.stats.ArchiveStatistics(r.Context(), *cfg); err == nil {
				out.Archive = &a
			}
		})
	}
	wg.Wait()
	if cfg != nil {
		if currentErr == nil && priorErr == nil {
			exams := stats.Aggregate(period, current)
			exams.PreviousTotal = stats.Aggregate(previous, prior).Total
			exams.Truncated = truncated || priorTruncated
			out.Exams, out.ExamsError = &exams, ""
		} else if errors.Is(currentErr, orthanc.Timeout) || errors.Is(priorErr, orthanc.Timeout) {
			out.ExamsError = "A consulta de exames excedeu o tempo disponível. Tente um período menor."
		}
	}
	// Somente agregados: nenhum identificador de estudo ou paciente sai daqui.
	writeJSON(w, s.log, http.StatusOK, out)
}
