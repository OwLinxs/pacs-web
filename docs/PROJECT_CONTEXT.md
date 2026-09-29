# PACS Web — contexto para continuidade

Atualizado para Administração V1 Etapa 2 em 29/09/2026 a partir do repositório e das decisões explícitas do
responsável pelo projeto. Este documento distingue implementação atual de plano
futuro; não comprova estado de banco ou infraestrutura em execução.

## Arquitetura e localização do código

`Browser React/TypeScript → API Go autenticada → Orthanc REST`.
O PostgreSQL da aplicação guarda usuários, sessões, auditoria, unidades e
configurações; permanece logicamente separado do banco interno do Orthanc.
Orthanc é o PACS. O navegador não recebe seu endereço interno nem credenciais.
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
fixados em 5.11.0. Docker/Compose existem; não os alterar nem iniciar serviços
como parte desta consolidação.

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
  CSRF para métodos de escrita, inclusive login: Origin/Referer e double submit
  cookie/header `X-CSRF-Token`. Não guardar autenticação no localStorage.
- ADMIN/GESTOR/MEDICO autorizados no backend para Worklist/Viewer. Configuração
  Orthanc e administração de unidades são exclusivas de ADMIN.
- `user/role.go` já contém regras de domínio: GESTOR cria/administra apenas
  MEDICO; ADMIN tem administração global. Isso não significa que o CRUD de
  usuários possa usar essas regras sem restrição: useradmin aplica a matriz da
  Etapa 2 e protege ADMIN. O helper genérico permite ADMIN criar ADMIN; a nova
  API/UI não permite.
- Auditoria PostgreSQL: LOGIN_SUCCESS, LOGIN_FAILURE, LOGOUT,
  ORTHANC_SETTINGS_CHANGED, ORTHANC_CONNECTION_TESTED e eventos de unidades.
  Política atual best-effort: falha da auditoria não desfaz operação concluída.
  Registrar autor/origem e metadados operacionais, nunca credenciais ou PHI.
- Rate limit de login em memória por processo; logs HTTP omitem query/corpo e
  mascaram IDs das rotas clínicas. Não considerar isso uma aprovação geral de
  segurança para produção: pendências anteriores continuam fora deste escopo.

**A tela de Auditoria ainda usa mocks** (`frontend/src/mocks/dados.ts`).
Usuários foi integrada à API na Etapa 2.
Não confundir a tela mockada de auditoria com a gravação real de eventos no banco.

## Orthanc e Worklist V2

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
- Anterior/Próxima com hasMore, sem total fictício; Atualizar mantém filtros e
  volta à primeira página. Sem polling. Debounce 350 ms, AbortController e
  proteção contra resultados obsoletos.
- Estado dos filtros/offset vive em App, somente em memória, preservado ao
  voltar do Viewer; logout/expiração limpam. Query string do fetch ainda pode
  aparecer em DevTools/logs externos: proxies devem omiti-la, sem PHI em logs.

## Viewer V4 e Invert

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
MONOCHROME1; ativação manual permanece possível. Não alterar essa preferência.
Fit preserva VOI/Invert/orientação/annotations; Reset restaura apresentação e
não apaga medições. Delete/Backspace removem seleção; Limpar medições exige
confirmação e respeita contexto ativo. Annotations temporárias, compartilhamento
da mesma série conforme Cornerstone, sem banco/Orthanc/SR. Teclado protege campos
editáveis; Escape restaura maximização, R gira à direita, F ajusta enquadramento.

Limites: primeiro frame de multiframe; thumbnail pode transferir um DICOM inteiro;
Cine 1–30 FPS (default 10) manual, sem significado clínico garantido; sem MPR/3D,
hanging protocol, sincronização, download, impressão ou persistência de medições.
Cleanup de timers, listeners, observers, grupos/engine e requests deve ser mantido.

## Banco e migrations existentes

Runner em `internal/database/migrate.go`, SQL embutido, checksum SHA-256,
`schema_migrations`, uma transação por migration, somente avanço. **Serve aplica
migrations pendentes na inicialização**; não iniciar backend contra banco existente
para simples inspeção. Não editar migrations já aplicadas.

| Migration | Objetivo |
| --- | --- |
| `0001_init.sql` | Cria units, users, sessions, audit_events e app_settings, constraints e índices |
| `0002_units_management.sql` | Valida nomes legados e acrescenta CHECK/índice case-insensitive a units, sem alterar usuários/vínculos |
| `0003_users_management.sql` | Cria user_units, migra vínculos não-ADMIN, acrescenta must_change_password default false e restringe username sem renomear legado |

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

