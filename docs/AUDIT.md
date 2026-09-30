# Auditoria V1

**VALIDADA EM AMBIENTE REAL**, conforme confirmação posterior do responsável,
incluindo migration 0004. Viewer V5 acrescenta localmente um evento específico de
exportação clínica; essa extensão ainda não foi validada em ambiente real.

## Objetivo e arquitetura

`Administração → Auditoria → GET /api/admin/audit → internal/audit → PostgreSQL`.
Consulta histórica somente para ADMIN. GESTOR/MEDICO recebem 403; sessão ausente
recebe 401; troca obrigatória de senha continua bloqueando a API. O menu e a tela
possuem verificações de perfil, mas a autorização efetiva é no backend.

Não há edição, exclusão, exportação, gráficos, limpeza, retenção automática nem
endpoints de escrita da auditoria. A primitiva antiga e não utilizada `Purge`
foi removida. Políticas de retenção e privilégios PostgreSQL são decisões futuras;
a V1 não oferece proteção contra adulteração por um administrador direto do banco.

## Modelo e histórico

Reutiliza `audit_events` de 0001, sem novas colunas:

- `id` bigserial, exposto como string para não perder precisão no JavaScript;
- `occurred_at` timestamptz, gerado pelo banco;
- `event`, valor técnico original;
- `actor_user_id`, referência administrativa;
- `actor_username`, cópia mínima somente para ator identificado;
- `unit_id`, alvo dos eventos de unidade (legado também pode representar contexto);
- `detail`, vocabulário fechado, nunca JSON arbitrário;
- `origin`, IP válido, sem headers ou identificação de sessão.

Alvos de usuário usam o formato existente `target_user_id=<UUID canônico>` no
campo detail. A API transforma isso em referência estruturada; não exibe o texto
bruto. Não há snapshots de usuário, e-mail, unidades vinculadas ou configuração.

LEFT JOIN resolve nome/username atuais de ator e alvo e nome atual da unidade,
inclusive inativos. A interface identifica essa resolução como cadastro atual:
renomear um cadastro altera seu rótulo na consulta, não o evento. A cópia histórica
de username é fallback para ator ainda referenciado. Nenhum evento depende de
INNER JOIN para existir. Se uma referência for removida futuramente, o evento
permanece; IDs de usuário alvo no detail continuam disponíveis. FKs existentes
com ON DELETE SET NULL podem perder ID do ator/unidade: sem inventar snapshot ou
recuperação retroativa, a UI indica referência indisponível. Não há DELETE de
usuário/unidade nas APIs atuais.

Login falho sem ator identificado não grava mais o username tentado: texto livre
nesse campo poderia ser uma senha colada por engano. Valores legados desse caso
não são devolvidos. Não se altera retroativamente o banco.

## Inventário dos 17 eventos

| Categoria | Eventos preservados |
| --- | --- |
| Unidades | UNIT_CREATED, UNIT_UPDATED, UNIT_ACTIVATED, UNIT_DEACTIVATED |
| Usuários | USER_CREATED, USER_UPDATED, USER_ACTIVATED, USER_DEACTIVATED, USER_ACCESS_RENEWED, USER_PASSWORD_RESET, USER_PASSWORD_CHANGED, USER_UNITS_CHANGED |
| Autenticação | LOGIN_SUCCESS, LOGIN_FAILURE, LOGOUT |
| Configuração | ORTHANC_SETTINGS_CHANGED, ORTHANC_CONNECTION_TESTED |

Categorias são derivadas no código, sem alterar valores persistidos. Labels
amigáveis ficam no frontend. Resultado é derivado do tipo e do detalhe permitido:
eventos administrativos/autenticação concluída = success; LOGIN_FAILURE = failure;
teste de conexão usa a categoria de resultado fechada, sem resposta upstream.
Evento desconhecido é apresentado como UNKNOWN/resultado unknown, sem texto livre.
Não se cria evento administrativo de sucesso para operação recusada ou revertida.

## Atomicidade

