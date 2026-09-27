package secrets

import (
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

func chaveDeTeste(t *testing.T) []byte {
	t.Helper()
	chave := make([]byte, TamanhoChave)
	for i := range chave {
		chave[i] = byte(i + 1)
	}
	return chave
}

func TestSealEOpen(t *testing.T) {
	cifra, err := NewCipher(chaveDeTeste(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}

	const valor = "credencial-de-teste"
	const contexto = "orthanc:credential"

	cifrado, err := cifra.Seal(valor, contexto)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if strings.Contains(cifrado, valor) {
		t.Error("o valor cifrado não pode conter o texto original")
	}

	aberto, err := cifra.Open(cifrado, contexto)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if aberto != valor {
		t.Errorf("Open = %q, esperado %q", aberto, valor)
	}
}

func TestSealGeraNonceDiferente(t *testing.T) {
	cifra, err := NewCipher(chaveDeTeste(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	primeiro, err := cifra.Seal("mesmo-valor", "ctx")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	segundo, err := cifra.Seal("mesmo-valor", "ctx")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if primeiro == segundo {
		t.Error("duas cifragens do mesmo valor devem diferir pelo nonce")
	}
}

func TestOpenRejeitaAlteracao(t *testing.T) {
	cifra, err := NewCipher(chaveDeTeste(t))
	if err != nil {
		t.Fatalf("NewCipher: %v", err)
	}
	cifrado, err := cifra.Seal("credencial-de-teste", "orthanc:credential")
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}

	t.Run("contexto diferente", func(t *testing.T) {
		if _, err := cifra.Open(cifrado, "outro:proposito"); !errors.Is(err, ErrValorCorrompido) {
			t.Errorf("erro = %v, esperado ErrValorCorrompido", err)
		}
	})

	t.Run("bytes alterados", func(t *testing.T) {
		bruto, err := base64.StdEncoding.DecodeString(cifrado)
		if err != nil {
			t.Fatalf("decodificar: %v", err)
		}
		bruto[len(bruto)-1] ^= 0xff
		alterado := base64.StdEncoding.EncodeToString(bruto)
		if _, err := cifra.Open(alterado, "orthanc:credential"); !errors.Is(err, ErrValorCorrompido) {
			t.Errorf("erro = %v, esperado ErrValorCorrompido", err)
		}
	})

	t.Run("chave trocada", func(t *testing.T) {
		outraChave := chaveDeTeste(t)
		outraChave[0] ^= 0xff
		outra, err := NewCipher(outraChave)
		if err != nil {
			t.Fatalf("NewCipher: %v", err)
		}
		if _, err := outra.Open(cifrado, "orthanc:credential"); !errors.Is(err, ErrValorCorrompido) {
			t.Errorf("erro = %v, esperado ErrValorCorrompido", err)
		}
	})

	t.Run("base64 inválido", func(t *testing.T) {
		if _, err := cifra.Open("###", "orthanc:credential"); !errors.Is(err, ErrValorCorrompido) {
			t.Errorf("erro = %v, esperado ErrValorCorrompido", err)
		}
	})

	t.Run("curto demais", func(t *testing.T) {
		curto := base64.StdEncoding.EncodeToString([]byte{1, 2, 3})
		if _, err := cifra.Open(curto, "orthanc:credential"); !errors.Is(err, ErrValorCorrompido) {
			t.Errorf("erro = %v, esperado ErrValorCorrompido", err)
		}
	})
}

func TestParseKey(t *testing.T) {
	chave := chaveDeTeste(t)

	validos := map[string]string{
		"base64 padrão":   base64.StdEncoding.EncodeToString(chave),
		"base64 sem pad":  base64.RawStdEncoding.EncodeToString(chave),
		"base64 url-safe": base64.RawURLEncoding.EncodeToString(chave),
		"hex":             hex.EncodeToString(chave),
		"com espaços":     "  " + base64.StdEncoding.EncodeToString(chave) + "\n",
	}
	for nome, bruto := range validos {
		t.Run(nome, func(t *testing.T) {
			lida, err := ParseKey(bruto)
			if err != nil {
				t.Fatalf("ParseKey: %v", err)
			}
			if string(lida) != string(chave) {
				t.Error("chave lida difere da original")
			}
		})
	}

	invalidos := map[string]string{
		"vazia":       "",
		"curta":       base64.StdEncoding.EncodeToString([]byte("chave-curta")),
		"longa":       base64.StdEncoding.EncodeToString(append(chave, chave...)),
		"não é chave": "isto-nao-e-uma-chave",
	}
	for nome, bruto := range invalidos {
		t.Run(nome, func(t *testing.T) {
			if _, err := ParseKey(bruto); err == nil {
				t.Error("esperava erro")
			}
		})
	}
}

func TestGenerateKey(t *testing.T) {
	primeira, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	segunda, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if primeira == segunda {
		t.Error("duas chaves geradas não podem ser iguais")
	}
	if _, err := NewCipherFromString(primeira); err != nil {
		t.Errorf("a chave gerada deveria ser utilizável: %v", err)
	}
}
