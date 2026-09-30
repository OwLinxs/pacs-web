// Package auth concentra hashing de senha, sessões server-side e o serviço de
// autenticação. Nada aqui registra senha, hash, token ou cookie em log.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Erros de verificação de senha.
var (
	// ErrHashInvalido indica hash armazenado em formato irreconhecível.
	ErrHashInvalido = errors.New("hash de senha em formato inválido")
	// ErrSenhaFraca indica senha abaixo da política mínima.
	ErrSenhaFraca = errors.New("senha fora da política mínima")
)

// MinPasswordLength é o mínimo aceito. Frases longas são preferíveis a regras
// de composição, então a política é comprimento.
const MinPasswordLength = 12

// maxPasswordLength evita que uma entrada enorme vire custo de CPU.
const maxPasswordLength = 1024

// Argon2Params são os parâmetros do Argon2id. Ficam gravados dentro do hash,
// então é possível endurecê-los depois sem invalidar as senhas existentes.
type Argon2Params struct {
	// Memory em KiB.
	Memory uint32
	// Time é o número de iterações.
	Time uint32
	// Parallelism é o número de lanes.
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params segue a recomendação de uso interativo do RFC 9106
// (variante com menos memória): 64 MiB, 3 passes, 2 lanes.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{
		Memory:      64 * 1024,
		Time:        3,
		Parallelism: 2,
		SaltLength:  16,
		KeyLength:   32,
	}
}

// Hasher calcula e verifica hashes Argon2id.
type Hasher struct {
	params Argon2Params
}

// NewHasher devolve um Hasher com os parâmetros informados.
func NewHasher(params Argon2Params) *Hasher { return &Hasher{params: params} }

// NewDefaultHasher devolve um Hasher com os parâmetros padrão.
func NewDefaultHasher() *Hasher { return NewHasher(DefaultArgon2Params()) }

// ValidatePassword aplica a política mínima de senha.
func ValidatePassword(senha string) error {
	if utf8.RuneCountInString(senha) < MinPasswordLength {
		return fmt.Errorf("%w: mínimo de %d caracteres", ErrSenhaFraca, MinPasswordLength)
	}
	if len(senha) > maxPasswordLength {
		return fmt.Errorf("%w: máximo de %d bytes", ErrSenhaFraca, maxPasswordLength)
	}
	if strings.TrimSpace(senha) == "" {
		return fmt.Errorf("%w: senha não pode ser só espaços", ErrSenhaFraca)
	}
	return nil
}

// Hash devolve o hash no formato PHC, com os parâmetros embutidos.
func (h *Hasher) Hash(senha string) (string, error) {
	if !validParams(h.params) || h.params.SaltLength == 0 || h.params.KeyLength == 0 {
		return "", ErrHashInvalido
	}
	if err := ValidatePassword(senha); err != nil {
		return "", err
	}

	sal := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(sal); err != nil {
		return "", fmt.Errorf("gerar sal: %w", err)
	}

	chave := argon2.IDKey([]byte(senha), sal, h.params.Time, h.params.Memory, h.params.Parallelism, h.params.KeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, h.params.Memory, h.params.Time, h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(sal),
		base64.RawStdEncoding.EncodeToString(chave),
	), nil
}

// Verify confere a senha contra o hash armazenado.
//
// O segundo retorno indica que o hash usa parâmetros mais fracos que os atuais
// e deveria ser recalculado no próximo login bem-sucedido.
func (h *Hasher) Verify(hashArmazenado, senha string) (ok bool, precisaRehash bool, err error) {
	if len(senha) > maxPasswordLength {
		return false, false, ErrSenhaFraca
	}
	params, sal, chaveEsperada, err := decodeHash(hashArmazenado)
	if err != nil {
		return false, false, err
	}

	chave := argon2.IDKey([]byte(senha), sal, params.Time, params.Memory, params.Parallelism, params.KeyLength)
	if subtle.ConstantTimeCompare(chave, chaveEsperada) != 1 {
		return false, false, nil
	}
	return true, h.parametrosDesatualizados(params), nil
}

func (h *Hasher) parametrosDesatualizados(usados Argon2Params) bool {
	return usados.Memory < h.params.Memory ||
		usados.Time < h.params.Time ||
		usados.Parallelism < h.params.Parallelism ||
		usados.KeyLength < h.params.KeyLength ||
		usados.SaltLength < h.params.SaltLength
}

func decodeHash(hashArmazenado string) (Argon2Params, []byte, []byte, error) {
	if len(hashArmazenado) > 512 {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}
	partes := strings.Split(hashArmazenado, "$")
	if len(partes) != 6 || partes[0] != "" || partes[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}

	if partes[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}

	fields := strings.Split(partes[3], ",")
	if len(fields) != 3 || !strings.HasPrefix(fields[0], "m=") || !strings.HasPrefix(fields[1], "t=") || !strings.HasPrefix(fields[2], "p=") {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}
	m, e1 := strconv.ParseUint(strings.TrimPrefix(fields[0], "m="), 10, 32)
	t, e2 := strconv.ParseUint(strings.TrimPrefix(fields[1], "t="), 10, 32)
	p, e3 := strconv.ParseUint(strings.TrimPrefix(fields[2], "p="), 10, 8)
	if e1 != nil || e2 != nil || e3 != nil {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}
	params := Argon2Params{Memory: uint32(m), Time: uint32(t), Parallelism: uint8(p)}
	if !validParams(params) {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}

	sal, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil || len(sal) < 8 || len(sal) > 64 {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}
	chave, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil || len(chave) < 16 || len(chave) > 64 {
		return Argon2Params{}, nil, nil, ErrHashInvalido
	}

	params.SaltLength = uint32(len(sal))
	params.KeyLength = uint32(len(chave))
	return params, sal, chave, nil
}

func validParams(p Argon2Params) bool {
	return p.Memory >= 1024 && p.Memory <= 128*1024 && p.Time >= 1 && p.Time <= 6 && p.Parallelism >= 1 && p.Parallelism <= 4 &&
		p.SaltLength <= 64 && p.KeyLength <= 64 && (p.SaltLength == 0 || p.SaltLength >= 8) && (p.KeyLength == 0 || p.KeyLength >= 16)
}
