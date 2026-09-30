# PACS Web — contexto para continuidade

Documento principal de continuidade do **novo PACS Web da Prefeitura Municipal
de Francisco Beltrão**. Consolida código/documentação do repositório e o estado
operacional informado pelo responsável. As validações reais e migrations 0001–0003
são informações do responsável. A atualização Auditoria V1 descreve código/testes
locais, ainda sem deploy ou validação real; 0004 permanece pendente no ambiente.

## Estado funcional confirmado

| Entrega | Estado atual |
| --- | --- |
| Auditoria V1 | **IMPLEMENTADA**, sem deploy ou validação real; migration 0004 apenas em banco descartável |
| Worklist V2 | **VALIDADA EM AMBIENTE REAL**, com integração Orthanc e ExtendedFind disponível no Orthanc utilizado |
| Viewer V4 | **VALIDADO COM DICOM REAL**; Invert OFF inicial e após Reset |
| Administração V1 — Etapa 1: Unidades | **VALIDADA EM AMBIENTE REAL**; migration 0002 aplicada |
| Administração V1 — Etapa 2: Usuários | **VALIDADA EM AMBIENTE REAL**; backend, frontend, migration 0003 e fluxos implantados/testados com sucesso |

A Etapa 2 foi validada pelo responsável com **contas fictícias**: login do ADMIN
existente, listagem, criação de MEDICO, múltiplas unidades, edição, validade,
renovação, ativação/desativação, criação de GESTOR, troca obrigatória, novo login
após troca, reset, revogação de sessão, permissões do GESTOR, acesso do MEDICO e
regressão básica Worklist V2 → Viewer V4. Não registrar os dados dessas contas.
Validação funcional não equivale a concluir hardening ou autorizar substituição
definitiva do sistema atual.

## Arquitetura e localização do código

`Browser React/TypeScript → API Go autenticada → Orthanc REST`.
O PostgreSQL da aplicação guarda usuários, sessões, auditoria, unidades e
configurações; permanece logicamente separado do banco interno do Orthanc.
Orthanc continua sendo o PACS/DICOM e mantém o armazenamento DICOM fora da
aplicação; o gateway não duplica arquivos persistentemente. O backend comunica-se
internamente com Orthanc. O navegador não recebe seu endereço interno nem credenciais.
Autenticação própria, sem Keycloak. Preservar o design existente, Worklist clara
e Viewer escuro, sem redesenho ou refatorações fora do escopo autorizado.

- `backend/cmd/server`: composição e comandos serve, migrate, admin create e keygen.
- `backend/internal/httpapi`: rotas, DTOs, autorização, CSRF e erros HTTP.
- `auth`, `user`, `audit`: senha/sessões/login, usuários/perfis, registro de eventos.
- `database`, `config`, `settings`, `secrets`: pgx/migrations, ambiente,
  configuração persistida e criptografia de credenciais.
- `useradmin`: serviço/validação/autorização e store transacional de usuários.
- `orthanc`: transporte e consultas explícitas; `studies`, `viewer`, `units`:
  contratos/validações e, para unidades, store PostgreSQL.
- `frontend/src/App.tsx`, `auth/SessionProvider.tsx`, `api/client.ts`:
  navegação/estado em memória, sessão e cliente HTTP com CSRF.
- `frontend/src/screens`: telas; `worklist`: modelo/filtros; `viewer`: engine,
  ToolGroups, apresentação, layouts, Cine e thumbnails; `design-system`: visual.
- `frontend/tests`: testes Node e Chrome com servidores/fixtures sintéticos.

Go 1.26.1, net/http, pgx/v5, UUID e x/crypto; sem ORM/framework HTTP.
React 19, TypeScript, Vite; Cornerstone core/tools/metadata/dicom-image-loader
fixados em 5.11.0.

### Docker, banco e comunicação interna

