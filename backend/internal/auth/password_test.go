package auth

import (
	"errors"
	"strings"
	"testing"
)

// paramsRapidos reduz o custo do Argon2id nos testes. A força real é validada
// separadamente em TestHashUsaParametrosPadrao.
func paramsRapidos() Argon2Params {
	return Argon2Params{Memory: 8 * 1024, Time: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

func TestHashEVerify(t *testing.T) {
	hasher := NewHasher(paramsRapidos())
	const senha = "senha-de-teste-longa"

	hash, err := hasher.Hash(senha)
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("hash deve estar no formato PHC do argon2id, veio %q", hash)
	}
	if strings.Contains(hash, senha) {
		t.Error("o hash não pode conter a senha")
	}

	casos := []struct {
		nome     string
		senha    string
		esperaOk bool
	}{
		{"senha correta", senha, true},
		{"senha errada", "senha-de-teste-erra", false},
		{"senha vazia", "", false},
		{"prefixo da senha", senha[:len(senha)-1], false},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			ok, _, err := hasher.Verify(hash, caso.senha)
			if err != nil {
				t.Fatalf("Verify: %v", err)
			}
			if ok != caso.esperaOk {
				t.Errorf("Verify = %v, esperado %v", ok, caso.esperaOk)
			}
		})
	}
}

func TestHashGeraSalDiferente(t *testing.T) {
	hasher := NewHasher(paramsRapidos())
	primeiro, err := hasher.Hash("senha-de-teste-longa")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	segundo, err := hasher.Hash("senha-de-teste-longa")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	if primeiro == segundo {
		t.Error("dois hashes da mesma senha devem diferir pelo sal")
	}
}

func TestHashUsaParametrosPadrao(t *testing.T) {
	hash, err := NewDefaultHasher().Hash("senha-de-teste-longa")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}
	params, sal, chave, err := decodeHash(hash)
	if err != nil {
		t.Fatalf("decodeHash: %v", err)
	}
	padrao := DefaultArgon2Params()
	if params.Memory != padrao.Memory || params.Time != padrao.Time || params.Parallelism != padrao.Parallelism {
		t.Errorf("parâmetros gravados = %+v, esperado %+v", params, padrao)
	}
	if len(sal) != int(padrao.SaltLength) {
		t.Errorf("tamanho do sal = %d, esperado %d", len(sal), padrao.SaltLength)
	}
	if len(chave) != int(padrao.KeyLength) {
		t.Errorf("tamanho da chave = %d, esperado %d", len(chave), padrao.KeyLength)
	}
}

func TestVerifyIndicaRehash(t *testing.T) {
	fraco := NewHasher(paramsRapidos())
	hashFraco, err := fraco.Hash("senha-de-teste-longa")
	if err != nil {
		t.Fatalf("Hash: %v", err)
	}

	forte := NewDefaultHasher()
	ok, precisaRehash, err := forte.Verify(hashFraco, "senha-de-teste-longa")
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if !ok {
		t.Fatal("senha correta deveria validar mesmo com parâmetros antigos")
	}
	if !precisaRehash {
		t.Error("hash com parâmetros mais fracos deveria pedir rehash")
	}

	ok, precisaRehash, err = fraco.Verify(hashFraco, "senha-de-teste-longa")
	if err != nil || !ok {
		t.Fatalf("Verify com os mesmos parâmetros: ok=%v err=%v", ok, err)
	}
	if precisaRehash {
		t.Error("hash com os parâmetros atuais não deveria pedir rehash")
	}
}

func TestVerifyHashInvalido(t *testing.T) {
	hasher := NewHasher(paramsRapidos())
	casos := []struct {
		nome string
		hash string
	}{
		{"vazio", ""},
		{"texto puro", "senha-em-texto-puro"},
		{"algoritmo diferente", "$argon2i$v=19$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaA"},
		{"sem parâmetros", "$argon2id$v=19$$c2FsdA$aGFzaA"},
		{"base64 inválido", "$argon2id$v=19$m=8192,t=1,p=1$###$###"},
		{"versão desconhecida", "$argon2id$v=1$m=8192,t=1,p=1$c2FsdHNhbHRzYWx0c2E$aGFzaA"},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			ok, _, err := hasher.Verify(caso.hash, "qualquer-senha-longa")
			if ok {
				t.Error("hash inválido não pode validar senha")
			}
			if !errors.Is(err, ErrHashInvalido) {
				t.Errorf("erro = %v, esperado ErrHashInvalido", err)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	casos := []struct {
		nome       string
		senha      string
		esperaErro bool
	}{
		{"comprimento mínimo", strings.Repeat("a", MinPasswordLength), false},
		{"frase longa", "uma frase de acesso bem comprida", false},
		{"curta demais", strings.Repeat("a", MinPasswordLength-1), true},
		{"vazia", "", true},
		{"só espaços", strings.Repeat(" ", MinPasswordLength+2), true},
		{"gigante", strings.Repeat("a", maxPasswordLength+1), true},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			err := ValidatePassword(caso.senha)
			if caso.esperaErro && err == nil {
				t.Error("esperava erro de política")
			}
			if !caso.esperaErro && err != nil {
				t.Errorf("erro inesperado: %v", err)
			}
		})
	}
}

func TestHashRejeitaSenhaFraca(t *testing.T) {
	_, err := NewHasher(paramsRapidos()).Hash("curta")
	if !errors.Is(err, ErrSenhaFraca) {
		t.Errorf("erro = %v, esperado ErrSenhaFraca", err)
	}
}
