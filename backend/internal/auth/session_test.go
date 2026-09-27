package auth

import (
	"crypto/sha256"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestNewSessionTokenGeraHashCorrespondente(t *testing.T) {
	token, hash, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if len(token) < 40 {
		t.Errorf("token curto demais: %d caracteres", len(token))
	}

	esperado := sha256.Sum256([]byte(token))
	if string(hash) != string(esperado[:]) {
		t.Error("hash devolvido não corresponde ao SHA-256 do token")
	}
	if string(hash) == token {
		t.Error("o hash não pode ser igual ao token")
	}

	outroToken, _, err := NewSessionToken()
	if err != nil {
		t.Fatalf("NewSessionToken: %v", err)
	}
	if token == outroToken {
		t.Error("dois tokens sorteados não podem ser iguais")
	}
}

func TestSessionAlive(t *testing.T) {
	agora := time.Date(2026, 9, 26, 10, 0, 0, 0, time.UTC)
	revogada := agora.Add(-time.Minute)
	const idleTTL = 30 * time.Minute

	casos := []struct {
		nome     string
		sessao   Session
		esperada bool
	}{
		{
			nome: "ativa",
			sessao: Session{
				LastSeenAt:        agora.Add(-5 * time.Minute),
				AbsoluteExpiresAt: agora.Add(time.Hour),
			},
			esperada: true,
		},
		{
			nome: "revogada",
			sessao: Session{
				LastSeenAt:        agora,
				AbsoluteExpiresAt: agora.Add(time.Hour),
				RevokedAt:         &revogada,
			},
			esperada: false,
		},
		{
			nome: "prazo absoluto vencido",
			sessao: Session{
				LastSeenAt:        agora,
				AbsoluteExpiresAt: agora.Add(-time.Second),
			},
			esperada: false,
		},
		{
			nome: "inatividade estourada",
			sessao: Session{
				LastSeenAt:        agora.Add(-idleTTL),
				AbsoluteExpiresAt: agora.Add(time.Hour),
			},
			esperada: false,
		},
		{
			nome: "inatividade no limite",
			sessao: Session{
				LastSeenAt:        agora.Add(-idleTTL + time.Second),
				AbsoluteExpiresAt: agora.Add(time.Hour),
			},
			esperada: true,
		},
	}

	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			caso.sessao.ID = uuid.New()
			if viva := SessionAlive(caso.sessao, idleTTL, agora); viva != caso.esperada {
				t.Errorf("SessionAlive = %v, esperado %v", viva, caso.esperada)
			}
		})
	}
}