O Dockerfile faz build multi-stage de frontend e backend; imagem final executa
como usuário sem privilégios e serve SPA/API na mesma origem. No Compose atual,
app usa a rede própria da aplicação e uma rede Docker externa do PACS para
alcançar Orthanc; PostgreSQL 18 fica na rede própria, com volume persistente
separado. O Compose principal não publica a porta do PostgreSQL no host e publica
a aplicação no loopback. O Compose de desenvolvimento publica apenas o banco
em loopback. Isso descreve arquivos do repositório, não uma inspeção de containers.
Não assumir HTTPS público já liberado: publicação controlada é etapa futura.

Configuração/segredos são fornecidos em execução; não reproduzir valores de
DATABASE_URL, PACS_MASTER_KEY ou arquivos .env. Preservar banco, armazenamento,
redes e serviços existentes; não expor Orthanc 8042 publicamente e não alterar
DICOM 4242. Não executar comandos de infraestrutura durante revisão documental.

## Autenticação, autorização e auditoria atuais

- `POST /api/auth/login`, `GET /api/auth/me`, `POST /api/auth/logout`.
- Argon2id: 64 MiB, três passes, paralelismo dois, sal aleatório de 16 bytes,
  chave de 32 bytes, formato PHC e comparação constante. Rehash no login quando
  parâmetros estão desatualizados; política atual mínima de 12 caracteres.
- Sessões server-side: identificador aleatório de 32 bytes, somente SHA-256 no
  banco; expiração absoluta padrão 12 h e inatividade 30 min. Logout revoga;
  autenticação da sessão também verifica conta ativa/validade. Existe primitiva
  `RevokeAllForUser`; reset/desativação administrativos agora revogam sessões
  atomicamente com a alteração, no store useradmin.
- Cookie de sessão HttpOnly, SameSite=Lax, Secure obrigatório em produção.
  CSRF para métodos de escrita, inclusive login: checagem atual de Origin/Referer
  (com limitações registradas em hardening) e double submit
  cookie/header `X-CSRF-Token`. Não guardar autenticação no localStorage.
- ADMIN/GESTOR/MEDICO autorizados no backend para Worklist/Viewer. Configuração
  Orthanc e administração de unidades são exclusivas de ADMIN.
- `user/role.go` já contém regras de domínio: GESTOR cria/administra apenas
  MEDICO; ADMIN tem administração global. Isso não significa que o CRUD de
  usuários possa usar essas regras sem restrição: useradmin aplica a matriz da
  Etapa 2 e protege ADMIN. O helper genérico permite ADMIN criar ADMIN; a nova
  API/UI não permite.
- Auditoria real em PostgreSQL; eventos e dívida de confiabilidade detalhados
  abaixo. Nunca registrar credenciais ou PHI.
- Rate limit de login em memória por processo; logs HTTP omitem query/corpo e
  mascaram IDs das rotas clínicas. Não considerar isso uma aprovação geral de
  segurança para produção: pendências anteriores continuam fora deste escopo.

**Auditoria V1 substitui a tela mockada por consulta real somente ADMIN**.
GESTOR/MEDICO não veem o menu e recebem 403 na API; gate de senha mantido.
Estado local: implementada/testada, ainda sem deploy/validação real. [AUDIT.md](AUDIT.md).

## Orthanc e Worklist V2 — VALIDADA EM AMBIENTE REAL

Configuração salva em `app_settings`, administrada por ADMIN; credencial cifrada
com AES-256-GCM e chave externa ao banco. API não retorna a credencial.
`POST /api/admin/settings/orthanc/test` consulta somente `GET /system` e registra
resultado sanitizado. Salvar configuração não testa a conexão automaticamente.

`internal/orthanc` centraliza http.Client, context e timeout configurado,
verificação TLS por cliente, validação SSRF no DNS/dial, bloqueio de redirects e
de proxies de ambiente. Redes privadas são permitidas; destinos especiais são
bloqueados. Não há proxy genérico nem URL arbitrária vinda do navegador.

