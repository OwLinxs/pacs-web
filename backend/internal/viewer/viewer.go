// Package viewer define os contratos mínimos do visualizador, sem resposta REST bruta.
package viewer

import "io"

const MaxDICOMBytes int64 = 128 << 20
const MaxInstances = 10000

type Series struct {
	OrthancSeriesID string `json:"orthancSeriesId"`
	Description     string `json:"description"`
	Number          string `json:"number"`
	Modality        string `json:"modality"`
	InstanceCount   int    `json:"instanceCount"`
}

type Instance struct {
	OrthancInstanceID string `json:"orthancInstanceId"`
	Number            *int   `json:"number"`
}

// DICOM possui apenas o stream e seu tamanho. O chamador precisa fechar Body.
// Headers, URL, credencial e resposta do Orthanc nunca atravessam esta interface.
type DICOM struct {
	Body io.ReadCloser
	Size int64
}