## Administração V1 — Etapa 1: Unidades (implementada)

Etapa 1 implantada e validada em ambiente real pelo responsável.
Tela Administração → Unidades: listar, criar, editar nome, desativar/reativar;
sem exclusão física ou exemplos pré-cadastrados. A Etapa 2 adiciona associações.

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
sem renomear ou mesclar automaticamente. Aplicação em banco existente não foi
realizada pelo agente; implantação da Etapa 1 foi confirmada pelo responsável.

## Administração V1 — Etapa 2: implementada, sem deploy

Detalhes de contrato, segurança e decisões: [USERS.md](USERS.md).

- MEDICO/GESTOR têm múltiplas unidades; novos cadastros exigem ao menos uma ativa.
  ADMIN não exige unidade. Associações inativas existentes são preservadas.
- ADMIN cria/administra MEDICO e GESTOR. GESTOR cria/administra somente MEDICO,
  independentemente de suas próprias unidades. API verifica papel real do alvo.
  ADMIN aparece protegido na lista: sem edição/reset/desativação/criação por UI.
- Username manual, minúsculo, único, imutável; sem ponto/espaço/acento. Papel
  também imutável. Cadastro inclui nome, e-mail opcional, unidades, prazo e senha.
- Validade 1/3/6 meses, 1 ano ou sem expiração; backend usa calendário São Paulo,
  dia final inclusivo e ajuste ao último dia do mês. Renovação parte da validade
  ainda vigente, senão de hoje; sem expiração grava NULL. Reativação não renova.
- Criação/reset usam Argon2id e must_change_password=true. Gate backend permite
  somente me/change-password/logout até troca; App não monta telas normais.
  Troca exige senha atual/nova diferente, limpa flag, revoga todas as sessões,
  limpa cookie e exige novo login. Reset/desativação também revogam atomicamente.
- Login verificado sob lock e rehash compare-and-swap protegem contra reset
  concorrente. Auditoria permanece best-effort, com ator/alvo por ID, sem senhas.
- `users.unit_id` mantido, sem sincronização com novos vínculos. Migration 0003
  copia vínculos não-ADMIN, inclusive inativos, sem mudar senha/validade/ADMIN.
- GET `/api/admin/users`; POST `/api/admin/users/medicos` e `/gestores`;
  PATCH `/api/admin/users/{id}`; POST `/api/admin/users/{id}/active`, `/renew`,
  `/reset-password`; POST `/api/auth/change-password`.
- Nenhuma associação por estudo, mudança de papel, recuperação por e-mail,
  criação de ADMIN via UI ou exclusão física foi implementada.

## Validação, pendências e limites de continuidade

Resultado da Etapa 2: gofmt, go vet/test/build e race em auth/user/useradmin/
httpapi/units aprovados. Frontend typecheck/build, 28 testes Node, Chrome Usuários,
Unidades, Worklist e Viewer V2/V3/V4 + Invert OFF aprovados. Sem mudança em produção.
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

Warnings existentes: chunks Vite grandes e módulos Node externalizados em codecs.
Vulnerabilidades npm não foram reavaliadas nesta consolidação; não executar
npm audit fix, não atualizar dependências sem necessidade. Documentos antigos
podem conter próximos passos já superados; conferir código e seções recentes.

Não acessar Orthanc real/produção, aplicar migrations, fazer deploy ou alterar
Docker/rede/firewall/NFS/storage/DICOM/portas/Keycloak/OHIF/proxies sem autorização
específica. Não ler/expor conteúdo de .env, credenciais, hashes reais, tokens,
cookies ou dados clínicos. Testes somente sintéticos; nenhum PHI em logs,
fixtures, documentação ou métricas. Não implementar features avançadas do Viewer,
integrações RIS/HL7/FHIR/MWL, escrita no Orthanc ou módulos não autorizados.

Referências detalhadas: [BACKEND.md](BACKEND.md),
[ORTHANC_CONNECTION.md](ORTHANC_CONNECTION.md), [WORKLIST.md](WORKLIST.md),
[VIEWER.md](VIEWER.md), [ADMINISTRATION.md](ADMINISTRATION.md), [USERS.md](USERS.md).