`GET /api/studies`: sessão + ADMIN/GESTOR/MEDICO. Filtros AND: datas inclusivas,
nome, ID, accession, descrição, instituição e modalidade; padrões textuais
seguem semântica Orthanc, sem busca multi-campo implícita. DTO próprio, nunca
resposta bruta. `POST /tools/find` upstream é consulta read-only, Level Study,
Expand, Since/Limit com sentinela; não carrega todo o acervo.

- API: limit 1–50 (padrão 25), offset 0–10000, sort dateDesc/dateAsc/native.
- Ordem global StudyDate/StudyTime com desempate por StudyInstanceUID, antes da
  paginação. Exige ExtendedFind; modalidade exige também versão compatível
  >= 1.12.6. Incompatibilidade retorna 422; não simular ordem global na página.
- Modalidades derivadas das séries dos estudos da página, sem consulta global;
  até quatro chamadas de séries simultâneas. Máximo 2 + limit chamadas upstream
  por página ordenada; 1 + limit em ordem nativa sem modalidade.
- UI: busca por tipo, filtros avançados, períodos Hoje/Ontem/7/30 dias e
  personalizado; default últimos sete dias. Datas DICOM sem conversão UTC,
  ausências/valores inválidos exibidos como travessão, sem descrição inventada.
- **Preferência final de UI:** removidos controles avulsos Ordenação, Modalidade
  e Por página. Página fixa em 25; clicar no cabeçalho Data/Hora alterna ordem.
  Modalidade continua suportada na API e exibida na tabela.
- Estados explícitos: loading, refreshing, error/retry, empty e sessão expirada.
- Anterior/Próxima com hasMore, sem total fictício; Atualizar mantém filtros e
  volta à primeira página. Sem polling. Debounce 350 ms, AbortController e
  proteção contra resultados obsoletos.
- Estado dos filtros/offset vive em App, somente em memória, preservado ao
  voltar do Viewer; logout/expiração limpam. Query string do fetch ainda pode
  aparecer em DevTools/logs externos: proxies devem omiti-la, sem PHI em logs.

## Viewer V4 — VALIDADO COM DICOM REAL

V0 pipeline real; V1 séries/stack; V2 ferramentas; V3 múltiplos viewports;
**V4 validado com DICOM real pelo responsável**, conforme histórico documentado.
Essa validação externa não autoriza testes contra PACS real em desenvolvimento.

Gateway autenticado, autorizado para os três perfis, valida parentesco:

- `GET /api/studies/{studyID}/series`
- `GET /api/studies/{studyID}/series/{seriesID}/instances`
- `GET /api/studies/{studyID}/series/{seriesID}/instances/{instanceID}/dicom`

DICOM Part 10 por streaming, limite 128 MiB, sem cópia persistida; imageIds
wadouri apontam somente à API local. Não há DICOMweb neste pipeline.
Um RenderingEngine compartilhado, até quatro StackViewports, um ToolGroup por
viewport; ferramentas e navegação atuam no ativo. Stack sob demanda, thumbnails
lazy com uma instância representativa e fila controlada, cache oficial.

V4 mantém séries, scroll/setas, contador, layouts 1x1/1x2/2x2, auto-preenchimento
determinístico sem duplicar enquanto houver séries disponíveis, seleção manual,
maximização interna preservando viewports, Cine, WL/Zoom/Pan/Length/Angle/Probe/
Rectangle ROI, Invert, Rotate ±90°, Flip H/V, Fit e Reset. Redução de layout
destrói slots removidos e para Cine; expansão restaura atribuições de séries,
não toda sua apresentação anterior. Maximização não destrói os outros slots.

**Invert inicia OFF em toda nova série/viewport e volta OFF no Reset**, inclusive
MONOCHROME1; ativação manual permanece possível. **Não alterar esse comportamento
sem decisão futura explícita.**
Fit preserva VOI/Invert/orientação/annotations; Reset restaura apresentação e
não apaga medições. Delete/Backspace removem seleção; Limpar medições exige
confirmação e respeita contexto ativo. Annotations temporárias, compartilhamento
da mesma série conforme Cornerstone, sem banco/Orthanc/SR. Teclado protege campos
editáveis; Escape restaura maximização, R gira à direita, F ajusta enquadramento.

