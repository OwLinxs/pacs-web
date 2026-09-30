# Administração V1 — Etapa 1: Unidades

Estado informado pelo responsável: **implantada e validada em ambiente real**.
Este documento mantém o histórico da Etapa 1. A Etapa 2 está documentada em
[USERS.md](USERS.md); ela introduz os vínculos muitos-para-muitos e gerenciamento
de usuários que ainda eram futuros no texto abaixo.

## Escopo e arquitetura

`Administração → Unidades → API autenticada → internal/units → PostgreSQL da aplicação`.
Unidades são entidades próprias; somente ADMIN as administra. GESTOR apenas
utilizará unidades existentes futuramente. Médicos poderão futuramente pertencer
a múltiplas unidades, mas **nenhuma associação usuário/unidade foi implementada
nesta etapa**. Não há `user_units`, gerenciamento de usuários, validade de contas,
redefinição de senha nem regra nova de acesso a exames.

Worklist V2, Viewer V4, Invert, Orthanc, DICOM e infraestrutura permanecem intactos.
Nenhum exemplo de unidade é cadastrado automaticamente. Sem exclusão física:
unidades são desativadas e podem ser reativadas.

## Compatibilidade com o banco existente

A migration `0001_init.sql` já continha a tabela `units`, incluindo `slug`, `kind`
e referências legadas em `users.unit_id` e `audit_events.unit_id`. Ela não foi
editada, pois o runner verifica checksum. Essas colunas e referências permanecem
intactas; nenhum usuário, vínculo, ID, hash ou sessão é alterado.

A nova `0002_units_management.sql`:

- verifica nomes legados inválidos e duplicados sem listar seus valores nos erros;
- acrescenta CHECK de nome presente, até 120 caracteres e sem controles;
- cria índice único `units_name_ci_unique` em `lower(btrim(name))`, abrangendo
  unidades ativas **e inativas**;
- roda na transação do runner existente e bloqueia escritas concorrentes na tabela
  durante a verificação/criação das constraints;
- não renomeia, mescla, exclui ou desativa dados legados automaticamente.

Se houver duplicatas ou nomes inválidos, a migration falha e faz rollback.
É necessária revisão explícita dos registros antes de tentar novamente; não há
correção automática. Unicidade segue `lower`/collation do PostgreSQL, sem prometer
comparação sem acentos. A aplicação remove espaços nas extremidades do nome,
rejeita controles, nome vazio/só espaços e mais de 120 caracteres Unicode.

Os campos públicos são `id`, `name`, `active`, `createdAt`, `updatedAt`.
`id` é UUID; novas unidades são ativas. Timestamps são `timestamptz` gerados pelo
banco: criação usa os defaults existentes e alteração efetiva atualiza
`updated_at` pelo relógio do banco. PATCH sem mudança efetiva preserva timestamps.

Para compatibilidade com as colunas obrigatórias legadas, novas unidades recebem
`slug` técnico `unit-<uuid>` e `kind=OUTRO`. Esses valores não são campos novos da
UI/API, não classificam automaticamente a unidade como um tipo clínico específico
e não mudam ao renomear. Unidades existentes preservam seu slug/kind.

O servidor já aplica migrations na inicialização. **Nenhuma migration foi
executada em banco existente nesta entrega**, nem houve deploy. O índice e o
CHECK precisam estar aplicados quando essa versão for utilizada.

## API

Todas as rotas exigem sessão. POST/PATCH mantêm cookie + header CSRF existentes.
PATCH foi incluído na lista CORS de métodos permitidos para as origens já
configuradas; não foram liberadas novas origens nem alteradas regras de sessão.
Respostas mantêm `Cache-Control: no-store` e o envelope padrão de erro.

| Rota | Autorização | Comportamento |
| --- | --- | --- |
| `GET /api/units` | ADMIN, GESTOR, MEDICO | Apenas ativas por padrão |
| `GET /api/units?includeInactive=true` | ADMIN | Ativas e inativas |
| `POST /api/admin/units` | ADMIN | Corpo `{ "name": "..." }`, resposta 201 com unidade |
| `PATCH /api/admin/units/{id}` | ADMIN | `name` e/ou `active`, resposta 200 com unidade |

GET aceita `includeInactive=true/false`, `limit=1..100` (padrão 50) e
`offset=0..10000` (padrão zero). Parâmetros desconhecidos/repetidos são recusados.
Ordenação global por nome normalizado e ID. Resposta:

```ts
{
  items: Array<{
    id: string;
    name: string;
    active: boolean;
    createdAt: string;
    updatedAt: string;
  }>;
  limit: number;
  offset: number;
  hasMore: boolean;
  nextOffset: number | null;
}
```

Consulta lê somente a página e uma sentinela; não exige contagem global.
Ao atingir offset máximo, hasMore pode ser true e nextOffset null.

POST/PATCH limitam o corpo a 4 KiB, recusam campos desconhecidos, null, tipos
incorretos, JSON extra e PATCH vazio. Não aceitam slug/kind/usuários/timestamps.
PATCH valida UUID e altera apenas campos fornecidos. O store usa queries
parametrizadas e bloqueio de linha na transação para não sobrescrever um campo
omitido sob concorrência. A unicidade é garantida no banco, não só por consulta
prévia ou frontend. Não existe DELETE.

