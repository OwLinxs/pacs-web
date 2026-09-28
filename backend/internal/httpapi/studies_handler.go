package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/pmfb-saude/pacs-web/backend/internal/orthanc"
	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
)

func parseStudiesQuery(raw string) (studies.Query, error) {
	q := studies.Query{Limit: studies.DefaultLimit, Sort: "dateDesc"}
	if len(raw) > 4096 {
		return q, errors.New("Filtros excedem o tamanho permitido.")
	}
	values, err := url.ParseQuery(raw)
	if err != nil {
		return q, errors.New("Parâmetros de consulta inválidos.")
	}
	texts := map[string]*string{
		"modality": &q.Modality, "sort": &q.Sort,
		"dateFrom": &q.DateFrom, "dateTo": &q.DateTo, "patientName": &q.PatientName,
		"patientId": &q.PatientID, "accessionNumber": &q.AccessionNumber,
		"studyDescription": &q.StudyDescription, "institutionName": &q.InstitutionName,
	}
	for key, list := range values {
		if len(list) != 1 {
			return q, errors.New("Não repita parâmetros de consulta.")
		}
		if key == "limit" || key == "offset" {
			n, err := strconv.Atoi(list[0])
			if err != nil {
				return q, errors.New("Paginação inválida.")
			}
			if key == "limit" {
				q.Limit = n
			} else {
				q.Offset = n
			}
		} else if field, ok := texts[key]; ok {
			*field = strings.TrimSpace(list[0])
		} else {
			return q, errors.New("Parâmetro de consulta não suportado.")
		}
	}
	return q, q.Validate()
}

func (s *Server) handleStudies(w http.ResponseWriter, r *http.Request) {
	query, err := parseStudiesQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, s.log, http.StatusBadRequest, CodeInvalidRequest, err.Error())
		return
	}
	if s.studies == nil {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Consulta de exames indisponível.")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	snapshot, err := s.settings.OrthancConnection(ctx)
	cancel()
	if err != nil || snapshot.Config.TimeoutSeconds < 1 || snapshot.Config.TimeoutSeconds > 120 {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Consulta de exames indisponível. Contate o administrador do PACS.")
		return
	}
	cfg := snapshot.Config
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(timeout + 5*time.Second)); err != nil && !errors.Is(err, http.ErrNotSupported) {
		writeError(w, s.log, http.StatusServiceUnavailable, CodeUnavailable, "Consulta de exames indisponível.")
		return
	}
	page, err := s.studies.FindStudies(r.Context(), orthanc.Config{
		BaseURL: cfg.BaseURL, Username: cfg.Username, Credential: snapshot.Credential,
		Timeout: timeout, VerifyTLS: cfg.VerifyTLS,
	}, query)
	if err != nil {
		// Nunca registra erro upstream, filtros, identificadores ou corpo clínico.
		status, code, message := http.StatusBadGateway, "PACS_UNAVAILABLE", "Não foi possível consultar os exames no PACS. Tente novamente; se persistir, contate o suporte de TI."
		if errors.Is(err, orthanc.UnsupportedQuery) {
			status, code, message = http.StatusUnprocessableEntity, "PACS_QUERY_UNSUPPORTED", "O PACS não suporta esta ordenação ou modalidade com paginação segura. Use ordem nativa sem modalidade ou contate o administrador."
		}
		if errors.Is(err, orthanc.Timeout) {
			status, code, message = http.StatusGatewayTimeout, "PACS_TIMEOUT", "A consulta excedeu o tempo disponível. Reduza o período ou refine os filtros."
		}
		if errors.Is(err, orthanc.InvalidResponse) {
			code, message = "PACS_INVALID_RESPONSE", "O PACS retornou uma resposta incompatível ou mudou durante a consulta. Tente novamente."
		}
		writeError(w, s.log, status, code, message)
		return
	}
	// O requestLogger existente registra somente rota, status, duração e origem.
	// Não introduz auditoria de pacientes nem persistência local de dados clínicos.
	writeJSON(w, s.log, http.StatusOK, page)
}
