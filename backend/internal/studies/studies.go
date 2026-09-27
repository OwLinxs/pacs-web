// Package studies define o contrato da Worklist, independente da resposta Orthanc.
package studies

import (
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultLimit = 25
	MaxLimit     = 50
	MaxOffset    = 10000
)

type Query struct {
	Limit, Offset                                                              int
	DateFrom, DateTo                                                           string
	PatientName, PatientID, AccessionNumber, StudyDescription, InstitutionName string
}

// Validate não inclui valores recebidos nas mensagens de erro.
func (q Query) Validate() error {
	if q.Limit < 1 || q.Limit > MaxLimit || q.Offset < 0 || q.Offset > MaxOffset {
		return errors.New("Paginação inválida: limite de 1 a 50 e deslocamento de 0 a 10000.")
	}
	for _, date := range []string{q.DateFrom, q.DateTo} {
		if date == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", date); err != nil || len(date) != 10 {
			return errors.New("Informe datas válidas no formato AAAA-MM-DD.")
		}
	}
	if q.DateFrom != "" && q.DateTo != "" && q.DateFrom > q.DateTo {
		return errors.New("A data inicial deve ser anterior ou igual à data final.")
	}
	for _, value := range []string{q.PatientName, q.PatientID, q.AccessionNumber, q.StudyDescription, q.InstitutionName} {
		if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 128 || strings.Contains(value, "\\") || strings.ContainsFunc(value, unicode.IsControl) {
			return errors.New("Filtro inválido: use até 128 caracteres, sem controles ou listas de valores.")
		}
	}
	return nil
}

// Study contém somente os campos clínicos necessários para a listagem.
// Tags ausentes são strings vazias; modalidades ausentes são uma lista vazia.
type Study struct {
	OrthancStudyID   string   `json:"orthancStudyId"`
	StudyInstanceUID string   `json:"studyInstanceUid"`
	StudyDate        string   `json:"studyDate"`
	StudyTime        string   `json:"studyTime"`
	PatientName      string   `json:"patientName"`
	PatientID        string   `json:"patientId"`
	AccessionNumber  string   `json:"accessionNumber"`
	StudyDescription string   `json:"studyDescription"`
	InstitutionName  string   `json:"institutionName"`
	Modalities       []string `json:"modalities"`
	SeriesCount      int      `json:"seriesCount"`
}

type Page struct {
	Items      []Study `json:"items"`
	Limit      int     `json:"limit"`
	Offset     int     `json:"offset"`
	HasMore    bool    `json:"hasMore"`
	NextOffset *int    `json:"nextOffset"`
}
