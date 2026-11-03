// Package terms guarda o termo de responsabilidade e sigilo exibido no primeiro
// acesso e registra o aceite de cada usuário por versão do texto.
package terms

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pmfb-saude/pacs-web/backend/internal/audit"
)

// Version muda sempre que o texto mudar: todos precisarão aceitar de novo.
// Formato: ^[0-9a-z.-]{1,40}$ (validado também no banco e na auditoria).
const Version = "2026-10-31-rascunho"

// Draft indica texto ainda não validado pela Prefeitura/Procuradoria.
const Draft = true

type Section struct {
	Heading    string   `json:"heading"`
	Paragraphs []string `json:"paragraphs"`
}

type Document struct {
	Version  string    `json:"version"`
	Title    string    `json:"title"`
	Draft    bool      `json:"draft"`
	Sections []Section `json:"sections"`
}

// Current é o termo vigente. RASCUNHO: substituir pelo texto aprovado pela
// Procuradoria do Município e, ao trocar, alterar Version.
var Current = Document{
	Version: Version,
	Title:   "Termo de responsabilidade e sigilo",
	Draft:   Draft,
	Sections: []Section{
		{"Finalidade", []string{
			"O PACS Web dá acesso a imagens e informações de exames de pacientes atendidos pela rede municipal de saúde de Francisco Beltrão. Esses dados são pessoais sensíveis (Lei nº 13.709/2018 — LGPD, art. 5º, II) e são tratados pelo Município para execução de políticas públicas de saúde e para a tutela da saúde (art. 11, II, \"b\" e \"f\").",
			"O acesso é pessoal e concedido exclusivamente para o exercício das suas atribuições profissionais.",
		}},
		{"Compromissos", []string{
			"Acessar somente exames de pacientes sob meus cuidados ou necessários às minhas atribuições, e apenas pelo tempo necessário.",
			"Manter sigilo sobre todas as informações acessadas, inclusive após o término do vínculo com o Município, conforme o sigilo profissional e funcional aplicável.",
			"Não compartilhar meu usuário e senha, não deixar a sessão aberta em equipamento sem supervisão e encerrá-la ao terminar.",
			"Não copiar, fotografar, exportar ou repassar imagens e dados por meios não autorizados (por exemplo, aplicativos de mensagem ou e-mail pessoal). Exportações com identificação do paciente só para finalidade assistencial ou legal.",
			"Comunicar imediatamente à chefia e ao encarregado de dados do Município qualquer suspeita de acesso indevido, perda de credencial ou vazamento.",
		}},
		{"Registro e responsabilização", []string{
			"Todos os acessos, visualizações relevantes e exportações são registrados com usuário, data, hora e origem, e podem ser auditados.",
			"O uso indevido pode resultar em responsabilização administrativa, civil e penal, nos termos da legislação aplicável.",
		}},
		{"Ciência", []string{
			"Ao clicar em \"Li e concordo\", declaro que li este termo, compreendi minhas responsabilidades e me comprometo a cumpri-las. A data, a hora e a versão deste aceite ficam registradas.",
		}},
	},
}

var ErrUnavailable = errors.New("termo indisponível")

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Accepted informa se o usuário já aceitou a versão indicada.
func (s *Store) Accepted(ctx context.Context, userID uuid.UUID, version string) (bool, error) {
	var ok bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM terms_acceptances WHERE user_id=$1 AND version=$2)`, userID, version).Scan(&ok); err != nil {
		return false, ErrUnavailable
	}
	return ok, nil
}

// Accept registra o aceite e o evento de auditoria na mesma transação. Repetir
// o aceite da mesma versão não gera novo registro.
func (s *Store) Accept(ctx context.Context, userID uuid.UUID, version string, entry audit.Entry) error {
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `INSERT INTO terms_acceptances(user_id, version) VALUES($1,$2) ON CONFLICT DO NOTHING`, userID, version)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		return audit.RecordTx(ctx, tx, entry)
	})
	if err != nil {
		return ErrUnavailable
	}
	return nil
}