Os stores chamam `audit.RecordTx(ctx, tx, entry)` **antes do commit**, utilizando
a mesma transação pgx da alteração. Não há outbox/transação distribuída.

| Operação | Garantia |
| --- | --- |
| Criar/editar/ativar/desativar unidade | Alteração e todos os eventos atômicos |
| Criar/editar usuário e vínculos | Usuário, user_units e todos os eventos atômicos |
| Ativar/desativar/renovar usuário | Alteração e eventos atômicos; revogação de sessões incluída na desativação |
| Reset administrativo de senha | Hash/flag, revogação de sessões e evento atômicos |
| Troca da própria senha | Hash/flag, revogação de sessões e evento atômicos |
| Salvar configuração PACS | Escrita em app_settings e evento atômicos, sem conexão Orthanc |

Falha de qualquer inserção obrigatória faz rollback, inclusive de eventos já
inseridos na mesma operação. Erros devolvidos são fixos, sem erro SQL/valores.
Os handlers não emitem uma segunda cópia após o commit. Operações sem mudança
não emitem eventos de alteração, preservando comportamento de Unidades/Usuários.
Salvar configuração continua sendo ação explícita; seu contexto agora é fixo
“configuração atualizada”, sem lista de campos/valores potencialmente sensíveis.

A atribuição de origem/ator de unidades vem de contexto tipado criado pelo
middleware autenticado; não vem de JSON ou query. Usuários continuam revalidando
ator/alvo na transação. Stores reais exigem registro obrigatório, independentemente
do Recorder opcional usado pelos fluxos não transacionais da API.

### Eventos não transacionais

LOGIN_SUCCESS, LOGIN_FAILURE, LOGOUT e ORTHANC_CONNECTION_TESTED permanecem
best-effort, com erro registrado de forma fixa e prazo de dois segundos,
independente do cancelamento da requisição. Não são silenciosamente ignorados
no logger, mas falha de persistência ainda pode perder um evento.

Justificativa: login falho não tem alteração administrativa para reverter;
login/logout mantêm o ciclo atual de sessões; o teste de conexão representa uma
operação externa que não se desfaz com rollback PostgreSQL. Não tornar logout
incapaz de revogar uma sessão porque a auditoria está indisponível. O status do
teste e seu evento não são uma transação única; resultado registra comunicação,
não garante gravação do status. Logout ainda não propaga IP. Melhorias futuras
nesses fluxos devem ser uma decisão explícita, sem afirmar entrega garantida.

## Política de conteúdo seguro

`policy.go` aplica allowlist na escrita **e na leitura de registros legados**:

- tipos de evento fechados;
- alvo de usuário: somente UUID canônico, não zero;
- unidade: ID e frases operacionais fixas;
- autenticação: apenas motivos categorizados predefinidos;
- configuração: somente termos predefinidos; novas gravações usam frase fixa;
- teste PACS: apenas códigos operacionais conhecidos;
- origem: somente endereço IP analisável;
- username: somente ator identificado e formato administrativo limitado.

Detail não reconhecido é rejeitado na escrita e omitido na leitura, sem alterar
o evento legado. Não há busca livre em detail, JSON, origem ou conteúdo clínico.
O frontend também só apresenta descrições permitidas, sem despejar JSON.

Proibidos: senhas/hash, tokens/cookies/CSRF/Authorization, secrets, configurações
de conexão, credenciais PACS, conteúdo de .env, DICOM, nomes/IDs de pacientes,
accession e UIDs clínicos. Falhas da auditoria não registram erro SQL bruto.
Nomes administrativos são resolvidos do cadastro; nenhum e-mail é incluído.

## API e filtros

`GET /api/admin/audit`, sessão ADMIN, no-store. Não aceita POST/PATCH/PUT/DELETE.
Filtros combinados por AND, query até 1024 bytes, desconhecidos/repetidos/vazios
rejeitados. Sem parâmetros SQL ou caminhos arbitrários.

