// Package secrets cifra e decifra valores sensíveis em repouso.
//
// Usa AES-256-GCM da biblioteca padrão — criptografia autenticada, algoritmo
// consolidado. Nada de algoritmo próprio.
//
// A chave mestra vem do ambiente (PACS_MASTER_KEY): não fica no PostgreSQL, não
// vai para o frontend, não é hardcoded, não entra no Git e não aparece em log.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Erros de cifragem.
var (
	// ErrSemChave indica que a chave mestra não foi configurada.
	ErrSemChave = errors.New("chave mestra não configurada")
	// ErrChaveInvalida indica chave em formato ou tamanho inválido.
	ErrChaveInvalida = errors.New("chave mestra inválida")
	// ErrValorCorrompido indica que o valor cifrado não pôde ser aberto.
	ErrValorCorrompido = errors.New("valor cifrado inválido ou chave trocada")
)

// TamanhoChave é o tamanho exigido da chave mestra: 32 bytes (AES-256).
const TamanhoChave = 32

// Cipher cifra e decifra com a chave mestra.
type Cipher struct {
	aead cipher.AEAD
}

// ParseKey lê a chave mestra de um texto em base64 (padrão ou URL-safe) ou hex.
// Exige exatamente 32 bytes depois de decodificar.
func ParseKey(bruto string) ([]byte, error) {
	bruto = strings.TrimSpace(bruto)
	if bruto == "" {
		return nil, ErrSemChave
	}

	decodificadores := []func(string) ([]byte, error){
		base64.StdEncoding.DecodeString,
		base64.RawStdEncoding.DecodeString,
		base64.URLEncoding.DecodeString,
		base64.RawURLEncoding.DecodeString,
		hex.DecodeString,
	}
	for _, decodificar := range decodificadores {
		if chave, err := decodificar(bruto); err == nil && len(chave) == TamanhoChave {
			return chave, nil
		}
	}
	return nil, fmt.Errorf("%w: precisa ser %d bytes em base64 ou hex", ErrChaveInvalida, TamanhoChave)
}

// NewCipher monta o Cipher a partir da chave já decodificada.
func NewCipher(chave []byte) (*Cipher, error) {
	if len(chave) != TamanhoChave {
		return nil, fmt.Errorf("%w: esperado %d bytes, recebido %d", ErrChaveInvalida, TamanhoChave, len(chave))
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrChaveInvalida)
	}
	aead, err := cipher.NewGCM(bloco)
	if err != nil {
		return nil, fmt.Errorf("%w", ErrChaveInvalida)
	}
	return &Cipher{aead: aead}, nil
}

// NewCipherFromString monta o Cipher direto do texto da chave.
func NewCipherFromString(bruto string) (*Cipher, error) {
	chave, err := ParseKey(bruto)
	if err != nil {
		return nil, err
	}
	return NewCipher(chave)
}

// Seal cifra o valor e devolve base64(nonce || texto cifrado).
//
// O contexto (`contexto`) entra como dado autenticado adicional: um valor
// cifrado para um propósito não pode ser reaproveitado em outro.
func (c *Cipher) Seal(valor, contexto string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("gerar nonce: %w", err)
	}
	cifrado := c.aead.Seal(nonce, nonce, []byte(valor), []byte(contexto))
	return base64.StdEncoding.EncodeToString(cifrado), nil
}

// Open decifra um valor produzido por Seal com o mesmo contexto.
func (c *Cipher) Open(cifradoBase64, contexto string) (string, error) {
	bruto, err := base64.StdEncoding.DecodeString(cifradoBase64)
	if err != nil {
		return "", ErrValorCorrompido
	}
	tamanhoNonce := c.aead.NonceSize()
	if len(bruto) < tamanhoNonce+1 {
		return "", ErrValorCorrompido
	}
	aberto, err := c.aead.Open(nil, bruto[:tamanhoNonce], bruto[tamanhoNonce:], []byte(contexto))
	if err != nil {
		return "", ErrValorCorrompido
	}
	return string(aberto), nil
}

// GenerateKey sorteia uma chave mestra nova, em base64 — para o administrador
// gerar a PACS_MASTER_KEY sem inventar valor à mão.
func GenerateKey() (string, error) {
	chave := make([]byte, TamanhoChave)
	if _, err := rand.Read(chave); err != nil {
		return "", fmt.Errorf("gerar chave: %w", err)
	}
	return base64.StdEncoding.EncodeToString(chave), nil
}
