package orthanc

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"slices"
	"sort"
	"strconv"

	"github.com/pmfb-saude/pacs-web/backend/internal/viewer"
)

func ValidResourceID(id string) bool { return resourceID.MatchString(id) }

type viewerSeries struct {
	ID, Type, ParentStudy string
	MainDicomTags         map[string]string
	Instances             []string
}

func validInstances(ids []string) bool {
	if ids == nil || len(ids) > viewer.MaxInstances {
		return false
	}
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !ValidResourceID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func (c *Client) ViewerSeries(ctx context.Context, cfg Config, studyID string) ([]viewer.Series, error) {
	if !ValidResourceID(studyID) {
		return nil, InvalidTarget
	}
	s, err := c.newSession(cfg)
	if err != nil {
		return nil, err
	}
	defer s.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	body, err := s.read(ctx, http.MethodGet, "/studies/"+studyID+"/series?expand=true", nil, maxStudyBytes)
	if err != nil {
		return nil, err
	}
	var raw []viewerSeries
	if json.Unmarshal(body, &raw) != nil || raw == nil || len(raw) > maxSeriesPerStudy {
		return nil, InvalidResponse
	}
	items := make([]viewer.Series, 0, len(raw))
	seen := map[string]bool{}
	for _, series := range raw {
		if !ValidResourceID(series.ID) || seen[series.ID] || series.Type != "Series" || series.ParentStudy != studyID || !validInstances(series.Instances) {
			return nil, InvalidResponse
		}
		seen[series.ID] = true
		items = append(items, viewer.Series{OrthancSeriesID: series.ID, Description: series.MainDicomTags["SeriesDescription"], Number: series.MainDicomTags["SeriesNumber"], Modality: series.MainDicomTags["Modality"], InstanceCount: len(series.Instances)})
	}
	// Ordem de apresentação, não ordenação espacial para reconstrução de volume.
	sort.Slice(items, func(i, j int) bool {
		a, ea := strconv.Atoi(items[i].Number)
		b, eb := strconv.Atoi(items[j].Number)
		if ea == nil && eb == nil && a != b {
			return a < b
		}
		if (ea == nil) != (eb == nil) {
			return ea == nil
		}
		return items[i].OrthancSeriesID < items[j].OrthancSeriesID
	})
	return items, nil
}

func (s *session) checkedSeries(ctx context.Context, studyID, seriesID string) (viewerSeries, error) {
	var series viewerSeries
	body, err := s.read(ctx, http.MethodGet, "/series/"+seriesID, nil, maxStudyBytes)
	if err != nil {
		return series, err
	}
	if json.Unmarshal(body, &series) != nil || series.ID != seriesID || series.Type != "Series" || !validInstances(series.Instances) {
		return series, InvalidResponse
	}
	if series.ParentStudy != studyID {
		return series, NotFound
	}
	return series, nil
}

func (c *Client) ViewerInstances(ctx context.Context, cfg Config, studyID, seriesID string) ([]viewer.Instance, error) {
	if !ValidResourceID(studyID) || !ValidResourceID(seriesID) {
		return nil, InvalidTarget
	}
	s, err := c.newSession(cfg)
	if err != nil {
		return nil, err
	}
	defer s.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	series, err := s.checkedSeries(ctx, studyID, seriesID)
	if err != nil {
		return nil, err
	}
	items := []viewer.Instance{}
	if len(series.Instances) == 0 {
		return items, nil
	}
	body, err := s.read(ctx, http.MethodGet, "/series/"+seriesID+"/instances?expand=true", nil, maxStudyBytes)
	if err != nil {
		return nil, err
	}
	var raw []struct {
		ID, Type, ParentSeries string
		IndexInSeries          *int
	}
	if json.Unmarshal(body, &raw) != nil || raw == nil || len(raw) != len(series.Instances) {
		return nil, InvalidResponse
	}
	expected := make(map[string]bool, len(series.Instances))
	for _, id := range series.Instances {
		expected[id] = true
	}
	for _, instance := range raw {
		if instance.Type != "Instance" || instance.ParentSeries != seriesID || !expected[instance.ID] {
			return nil, InvalidResponse
		}
		delete(expected, instance.ID)
		items = append(items, viewer.Instance{OrthancInstanceID: instance.ID, Number: instance.IndexInSeries})
	}
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i].Number, items[j].Number
		if a != nil && b != nil && *a != *b {
			return *a < *b
		}
		if (a != nil) != (b != nil) {
			return a != nil
		}
		return items[i].OrthancInstanceID < items[j].OrthancInstanceID
	})
	return items, nil
}

// OpenDICOM transmite um único arquivo Part 10. Lê apenas o prefixo antes de
// devolver o stream; não grava arquivo nem mantém o DICOM inteiro em memória.
func (c *Client) OpenDICOM(ctx context.Context, cfg Config, studyID, seriesID, instanceID string) (viewer.DICOM, error) {
	if !ValidResourceID(studyID) || !ValidResourceID(seriesID) || !ValidResourceID(instanceID) {
		return viewer.DICOM{}, InvalidTarget
	}
	s, err := c.newSession(cfg)
	if err != nil {
		return viewer.DICOM{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	cleanup := func() { cancel(); s.client.CloseIdleConnections() }
	failed := true
	defer func() {
		if failed {
			cleanup()
		}
	}()
	series, err := s.checkedSeries(ctx, studyID, seriesID)
	if err != nil {
		return viewer.DICOM{}, err
	}
	if !slices.Contains(series.Instances, instanceID) {
		return viewer.DICOM{}, NotFound
	}
	response, err := s.open(ctx, http.MethodGet, "/instances/"+instanceID+"/file", nil, "application/dicom")
	if err != nil {
		return viewer.DICOM{}, err
	}
	defer func() {
		if failed {
			response.Body.Close()
		}
	}()
	if response.ContentLength > viewer.MaxDICOMBytes {
		return viewer.DICOM{}, TooLarge
	}
	media, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || (media != "application/dicom" && media != "application/octet-stream") || (response.Header.Get("Content-Encoding") != "" && response.Header.Get("Content-Encoding") != "identity") {
		return viewer.DICOM{}, InvalidResponse
	}
	prefix := make([]byte, 132)
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		if ctx.Err() != nil {
			return viewer.DICOM{}, classify(ctx.Err())
		}
		return viewer.DICOM{}, InvalidResponse
	}
	if !bytes.Equal(prefix[128:], []byte("DICM")) {
		return viewer.DICOM{}, InvalidResponse
	}
	// Preamble não carrega metadados para o frontend antes da validação mínima.
	reader := io.MultiReader(bytes.NewReader(prefix), response.Body)
	failed = false
	return viewer.DICOM{Size: response.ContentLength, Body: &dicomStream{reader: reader, closer: response.Body, cleanup: cleanup, remaining: viewer.MaxDICOMBytes}}, nil
}

type dicomStream struct {
	reader    io.Reader
	closer    io.Closer
	cleanup   func()
	remaining int64
}

func (d *dicomStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if d.remaining == 0 {
		var probe [1]byte
		n, err := d.reader.Read(probe[:])
		if n > 0 {
			return 0, TooLarge
		}
		return 0, err
	}
	if int64(len(p)) > d.remaining {
		p = p[:d.remaining]
	}
	n, err := d.reader.Read(p)
	d.remaining -= int64(n)
	return n, err
}

func (d *dicomStream) Close() error { err := d.closer.Close(); d.cleanup(); return err }
