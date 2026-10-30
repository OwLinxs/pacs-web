package orthanc

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/stats"
)

// Estudos expandidos ocupam ~2 KiB; o teto cobre MaxStudies+1 com folga.
const maxStatsBytes = 48 << 20

// StudyMoments lê, numa única consulta read-only, StudyDate/StudyTime dos estudos
// do intervalo. Demais tags (inclusive de paciente) não são decodificadas.
// truncated indica que o período excede stats.MaxStudies.
func (c *Client) StudyMoments(ctx context.Context, cfg Config, from, to time.Time) ([]stats.Moment, bool, error) {
	if to.Before(from) {
		return nil, false, InvalidTarget
	}
	s, err := c.newSession(cfg)
	if err != nil {
		return nil, false, err
	}
	defer s.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	payload, err := json.Marshal(map[string]any{
		"Level": "Study", "Expand": true, "Limit": stats.MaxStudies + 1,
		"Query": map[string]string{"StudyDate": from.Format("20060102") + "-" + to.Format("20060102")},
	})
	if err != nil {
		return nil, false, InvalidTarget
	}
	body, err := s.read(ctx, http.MethodPost, "/tools/find", payload, maxStatsBytes)
	if err != nil {
		return nil, false, err
	}
	var found []struct {
		Type          string
		MainDicomTags struct{ StudyDate, StudyTime string }
	}
	if json.Unmarshal(body, &found) != nil || found == nil || len(found) > stats.MaxStudies+1 {
		return nil, false, InvalidResponse
	}
	truncated := len(found) > stats.MaxStudies
	if truncated {
		found = found[:stats.MaxStudies]
	}
	moments := make([]stats.Moment, 0, len(found))
	for _, study := range found {
		if study.Type != "Study" {
			return nil, false, InvalidResponse
		}
		moments = append(moments, stats.Moment{Date: study.MainDicomTags.StudyDate, Time: study.MainDicomTags.StudyTime})
	}
	return moments, truncated, nil
}

// ArchiveStatistics lê GET /statistics: totais do acervo inteiro.
func (c *Client) ArchiveStatistics(ctx context.Context, cfg Config) (stats.Archive, error) {
	s, err := c.newSession(cfg)
	if err != nil {
		return stats.Archive{}, err
	}
	defer s.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	body, err := s.read(ctx, http.MethodGet, "/statistics", nil, maxSystemBytes)
	if err != nil {
		return stats.Archive{}, err
	}
	var raw struct {
		CountStudies, CountSeries, CountInstances *int64
		TotalDiskSizeMB                           *int64
	}
	if json.Unmarshal(body, &raw) != nil || raw.CountStudies == nil || raw.CountSeries == nil || raw.CountInstances == nil || raw.TotalDiskSizeMB == nil ||
		*raw.CountStudies < 0 || *raw.CountSeries < 0 || *raw.CountInstances < 0 || *raw.TotalDiskSizeMB < 0 {
		return stats.Archive{}, InvalidResponse
	}
	return stats.Archive{Studies: *raw.CountStudies, Series: *raw.CountSeries, Instances: *raw.CountInstances, DiskMB: *raw.TotalDiskSizeMB}, nil
}