Erros: 400 entrada inválida; 401 sessão ausente; 403 perfil/CSRF sem permissão;
404 unidade ausente; 409 nome duplicado, inclusive de inativa; 503 indisponibilidade.
Não retorna SQL, mensagens internas do PostgreSQL ou conteúdo de credenciais.
Timeout de cinco segundos para operações de dados.

## Auditoria

Reutiliza `AuditRecorder` e `audit_events` existentes:

- `UNIT_CREATED`: criação;
- `UNIT_UPDATED`: alteração efetiva de nome;
- `UNIT_ACTIVATED`: false → true;
- `UNIT_DEACTIVATED`: true → false.

Registra o ADMIN responsável, origem e ID administrativo da unidade. Detail é
texto operacional fixo, sem valores do formulário. Uma requisição que muda nome
e status produz os dois eventos correspondentes; PATCH sem mudança não gera
novo evento. Recusas/erros não geram evento de sucesso.

Atualização Auditoria V1: o store grava alteração e eventos na mesma transação.
Falha de auditoria obrigatória reverte a operação. A política best-effort era
o comportamento da Etapa 1; foi substituída conforme [AUDIT.md](AUDIT.md).

## Interface

Item Unidades aparece na seção Administração somente para ADMIN. Há uma segunda
verificação de perfil ao renderizar a tela; a proteção real continua no backend.
GESTOR/MEDICO não recebem controles administrativos. Outras telas administrativas
já existentes não foram implementadas/refatoradas nesta entrega.

Lista compacta com Nome, Status e Ações, incluindo inativas. Formulário inline
para criar/editar nome, status de salvamento, erros sanitizados, confirmação de
desativação e reativação. Paginação 50 por página. Sem gráficos ou estatísticas.
Requests de leitura e escrita têm AbortController; cleanup evita atualizações
após unmount. Controles ficam bloqueados durante gravação, evitando submissão
duplicada. 401 segue a tela de sessão expirada existente.

## Testes e validação

- Go/httptest: ciclo ADMIN completo; rejeição de GESTOR/MEDICO/outros e sem sessão;
  CSRF; nomes inválidos; unicidade case-insensitive incluindo inativas;
  listagem ativa/admin; UUID/patch/query inválidos; auditoria e ausência de DELETE.
- Modelo/store: normalização Unicode, limites e sanitização de erros SQL.
- `TestStoreAndMigrationPostgres`: cria cluster descartável próprio, somente socket
  Unix, quando `postgres` e `initdb` estão disponíveis; nunca lê DATABASE_URL/.env
  nem conecta a serviços existentes. Cobre rollback com legado inválido/duplicado,
  preservação de referências/usuários, aplicação repetida, store PostgreSQL e
  unicidade sob concorrência. Sem binários de servidor, retorna SKIP explícito.
- Node: cliente GET/POST/PATCH, CSRF, cookies, campos, ID inválido e erros.
- Chrome (`tests/units-browser.mjs`): criar, editar, nome vazio, duplicidade,
  confirmar/cancelar desativação, reativar, retry, expiração e menu por perfil.
- Regressão dos testes existentes de backend/frontend, Worklist e Viewer.

Fixtures exclusivamente sintéticas. Não há acesso a Orthanc real nem a banco
existente. Dependências não foram alteradas e não foi executado npm audit fix.
Warnings existentes de chunks e módulos Node externalizados do Viewer permanecem.

Comandos:

```sh
cd backend
gofmt -l $(rg --files -g '*.go')
go vet ./...
go test ./...
go build ./...
go test -race ./internal/units ./internal/httpapi ./internal/orthanc
go test -run TestStoreAndMigrationPostgres -v ./internal/units
cd ../frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
node tests/units-browser.mjs
node tests/worklist-browser.mjs
node tests/viewer-smoke.mjs
```

### Resultado da validação desta entrega

- gofmt sem diferenças; go vet, go test, go build e race tests aprovados.
- Typecheck e build frontend aprovados; 26 testes Node aprovados.
- Chrome de Unidades e Worklist aprovado, com API sintética local.
- Integração SQL/migration: **SKIP**, porque o executável servidor `postgres`
  não está instalado. Não foi utilizado banco existente para contornar isso.
- Smoke completo do Viewer: V2/V3/V4 passaram até as verificações finais; houve
  falha na asserção de Invert numa execução e timeout na simulação de sessão
  expirada em outra. A cópia anterior à Administração passou na comparação.
  O smoke foi ajustado somente para aguardar a toolbar habilitada e a propagação
  do estado de apresentação pelo effect React antes de clicar/afirmar valores.
  Nenhuma implementação do Viewer ou comportamento de Invert foi alterado.
- Após corrigir a sincronização do smoke, a regressão completa do build atual
  passou: Viewer V2/V3/V4, Invert OFF e sessão expirada. O ajuste é exclusivo do
  teste; não introduz sleeps fixos nem alterações no Viewer.