| Parâmetro | Regra |
| --- | --- |
| limit | 1–100, padrão 50 |
| offset | 0–10000, padrão 0 |
| dateFrom/dateTo | AAAA-MM-DD válidas, anos 1900–9998, início ≤ fim |
| actorId | UUID administrativo canônico, não zero |
| targetUserId | UUID canônico do usuário afetado por evento USER_* |
| event | Um dos 17 eventos conhecidos |
| category | users, units, auth, settings |

Período representa dias inclusivos em America/Sao_Paulo: início à meia-noite,
fim exclusivo à meia-noite seguinte. PostgreSQL compara instantes timestamptz;
frontend usa Intl com o mesmo fuso, sem alterar o timestamp original.
Limites abertos são permitidos. Por padrão não há restrição de período.

Resposta:

```ts
{
  items: Array<{
    id: string;
    occurredAt: string; // timestamp RFC3339
    event: string;
    category: string;
    actor: { id: string; name: string; username: string; kind: 'user' } | null;
    target: { id: string; name: string; username: string; kind: 'user' | 'unit' } | null;
    result: 'success' | 'failure' | 'unknown';
    detail: string; // somente vocabulário permitido
    origin: string; // IP ou vazio
  }>;
  limit: number;
  offset: number;
  hasMore: boolean;
  nextOffset: number | null;
}
```

ORDER BY occurred_at DESC, id DESC no banco, limit+1 como sentinela. Sem contagem
global nem carga de toda a tabela. Timeout de cinco segundos e queries
parametrizadas. 400 para filtros inválidos, 401/403 autorização, 503 sanitizado
para indisponibilidade. IDs sem correspondência retornam página vazia.

Offset segue a convenção administrativa atual: novos eventos entre consultas
podem deslocar páginas. Atualizar volta à primeira página, conservando filtros.
Ao atingir limite de offset, refinar período/ator/ação; não se promete snapshot
estável ou exportação integral. Cursor pode ser avaliado futuramente.

## Índices e migration

0001–0003 permanecem intactas. `0004_audit_query.sql` adiciona somente
`audit_events_target_user_idx (detail, occurred_at DESC, id DESC)` parcial para
`detail LIKE 'target_user_id=%'`, condição também explícita na consulta por alvo.
Evita varredura textual genérica para esse filtro novo. Índices existentes de
data, ator e evento são reutilizados; não adicionados índices redundantes em tudo.

Runner continua forward-only/transacional/checksum. Na entrega original, 0004 foi
testada em PostgreSQL 18.6 descartável; foi aplicada e validada posteriormente no
ambiente real conforme confirmação do responsável. Migrations aplicadas são imutáveis.

## Interface

Tabela administrativa compacta, filtros explícitos com Aplicar/Limpar, data,
ação/categoria, busca paginada de ator/alvo pelo cadastro atual. Nenhum UUID precisa
ser digitado para selecionar usuário existente. Usuários já removidos não estão
nesse seletor, mas eventos continuam na lista e IDs conhecidos podem filtrar via
API. Campos de busca são administrativos, não clínicos.

Loading/refreshing, vazio, erro/retry, sessão expirada, anterior/próxima e detalhes
por expansão. Sem total fictício, CSV, gráficos ou edição. AbortController e
guarda de resultado obsoleto; cleanup ao desmontar. Filtros ficam só em memória
React, sem localStorage ou URL de navegação. Query do fetch é visível ao browser;
logger existente omite query, proxies devem seguir a política de sanitização.

## Testes e validação

Fixtures exclusivamente sintéticas:

- Go: inventário/allowlist, filtros/datas/limites/injeção, sessão e matriz ADMIN /
  GESTOR / MEDICO, ausência de rotas de escrita, erros sanitizados.
- PostgreSQL descartável: migration/checksum, paginação/ordem, filtros AND,
  referências inativas/renomeadas/removidas, ocultação de texto legado; falha
  obrigatória de auditoria reverte usuários/unidades/sessões/vínculos/configuração,
  incluindo falha no segundo evento da operação.
- Node: contrato autenticado, cancelamento, labels, fuso, metadata segura e erros.
- Chrome: navegação/menu, filtros, ator/alvo, paginação, detalhes, vazio, retry,
  concorrência, unmount e expiração; regressões Usuários/Unidades/Worklist/Viewer.