Limites: primeiro frame de multiframe; thumbnail pode transferir um DICOM inteiro;
Cine 1–30 FPS (default 10) manual, sem significado clínico garantido; sem MPR/3D,
hanging protocol, sincronização, download, impressão ou persistência de medições.
Cleanup de timers, listeners, observers, grupos/engine e requests deve ser mantido.

## Banco e migrations — 0001, 0002 e 0003 aplicadas

Runner em `internal/database/migrate.go`, SQL embutido, checksum SHA-256,
`schema_migrations`, uma transação por migration, somente avanço. **Serve aplica
migrations pendentes na inicialização**; não iniciar backend contra banco existente
para simples inspeção. **0001, 0002 e 0003 já estão aplicadas no ambiente atual**,
conforme confirmação do responsável; 0002/0003 também foram validadas funcionalmente.
São imutáveis: não editar esses arquivos nem seus checksums. Qualquer alteração
futura de schema deve utilizar nova migration. Auditoria V1 acrescenta 0004,
aplicada somente em bancos descartáveis de teste nesta entrega.

| Migration | Objetivo |
| --- | --- |
| `0001_init.sql` | Cria units, users, sessions, audit_events e app_settings, constraints e índices |
| `0002_units_management.sql` | Valida nomes legados e acrescenta CHECK/índice case-insensitive a units, sem alterar usuários/vínculos |
| `0003_users_management.sql` | Cria user_units, migra vínculos não-ADMIN, acrescenta must_change_password default false e restringe username sem renomear legado |
| `0004_audit_query.sql` | Índice parcial de alvo de usuário em audit_events; **pendente no ambiente real** |

`users` atual: UUID id; name; username único; email opcional; password_hash;
role; **unit_id opcional legado** (FK units, ON DELETE SET NULL); active;
access_valid_until DATE opcional (último dia permitido inclusive); last_login_at;
created_at/updated_at timestamptz e must_change_password. Username agora permite
3–64 caracteres, começa alfanumérico e aceita somente a-z, 0-9, hífen/underscore,
sem ponto. A migration falha se encontrar legado incompatível. `user_units` tem
PK (user_id,unit_id), FKs e índice de unidade; é a fonte dos novos vínculos.

`units` atual: UUID id; slug obrigatório único; name; kind obrigatório limitado
aos tipos legados; active; created_at/updated_at timestamptz. Nome com limite
120 caracteres, sem controles/vazio; índice `lower(btrim(name))` único inclusive
entre inativas. `audit_events.unit_id` também referencia units.
Novas unidades recebem slug técnico e kind OUTRO internamente; não são campos da
UI. Não remover essas colunas/referências por suposição de que não são utilizadas.

## Administração V1 — Etapa 1: Unidades — VALIDADA EM AMBIENTE REAL

Etapa 1 implantada e validada em ambiente real pelo responsável.
Tela Administração → Unidades: listar, criar, editar nome, desativar/reativar;
sem exclusão física ou exemplos pré-cadastrados. Somente ADMIN administra;
GESTOR/MEDICO não criam/editam/ativam/desativam unidades. Inativas permanecem
para histórico. A Etapa 2 adiciona associações.

| Endpoint | Regra |
| --- | --- |
| `GET /api/units` | Três perfis autenticados; somente ativas por padrão |
| `GET /api/units?includeInactive=true` | ADMIN; inclui inativas |
| `POST /api/admin/units` | ADMIN + CSRF; name obrigatório |
| `PATCH /api/admin/units/{id}` | ADMIN + CSRF; name e/ou active |

