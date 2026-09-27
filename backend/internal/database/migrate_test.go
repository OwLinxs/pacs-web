package database

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadMigrationsDoProjeto(t *testing.T) {
	lista, err := LoadMigrations()
	if err != nil {
		t.Fatalf("LoadMigrations: %v", err)
	}
	if len(lista) == 0 {
		t.Fatal("o projeto deveria ter ao menos uma migration embutida")
	}
	if lista[0].Version != 1 {
		t.Errorf("primeira versão = %d, esperado 1", lista[0].Version)
	}
	if lista[0].Name != "init" {
		t.Errorf("nome da primeira migration = %q, esperado \"init\"", lista[0].Name)
	}
	if lista[0].Checksum == "" {
		t.Error("checksum não deveria ser vazio")
	}
	for _, migration := range lista {
		if strings.TrimSpace(migration.SQL) == "" {
			t.Errorf("migration %04d está vazia", migration.Version)
		}
		if strings.Contains(strings.ToUpper(migration.SQL), "DROP TABLE") {
			t.Errorf("migration %04d contém DROP TABLE: migrations desta fase não são destrutivas", migration.Version)
		}
	}
}

func TestLoadFromOrdenaPorVersao(t *testing.T) {
	sistema := fstest.MapFS{
		"0010_sessoes.sql":  {Data: []byte("SELECT 10;")},
		"0002_usuarios.sql": {Data: []byte("SELECT 2;")},
		"0001_init.sql":     {Data: []byte("SELECT 1;")},
		"leia-me.txt":       {Data: []byte("ignorado")},
	}

	lista, err := loadFrom(sistema)
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if len(lista) != 3 {
		t.Fatalf("migrations carregadas = %d, esperado 3", len(lista))
	}
	esperado := []int{1, 2, 10}
	for i, versao := range esperado {
		if lista[i].Version != versao {
			t.Errorf("posição %d = versão %d, esperado %d", i, lista[i].Version, versao)
		}
	}
}

func TestLoadFromRejeitaEntradasInvalidas(t *testing.T) {
	casos := map[string]fstest.MapFS{
		"sem separador": {
			"0001.sql": {Data: []byte("SELECT 1;")},
		},
		"versão não numérica": {
			"inicial_init.sql": {Data: []byte("SELECT 1;")},
		},
		"versão zero": {
			"0000_init.sql": {Data: []byte("SELECT 1;")},
		},
		"versão duplicada": {
			"0001_init.sql":  {Data: []byte("SELECT 1;")},
			"0001_outro.sql": {Data: []byte("SELECT 2;")},
		},
	}

	for nome, sistema := range casos {
		t.Run(nome, func(t *testing.T) {
			if _, err := loadFrom(sistema); err == nil {
				t.Error("esperava erro de carregamento")
			}
		})
	}
}

func TestChecksumMudaComConteudo(t *testing.T) {
	primeiro, err := loadFrom(fstest.MapFS{"0001_init.sql": {Data: []byte("SELECT 1;")}})
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	segundo, err := loadFrom(fstest.MapFS{"0001_init.sql": {Data: []byte("SELECT 2;")}})
	if err != nil {
		t.Fatalf("loadFrom: %v", err)
	}
	if primeiro[0].Checksum == segundo[0].Checksum {
		t.Error("conteúdos diferentes deveriam gerar checksums diferentes")
	}
}
