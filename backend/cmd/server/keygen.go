package main

import (
	"fmt"
	"os"

	"github.com/pmfb-saude/pacs-web/backend/internal/secrets"
)

// executarKeygen sorteia uma chave mestra nova e a imprime na saída padrão.
//
// A chave sai só aqui: guarde-a no gerenciador de segredos da Prefeitura e
// coloque-a no ambiente como PACS_MASTER_KEY. Não vai para o banco, para o Git,
// para a documentação nem para log.
//
// Trocar a chave torna ilegíveis as credenciais já cifradas — elas precisam ser
// cadastradas de novo.
func executarKeygen() error {
	chave, err := secrets.GenerateKey()
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, chave)
	fmt.Fprintln(os.Stderr, "Guarde este valor como PACS_MASTER_KEY. Ele não é recuperável.")
	return nil
}