DTO: id, name, active, createdAt, updatedAt. Listagem paginada com items, limit,
offset, hasMore, nextOffset; limite padrão 50/máximo 100, offset máximo 10000.
Sem DELETE. Unicidade no PostgreSQL, queries parametrizadas, transação/lock de
linha no PATCH, timeout cinco segundos; 409 para duplicidade, erros sanitizados.
Auditoria: UNIT_CREATED, UNIT_UPDATED, UNIT_ACTIVATED, UNIT_DEACTIVATED; autor e
ID da unidade, sem nome submetido; alteração sem efeito não gera evento.

Migration 0002 aborta/rollback se legado contiver nomes inválidos/duplicados,
sem renomear ou mesclar automaticamente. **0002_units_management.sql está aplicada
e validada no ambiente real**, conforme informado pelo responsável.

## Administração V1 — Etapa 2: Usuários — VALIDADA EM AMBIENTE REAL

Backend, frontend, migration e fluxos implantados e validados pelo responsável.
**0003_users_management.sql está aplicada e validada no ambiente real.** Nenhuma
implantação ou acesso a esse ambiente foi feito nesta atualização documental.
Detalhes de contrato, segurança e decisões: [USERS.md](USERS.md).

- MEDICO/GESTOR têm múltiplas unidades; novos cadastros exigem ao menos uma ativa.
  ADMIN não exige unidade. Associações inativas existentes são preservadas.
- ADMIN cria/administra MEDICO e GESTOR. GESTOR cria/administra somente MEDICO,
  independentemente de suas próprias unidades. MEDICO pode ser administrado por
  ambos. GESTOR pode criar, editar, ativar/desativar, renovar e resetar MEDICO;
  não administra ADMIN/outro GESTOR, não cria esses papéis nem promove MEDICO.
  **Unidades do GESTOR NÃO limitam os médicos administrados nesta versão.**
  ADMIN tem administração global e não precisa de vínculo com unidade.
  API verifica papel real do alvo.
  ADMIN aparece protegido na lista: sem edição/reset/desativação/criação por UI.
- Username manual, minúsculo, único, imutável; sem ponto/espaço/acento. Papel
  também imutável. Não existe geração automática baseada no nome. Cadastro inclui
  nome, e-mail opcional, unidades, prazo e senha inicial, sem CRM/UF.
- Validade 1/3/6 meses, 1 ano ou sem expiração; backend usa calendário São Paulo,
  dia final inclusivo e ajuste ao último dia do mês. Renovação parte da validade
  ainda vigente, senão de hoje (inclusive conta antes sem prazo); sem expiração
  grava access_valid_until=NULL. Backend calcula a data, sem confiar no frontend.
  Conta expirada não autentica e sessão existente é rejeitada quando a validade
  termina. Reativação não renova: conta reativada pode continuar expirada.
- Senha nunca é armazenada em texto claro. Criação administrativa de MEDICO/GESTOR
  e reset usam Argon2id e must_change_password=true. Gate backend permite
  somente me/change-password/logout até troca; App não monta telas normais.
  Troca exige senha atual/nova diferente, limpa flag, revoga todas as sessões,
  limpa cookie e exige novo login. Reset/desativação também revogam atomicamente.
- Login verificado sob lock e rehash compare-and-swap protegem contra reset
  concorrente. Auditoria administrativa agora é atômica, com ator/alvo por ID, sem senhas.
- `users.unit_id` mantido, sem sincronização com novos vínculos. Migration 0003
  copia vínculos não-ADMIN, inclusive inativos, sem mudar senha/validade/ADMIN.
- GET `/api/admin/users`; POST `/api/admin/users/medicos` e `/gestores`;
  PATCH `/api/admin/users/{id}`; POST `/api/admin/users/{id}/active`, `/renew`,
  `/reset-password`; POST `/api/auth/change-password`.
- Nenhuma associação por estudo, mudança de papel, recuperação por e-mail,
  criação de ADMIN via UI ou exclusão física foi implementada.

