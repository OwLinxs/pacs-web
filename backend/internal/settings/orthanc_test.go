package settings

import "testing"

func configValida() Orthanc {
	return Orthanc{
		Name:           "Orthanc Principal",
		BaseURL:        "http://orthanc.interno.invalid:8042",
		Username:       "pacs-web",
		DICOMWebPath:   "/dicom-web",
		TimeoutSeconds: 10,
		VerifyTLS:      true,
	}
}

func TestValidateAceitaConfiguracaoValida(t *testing.T) {
	config := configValida()
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if config.Status != StatusNaoVerificado {
		t.Errorf("Status = %q, esperado %q", config.Status, StatusNaoVerificado)
	}
}

func TestValidateNormaliza(t *testing.T) {
	config := Orthanc{
		Name:         "  Orthanc Principal  ",
		BaseURL:      "  https://orthanc.interno.invalid/  ",
		Username:     "  pacs-web  ",
		DICOMWebPath: " /dicom-web/ ",
	}
	if err := config.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	casos := map[string][2]string{
		"nome":     {config.Name, "Orthanc Principal"},
		"url":      {config.BaseURL, "https://orthanc.interno.invalid"},
		"usuario":  {config.Username, "pacs-web"},
		"dicomweb": {config.DICOMWebPath, "/dicom-web"},
	}
	for campo, par := range casos {
		if par[0] != par[1] {
			t.Errorf("%s = %q, esperado %q", campo, par[0], par[1])
		}
	}
	if config.TimeoutSeconds != 10 {
		t.Errorf("timeout padrão = %d, esperado 10", config.TimeoutSeconds)
	}
}

func TestValidateRejeita(t *testing.T) {
	casos := []struct {
		nome    string
		ajustar func(*Orthanc)
	}{
		{"sem nome", func(o *Orthanc) { o.Name = "   " }},
		{"sem URL", func(o *Orthanc) { o.BaseURL = "" }},
		{"URL sem esquema", func(o *Orthanc) { o.BaseURL = "orthanc.interno.invalid:8042" }},
		{"esquema inesperado", func(o *Orthanc) { o.BaseURL = "ftp://orthanc.interno.invalid" }},
		{"esquema de arquivo", func(o *Orthanc) { o.BaseURL = "file:///etc/passwd" }},
		{"URL sem host", func(o *Orthanc) { o.BaseURL = "http://" }},
		{"credencial embutida na URL", func(o *Orthanc) { o.BaseURL = "http://usuario:senha@orthanc.interno.invalid" }},
		{"URL com query", func(o *Orthanc) { o.BaseURL = "http://orthanc.interno.invalid?a=1" }},
		{"URL com fragmento", func(o *Orthanc) { o.BaseURL = "http://orthanc.interno.invalid#x" }},
		{"porta inválida", func(o *Orthanc) { o.BaseURL = "http://orthanc.interno.invalid:99999" }},
		{"dicomweb sem barra", func(o *Orthanc) { o.DICOMWebPath = "dicom-web" }},
		{"dicomweb com travessia", func(o *Orthanc) { o.DICOMWebPath = "/dicom-web/../admin" }},
		{"timeout zero", func(o *Orthanc) { o.TimeoutSeconds = -1 }},
		{"timeout alto demais", func(o *Orthanc) { o.TimeoutSeconds = 1000 }},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			config := configValida()
			caso.ajustar(&config)
			if err := config.Validate(); err == nil {
				t.Error("esperava erro de validação")
			}
		})
	}
}

func TestValidateAceitaOrthancSemAutenticacao(t *testing.T) {
	config := configValida()
	config.Username = ""
	config.DICOMWebPath = ""
	if err := config.Validate(); err != nil {
		t.Errorf("Orthanc sem usuário e sem DICOMweb deveria ser válido: %v", err)
	}
}
