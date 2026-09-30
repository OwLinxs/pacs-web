package httpapi

import (
	"sync"
	"time"
)

// rateLimiter é um contador de janela fixa por chave, em memória.
//
// Limitações conhecidas e aceitas nesta etapa:
//   - vale por processo: com mais de uma instância, o limite é por instância;
//   - zera ao reiniciar o servidor;
//   - a memória é limitada por maxEntradas; ao estourar, novas chaves são
//     negadas para que o limite nunca falhe aberto.
//
// Quando houver mais de uma instância, o caminho é mover o contador para o
// PostgreSQL ou para o proxy.
type rateLimiter struct {
	limite  int
	janela  time.Duration
	agora   func() time.Time
	maximo  int
	mu      sync.Mutex
	janelas map[string]*janelaContagem
}

type janelaContagem struct {
	inicio     time.Time
	tentativas int
}

const maxEntradasRateLimit = 10_000

func newRateLimiter(limite int, janela time.Duration, agora func() time.Time) *rateLimiter {
	if agora == nil {
		agora = time.Now
	}
	return &rateLimiter{
		limite:  limite,
		janela:  janela,
		agora:   agora,
		maximo:  maxEntradasRateLimit,
		janelas: make(map[string]*janelaContagem),
	}
}

// Allow contabiliza uma tentativa e diz se ela pode prosseguir. O segundo
// retorno é quanto falta para a janela reabrir.
func (rl *rateLimiter) Allow(chave string) (bool, time.Duration) {
	agora := rl.agora()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	atual, existe := rl.janelas[chave]
	if !existe || agora.Sub(atual.inicio) >= rl.janela {
		if len(rl.janelas) >= rl.maximo {
			rl.limparVencidasLocked(agora)
			if len(rl.janelas) >= rl.maximo {
				return false, rl.janela
			}
		}
		rl.janelas[chave] = &janelaContagem{inicio: agora, tentativas: 1}
		return true, 0
	}

	atual.tentativas++
	if atual.tentativas > rl.limite {
		return false, rl.janela - agora.Sub(atual.inicio)
	}
	return true, 0
}

// Reset limpa a contagem de uma chave. Chamado após login bem-sucedido, para
// que um acerto não deixe o usuário legítimo bloqueado.
func (rl *rateLimiter) Reset(chave string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.janelas, chave)
}

func (rl *rateLimiter) limparVencidasLocked(agora time.Time) {
	for chave, janela := range rl.janelas {
		if agora.Sub(janela.inicio) >= rl.janela {
			delete(rl.janelas, chave)
		}
	}
}

// Vacuum remove janelas vencidas. O servidor chama periodicamente.
func (rl *rateLimiter) Vacuum() {
	agora := rl.agora()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.limparVencidasLocked(agora)
}