## Auditoria existente e confiabilidade

Eventos confirmados em `internal/audit/audit.go`:

- Unidades: UNIT_CREATED, UNIT_UPDATED, UNIT_ACTIVATED, UNIT_DEACTIVATED.
- Usuários: USER_CREATED, USER_UPDATED, USER_ACTIVATED, USER_DEACTIVATED,
  USER_ACCESS_RENEWED, USER_PASSWORD_RESET, USER_PASSWORD_CHANGED, USER_UNITS_CHANGED.
- Autenticação: LOGIN_SUCCESS, LOGIN_FAILURE, LOGOUT.
- Configuração/conexão: ORTHANC_SETTINGS_CHANGED, ORTHANC_CONNECTION_TESTED.

`audit_events` registra horário, evento, ator, origem, unidade quando aplicável
e detalhe operacional. Usuários usam ID do alvo no detalhe; unidades usam unit_id.
Não registrar senha/hash/token/secret, nome/e-mail desnecessários do alvo ou PHI.
Auditoria V1: `GET /api/admin/audit`, somente ADMIN, filtros AND de período,
ator, ação, categoria e usuário alvo; limit padrão 50/máximo 100, offset até 10000,
ordem global occurred_at/id DESC. Sem contagem global, escrita ou exclusão.
Datas inclusivas de São Paulo. Ator/alvo resolvidos por LEFT JOIN ao cadastro
atual, sem e-mail/snapshot; evento não desaparece se referência estiver ausente.
Details têm allowlist na escrita e leitura; texto legado arbitrário não é retornado.
Login sem ator identificado deixa de persistir username tentado para evitar texto
sensível colado inadvertidamente. Histórico não foi reescrito.

**Unidades, Usuários, troca de senha e gravação de configuração PACS agora gravam
seus eventos obrigatórios na mesma transação da alteração.** Falha faz rollback,
incluindo revogação de sessões e eventos anteriores da operação. Sem duplicação
nos handlers. Login/logout/teste de conexão permanecem best-effort com prazo de
2 segundos independente do cancelamento e log fixo de falha, sem erro SQL bruto.
Essa limitação restante está documentada em [AUDIT.md](AUDIT.md).

0004_audit_query.sql acrescenta índice parcial para detail canônico de alvo,
aproveitando índices de data/ator/evento existentes. Sem novas colunas e sem
editar 0001–0003. **0004 não foi aplicada no ambiente real**; sua criação de índice
pode bloquear escritas temporariamente e exige avaliação antes de deploy futuro.
Retenção/exclusão automática continuam fora de escopo; antiga primitiva Purge
não utilizada foi removida. Não há garantia contra adulteração por acesso SQL direto.

## Validação automatizada registrada

Resultado da Etapa 2: gofmt, go vet/test/build e race em auth/user/useradmin/
httpapi/units aprovados. Frontend typecheck/build, 28 testes Node, Chrome Usuários,
Unidades, Worklist e Viewer V2/V3/V4 + Invert OFF aprovados durante a entrega.
Esse registro de testes locais é distinto da implantação/validação real posterior
informada pelo responsável. A Auditoria V1 posteriormente repetiu as regressões
locais e acrescentou os testes descritos abaixo.
Testes PostgreSQL inicialmente retornaram SKIP por ausência do servidor local.
Após obter PostgreSQL 18.6 em diretório temporário, os testes SQL/migration de
Unidades e Usuários passaram em clusters descartáveis isolados por socket Unix,
sem serviço instalado ou acesso a banco existente. Nas demais máquinas exigem
postgres/initdb no PATH; caso contrário retornam SKIP explícito.

Comandos usuais, quando uma entrega futura exigir validação:

```sh
cd backend
gofmt -l $(rg --files -g '*.go')
go vet ./...
go test ./...
go build ./...
go test -race ./internal/auth ./internal/user ./internal/useradmin ./internal/httpapi ./internal/units
cd ../frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
node tests/users-browser.mjs
node tests/units-browser.mjs
node tests/worklist-browser.mjs
node tests/viewer-smoke.mjs
```