Os testes PostgreSQL só criam clusters temporários por socket Unix, sem ler .env
ou DATABASE_URL; requerem postgres/initdb no PATH e sinalizam SKIP se ausentes.
Não confundir SKIP com validação SQL. Warnings preexistentes de bundles grandes
e módulos Node externalizados pelo Cornerstone não foram corrigidos. Nenhuma
dependência alterada, nenhum audit fix executado. Vulnerabilidades npm não foram
reavaliadas nesta entrega; permanecem no plano de hardening.

## Resultado local e arquivos da entrega

Aprovados: gofmt (sem diferenças), go vet, go test ./..., go build e race tests
em audit/auth/httpapi/units/useradmin/settings. PostgreSQL 18.6 descartável executou
os testes SQL (não SKIP). Frontend typecheck/build, 31 testes Node, Chrome Auditoria,
Usuários, Unidades, Worklist e smoke Viewer V2/V3/V4, incluindo Invert OFF, passaram.
Tela de Auditoria também revisada por screenshot sintético. Sem dependências novas.

A documentação anterior descrevia best-effort administrativo e tela mockada;
isso foi substituído nesta V1. O menu antigo também aparecia para GESTOR e agora
é exclusivo de ADMIN. As demais divergências históricas estão no PROJECT_CONTEXT.
Esse documento já tinha atualização pendente antes desta entrega, preservada aqui.

Arquivos criados:

- `backend/internal/audit/policy.go`
- `backend/internal/audit/policy_test.go`
- `backend/internal/audit/query.go`
- `backend/internal/httpapi/audit_handler.go`
- `backend/internal/httpapi/audit_test.go`
- `backend/internal/useradmin/audit_postgres_test.go`
- `backend/migrations/0004_audit_query.sql`
- `docs/AUDIT.md`
- `frontend/src/audit/model.ts`
- `frontend/tests/audit-browser.mjs`
- `frontend/tests/audit.test.mjs`

Arquivos modificados:

- `backend/cmd/server/serve.go`
- `backend/internal/audit/audit.go`
- `backend/internal/httpapi/api.go`
- `backend/internal/httpapi/api_test.go`
- `backend/internal/httpapi/middleware.go`
- `backend/internal/httpapi/settings_handlers.go`
- `backend/internal/httpapi/settings_test.go`
- `backend/internal/httpapi/units_handlers.go`
- `backend/internal/httpapi/units_test.go`
- `backend/internal/httpapi/users_handlers.go`
- `backend/internal/httpapi/users_test.go`
- `backend/internal/settings/store.go`
- `backend/internal/units/store.go`
- `backend/internal/units/store_postgres_test.go`
- `backend/internal/useradmin/store.go`
- `backend/internal/useradmin/store_postgres_test.go`
- `docs/ADMINISTRATION.md`
- `docs/BACKEND.md`
- `docs/PROJECT_CONTEXT.md`
- `docs/USERS.md`
- `frontend/src/App.tsx`
- `frontend/src/api/client.ts`
- `frontend/src/app/AppShell.tsx`
- `frontend/src/screens/AuditoriaScreen.tsx`

## Extensão local do Viewer V5

`VIEWER_IMAGE_EXPORTED` é evento novo, ainda não validado no ambiente real. O
endpoint específico `POST /api/viewer/exports` valida formato PNG/JPEG/PDF e
booleano `identified`, atribui ator da sessão e registra apenas detalhe canônico
`format=<formato>;identified=<bool>`. Não recebe identificadores de paciente ou
DICOM. A consulta ADMIN mostra categoria Viewer e rótulo amigável. A chamada é
feita depois da composição do arquivo e antes do clique de download; a gravação
não comprova que o arquivo foi salvo. Falha de auditoria é sinalizada ao operador
após revalidação da sessão; não há transação distribuída com o browser.
Consulte [VIEWER_EXPORT.md](VIEWER_EXPORT.md).
