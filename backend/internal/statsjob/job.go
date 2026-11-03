// Package statsjob mantém stats_daily e stats_archive_daily a partir do
// Orthanc: recalcula os últimos dias uma vez por dia, preenche dias antigos aos
// poucos e registra o retrato do acervo. Só lê do PACS; grava apenas contagens.
package statsjob

import (
	"context"
	"log/slog"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/settings"
	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

const (
	// RecentDays são recalculados diariamente: exames chegam ao PACS com atraso.
	RecentDays = 7
	// BackfillDays cobre o ano corrente e o anterior para a comparação anual.
	BackfillDays = 800
	// perRun limita os dias antigos preenchidos por execução (carga no Orthanc).
	perRun = 200
	// chunk é o maior intervalo lido numa consulta ao Orthanc.
	chunk = 7
	// Interval entre execuções.
	Interval = time.Hour
)

type Source interface {
	StudyMoments(context.Context, orthanc.Config, time.Time, time.Time) ([]stats.Moment, bool, error)
	ArchiveStatistics(context.Context, orthanc.Config) (stats.Archive, error)
}

type Store interface {
	SaveDays(context.Context, []stats.Daily) error
	MissingDays(ctx context.Context, from, to time.Time, limit int) ([]time.Time, error)
	SaveArchive(context.Context, time.Time, stats.Archive) error
}

type Settings interface {
	OrthancConnection(context.Context) (settings.ConnectionSnapshot, error)
}

type Job struct {
	source   Source
	store    Store
	settings Settings
	log      *slog.Logger
	now      func() time.Time
	// recentDone é o dia civil em que os últimos dias já foram recalculados.
	recentDone time.Time
}

func New(source Source, store Store, cfg Settings, log *slog.Logger) *Job {
	return &Job{source: source, store: store, settings: cfg, log: log, now: time.Now}
}

// Run executa logo após o início e depois a cada Interval, até ctx encerrar.
// O canal devolvido fecha quando a rotina termina.
func (j *Job) Run(ctx context.Context) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		timer := time.NewTimer(time.Minute)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				j.RunOnce(ctx)
				timer.Reset(Interval)
			}
		}
	}()
	return done
}

// RunOnce faz uma passada completa. Falhas são registradas sem detalhes do
// PACS e a próxima execução tenta de novo.
func (j *Job) RunOnce(ctx context.Context) {
	cfg, ok := j.config(ctx)
	if !ok {
		return
	}
	today := stats.Today(j.now())

	if a, err := j.source.ArchiveStatistics(ctx, cfg); err != nil {
		j.log.Warn("estatísticas: acervo do PACS indisponível")
	} else if err := j.store.SaveArchive(ctx, today, a); err != nil {
		j.log.Error("estatísticas: falha ao gravar retrato do acervo")
	}

	if !j.recentDone.Equal(today) {
		if j.collect(ctx, cfg, today.AddDate(0, 0, -RecentDays), today.AddDate(0, 0, -1)) {
			j.recentDone = today
		}
	}

	missing, err := j.store.MissingDays(ctx, today.AddDate(0, 0, -BackfillDays), today.AddDate(0, 0, -1), perRun)
	if err != nil {
		j.log.Error("estatísticas: falha ao listar dias pendentes")
		return
	}
	filled := 0
	// missing vem do mais recente ao mais antigo; agrupa dias consecutivos.
	for i := 0; i < len(missing); {
		to, from := missing[i], missing[i]
		i++
		for i < len(missing) && missing[i].Equal(from.AddDate(0, 0, -1)) && to.Sub(missing[i]) < chunk*24*time.Hour {
			from = missing[i]
			i++
		}
		if !j.collect(ctx, cfg, from, to) {
			break
		}
		filled += int(to.Sub(from).Hours()/24) + 1
	}
	if filled > 0 {
		j.log.Info("estatísticas: dias calculados", "quantidade", filled)
	}
}

// collect lê o intervalo no Orthanc e grava os dias. Se o intervalo exceder o
// limite de leitura, repete dia a dia.
func (j *Job) collect(ctx context.Context, cfg orthanc.Config, from, to time.Time) bool {
	moments, truncated, err := j.source.StudyMoments(ctx, cfg, from, to)
	if err != nil {
		j.log.Warn("estatísticas: consulta de exames ao PACS falhou")
		return false
	}
	if truncated && from.Before(to) {
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if !j.collect(ctx, cfg, d, d) {
				return false
			}
		}
		return true
	}
	days := stats.Days(from, to, moments)
	for i := range days {
		days[i].Truncated = truncated
	}
	if err := j.store.SaveDays(ctx, days); err != nil {
		j.log.Error("estatísticas: falha ao gravar dias calculados")
		return false
	}
	return true
}

func (j *Job) config(ctx context.Context) (orthanc.Config, bool) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	snapshot, err := j.settings.OrthancConnection(ctx)
	if err != nil {
		// PACS ainda não configurado: nada a fazer, sem ruído no log.
		return orthanc.Config{}, false
	}
	return ConfigFrom(snapshot)
}

// ConfigFrom converte a conexão salva em configuração do cliente, recusando
// timeout fora de 1–120 s.
func ConfigFrom(snapshot settings.ConnectionSnapshot) (orthanc.Config, bool) {
	seconds := snapshot.Config.TimeoutSeconds
	if seconds < 1 || seconds > 120 {
		return orthanc.Config{}, false
	}
	return orthanc.Config{BaseURL: snapshot.Config.BaseURL, Username: snapshot.Config.Username, Credential: snapshot.Credential,
		Timeout: time.Duration(seconds) * time.Second, VerifyTLS: snapshot.Config.VerifyTLS}, true
}