## Validação local da Auditoria V1

Gofmt, go vet, go test, go build e race tests passaram. PostgreSQL 18.6 descartável
validou 0004, consultas e rollback obrigatório de alteração/eventos/sessões, inclusive
falha no segundo evento. Frontend typecheck/build, 31 testes Node e Chrome Auditoria,
Usuários, Unidades, Worklist e Viewer V2/V3/V4 + Invert OFF passaram. Fixtures
sintéticas; nenhum banco existente ou Orthanc real acessado. Detalhes: [AUDIT.md](AUDIT.md).

## Dívidas técnicas e hardening — pendências restantes

Manter visíveis para pré-produção:

- **Auditoria:** atomicidade administrativa implementada na V1, ainda exige
  validação real. Login/logout/teste PACS seguem best-effort; revisar confiabilidade
  desses fluxos, origem do logout, retenção e privilégios SQL antes de produção.
- **PostgreSQL:** revisar usuário da aplicação e privilégio mínimo, separando
  necessidades de operação e migrations; não inferir permissões reais do Compose.
- **Login/Argon2id:** rate limit em memória por processo, IP da conexão atrás de
  reverse proxy (sem confiar arbitrariamente em headers), proteção contra abuso
  computacional e limites de concorrência. O parser atual verifica parâmetros
  não nulos, mas não impõe tetos seguros de memória/iterações/paralelismo/tamanhos.
- **CSRF/Origin:** revisar política; código permite ausência de Origin/Referer
  e compara somente Host no caminho same-host, sem exigir igualdade de esquema.
  A descrição resumida de proteção em camadas não elimina essas limitações.
- **Banco:** revisar deadlines de queries e cobertura de contextos; Unidades e
  Usuários têm timeouts explícitos, o que não comprova cobertura de todo o backend.
- **Concorrência:** revisar gravação de configurações e migrations. Proteção de
  resultado de teste Orthanc por revisão não garante atomicidade de toda edição;
  runner tem transações/checksums, mas não lock global entre instâncias concorrentes.
- **Logs:** revisar sanitização de erros internos/SQL/panics e dos proxies; o
  logger HTTP omite query/corpo, mas isso não prova sanitização de todos os caminhos.
- **Frontend/dependências:** revisar vulnerabilidades npm sem audit fix automático.
  Warnings de chunks Vite grandes e módulos Node externalizados permanecem;
  números antigos de vulnerabilidades não equivalem a uma auditoria atualizada.
- **Build:** revisar `.dockerignore` continuamente; já contém `**/.env` e
  `**/.env.*`, cobrindo variantes em subdiretórios, inclusive backend/.env.
  Não registrar a correção anterior como ausente nem presumir hardening completo.
- **Logout:** `SessionProvider` limpa o estado local no finally mesmo quando o
  servidor falha. Isso não garante revogação remota; revisar UX e consistência.
- **Secrets:** conforme informado pelo responsável, credenciais/secrets reais
  expostos anteriormente exigem rotação controlada antes da produção, seguindo
  as orientações de hardening, sem reproduzir valores. Trocar a chave de cifra
  exige tratar as credenciais Orthanc já cifradas; não rotacionar nesta tarefa.

Não foi localizado arquivo dedicado de hardening entre os documentos versionados
consultados. Esta seção consolida as pendências solicitadas e as limitações de
BACKEND/USERS/ORTHANC_CONNECTION; não afirma que uma rotação já ocorreu.

## Divergências e trechos históricos que não devem orientar novas implementações

- WORKLIST.md ainda diz que V2 aguarda validação real; a confirmação atual do
  responsável é **V2 validada**, com ExtendedFind disponível. Modalidade existe
  na API/coluna, mas o seletor foi retirado da UI; não reintroduzi-lo por inferência.
