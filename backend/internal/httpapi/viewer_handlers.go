package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/viewer"
)

func (s *Server) prepareViewer(w http.ResponseWriter, r *http.Request, fields ...string) (orthanc.Config, bool) {
	var cfg orthanc.Config
	if r.Method != http.MethodGet || r.URL.RawQuery != "" || r.URL.ForceQuery || r.Header.Get("Range") != "" {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Consulta de imagem inválida.")
		return cfg, false
	}
	for _, field := range fields {
		if !orthanc.ValidResourceID(r.PathValue(field)) {
			writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, "Identificador de recurso inválido.")
			return cfg, false
		}
	}
	if s.viewer == nil {
		s.viewerError(w, orthanc.Unavailable)
		return cfg, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	snapshot, err := s.settings.OrthancConnection(ctx)
	cancel()
	if err != nil || snapshot.Config.TimeoutSeconds < 1 || snapshot.Config.TimeoutSeconds > 120 {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Visualização indisponível. Contate o administrador do PACS.")
		return cfg, false
	}
	cfg = orthanc.Config{BaseURL: snapshot.Config.BaseURL, Username: snapshot.Config.Username, Credential: snapshot.Credential, Timeout: time.Duration(snapshot.Config.TimeoutSeconds) * time.Second, VerifyTLS: snapshot.Config.VerifyTLS}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(cfg.Timeout + 5*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		s.viewerError(w, orthanc.Unavailable)
		return cfg, false
	}
	return cfg, true
}

func (s *Server) handleViewerSeries(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.prepareViewer(w, r, "studyID")
	if !ok {
		return
	}
	items, err := s.viewer.ViewerSeries(r.Context(), cfg, r.PathValue("studyID"))
	if err != nil {
		s.viewerError(w, err)
		return
	}
	if items == nil {
		items = []viewer.Series{}
	}
	writeJSON(w, s.log, http.StatusOK, struct {
		Items []viewer.Series `json:"items"`
	}{items})
}

func (s *Server) handleViewerInstances(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.prepareViewer(w, r, "studyID", "seriesID")
	if !ok {
		return
	}
	items, err := s.viewer.ViewerInstances(r.Context(), cfg, r.PathValue("studyID"), r.PathValue("seriesID"))
	if err != nil {
		s.viewerError(w, err)
		return
	}
	if items == nil {
		items = []viewer.Instance{}
	}
	writeJSON(w, s.log, http.StatusOK, struct {
		Items []viewer.Instance `json:"items"`
	}{items})
}

func (s *Server) handleViewerDICOM(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.prepareViewer(w, r, "studyID", "seriesID", "instanceID")
	if !ok {
		return
	}
	file, err := s.viewer.OpenDICOM(r.Context(), cfg, r.PathValue("studyID"), r.PathValue("seriesID"), r.PathValue("instanceID"))
	if err != nil {
		s.viewerError(w, err)
		return
	}
	defer file.Body.Close()
	if file.Size > viewer.MaxDICOMBytes {
		s.viewerError(w, orthanc.TooLarge)
		return
	}
	w.Header().Set("Content-Type", "application/dicom")
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("X-Accel-Buffering", "no")
	if file.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	}
	w.WriteHeader(http.StatusOK)
	// Buffer fixo: nenhum arquivo temporário e nenhum io.ReadAll para DICOM.
	if _, err := io.CopyBuffer(w, file.Body, make([]byte, 32<<10)); err != nil {
		s.log.WarnContext(r.Context(), "transferência DICOM interrompida")
		// Depois dos headers não é possível enviar JSON. Interrompe o stream,
		// evitando que um arquivo parcial seja apresentado como resposta completa.
		panic(http.ErrAbortHandler)
	}
}

func (s *Server) viewerError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusBadGateway, "VIEWER_UNAVAILABLE", "Não foi possível carregar a imagem no PACS. Tente novamente."
	switch {
	case errors.Is(err, orthanc.NotFound):
		status, code, message = http.StatusNotFound, CodeNotFound, "Estudo, série ou imagem não disponível."
	case errors.Is(err, orthanc.Timeout):
		status, code, message = http.StatusGatewayTimeout, "VIEWER_TIMEOUT", "O PACS não respondeu dentro do tempo disponível."
	case errors.Is(err, orthanc.TooLarge):
		status, code, message = http.StatusBadGateway, "VIEWER_LIMIT", "A imagem excede o limite desta versão do visualizador."
	case errors.Is(err, orthanc.InvalidResponse):
		code, message = "VIEWER_INVALID_RESPONSE", "O PACS retornou conteúdo incompatível com o visualizador."
	}
	writeError(w, s.log, status, code, message)
}