- USERS.md e trechos antigos de ADMINISTRATION/BACKEND registram “sem deploy” ou
  gestão de usuários/vínculos ainda futura: são históricos de suas entregas.
  O código atual contém user_units e gestão real, e a implantação das Etapas 1/2
  com migrations 0001–0003 aplicadas foi confirmada agora pelo responsável.
- VIEWER.md confirma V4 real no início, mas ainda pede validação em uma seção
  posterior. Há descrição antiga de Reset recuperando invert nativo; o código
  `viewer/cornerstone.ts` explicitamente força false, como a correção documentada
  no início do arquivo. Invert inicial/Reset OFF é a decisão vigente.
- BACKEND.md e comentário do Dockerfile descrevem isolamento de qualquer rede do
  PACS; o Compose atual conecta app à rede externa do PACS para integração interna.
  Não confundir a separação do PostgreSQL da aplicação com ausência dessa conexão.
- DECISOES.md contém decisão histórica de conta compartilhada por unidade. O
  modelo vigente possui usuários MEDICO/GESTOR com múltiplas unidades e senha
  individual/troca obrigatória; não implementar contas compartilhadas com base
  nesse trecho sem nova decisão explícita.
- Descrições genéricas de CSRF e “nenhum erro sensível em logs” não substituem
  a revisão dos caminhos concretos indicada acima. `user/role.go` é mais amplo
  que as rotas de administração: a proteção do ADMIN é aplicada em useradmin.
- SKIP SQL registrado na Etapa 1 é histórico: a Etapa 2 posteriormente validou
  migrations/stores em PostgreSQL descartável. Em máquinas sem binários, o teste
  ainda pode retornar SKIP; conferir o resultado, não assumir que executou SQL.

Essas divergências históricas continuam registradas. Na entrega Auditoria V1,
ADMINISTRATION/USERS/BACKEND foram atualizados apenas quanto à nova auditoria.

## Próximas etapas planejadas — sem execução automática

1. Revisão e posterior validação autorizada da **Auditoria V1 implementada**, sem
   confundir testes locais com validação real. Contrato/decisões: [AUDIT.md](AUDIT.md).
2. **HARDENING / PRÉ-PRODUÇÃO:** revisar e tratar dívidas restantes.
3. Publicação HTTPS controlada; depois piloto controlado.
4. Somente após essas etapas avaliar substituição definitiva do sistema atual.

Este planejamento não autoriza deploy, migrations reais ou etapas adicionais.

## Regras permanentes

- Trabalhar incrementalmente, preservar decisões anteriores e design existente.
- Em deploy/produção, uma alteração relevante por vez; avaliar impacto antes de
  agir. Não fazer deploy ou operar ambiente real sem autorização específica.
- Nunca registrar passwords, hashes reais, secrets, tokens, cookies ou PHI;
  nunca usar pacientes reais em testes. Não ler/copiar conteúdo real de .env
  para documentação. Fixtures exclusivamente sintéticas.
- Browser → backend autenticado → Orthanc; sem proxy genérico, credencial no
  frontend ou publicação pública de 8042. Preservar DICOM 4242, armazenamento,
  NFS, Orthanc e a infraestrutura existente.
- Migrations aplicadas são imutáveis; mudanças futuras exigem nova migration.
- Não executar npm audit fix/--force nem atualizar dependências automaticamente.
- Não alterar Docker/rede/firewall/proxies ou infraestrutura fora do escopo.
- Não implementar features adicionais do Viewer/Worklist, MPR/3D, escrita DICOM,
  integrações RIS/HL7/FHIR/MWL ou novos módulos sem uma etapa autorizada.

Referências detalhadas: [BACKEND.md](BACKEND.md),
[ORTHANC_CONNECTION.md](ORTHANC_CONNECTION.md), [WORKLIST.md](WORKLIST.md),
[VIEWER.md](VIEWER.md), [ADMINISTRATION.md](ADMINISTRATION.md), [USERS.md](USERS.md).
