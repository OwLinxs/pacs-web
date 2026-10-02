# PACS Web — contexto para continuidade

Documento principal de continuidade do **novo PACS Web da Prefeitura Municipal
de Francisco Beltrão**. Revisado contra o repositório em 01/10/2026. Distingue
estado do código de operações informadas pelo responsável, sem acesso ao servidor
ou a `.env` real nesta consolidação. Leia antes de qualquer alteração.

## Estado funcional confirmado

| Entrega | Estado atual |
| --- | --- |
| Auditoria V1 | **VALIDADA EM AMBIENTE REAL**, conforme confirmação posterior do responsável; migration 0004 aplicada |
| Viewer V5 — exportação | **IMPLEMENTADO NO CÓDIGO**; validação real/deploy específicos de V5 não confirmados nesta sessão |
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

```text
Clientes --HTTPS--> pacs.franciscobeltrao.com.br --> Nginx Proxy Manager
                                                       | HTTP interno
                                                       v
                                                PACS Web (Go + React)
                                                 |               |
                                                 v               v
                                       PostgreSQL PACS Web   Orthanc HTTP interno
                                                                |          |
                                                                v          v
                                                      PostgreSQL Orthanc  TrueNAS/NFS DICOM
Modalidades DICOM --C-STORE--> Orthanc :4242
```

Fluxo da aplicação: `Browser React/TypeScript → API Go autenticada → Orthanc REST`.
O PostgreSQL da aplicação guarda usuários, sessões, auditoria, unidades e
configurações; permanece logicamente separado do banco interno do Orthanc.
Orthanc continua sendo o PACS/DICOM e mantém o armazenamento DICOM fora da
aplicação; o gateway não duplica arquivos persistentemente. O backend comunica-se
internamente com Orthanc. O navegador não recebe seu endereço interno nem credenciais.
Autenticação própria, sem Keycloak. **OHIF + Keycloak + oauth2-proxy antigos
permanecem como rollback**: não remover containers, dados, autenticação, proxy ou
configuração sem decisão explícita. Preservar o design existente, Worklist clara
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
separado. O Compose principal não publica PostgreSQL no host. **Estado atual do
arquivo:** publicação da aplicação no IP específico do host definido em
`docker-compose.yml`, com porta do host `${APP_PORT:-8080}` e 8080 no container.
O responsável informa uso operacional da porta **8082**, restrita por firewall
ao NPM; o valor efetivo de `APP_PORT` e a regra não foram inspecionados aqui.
O Compose de desenvolvimento publica apenas o banco em loopback.
`HARDENING.md`/`BACKEND.md` ainda descrevem bind do app em loopback: essa parte
diverge do Compose versionado atual. NPM faz a terminação HTTPS segundo o relato
operacional; não inferir o estado de TLS/firewall apenas dos arquivos do projeto.

Configuração/segredos são fornecidos em execução; não reproduzir valores de
DATABASE_URL, PACS_MASTER_KEY ou arquivos .env. Preservar banco, armazenamento,
redes e serviços existentes; HTTP Orthanc 8042 continua interno, DICOM 4242 deve
continuar operacional. TrueNAS/NFS para DICOM e NFS separado para backups são
informações operacionais do responsável; não foram inspecionados no repositório.

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
Estado: validada em ambiente real conforme confirmação posterior do responsável. [AUDIT.md](AUDIT.md).

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
  Modalidade continua suportada na API e exibida na tabela. O filtro de
  instituição é texto DICOM; não há filtro clínico por vínculo à entidade `units`.
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
Viewer V5 adiciona exportação da imagem atual como PNG/JPEG/PDF, presente no código,
mas sem validação real específica confirmada;
consulta [VIEWER_EXPORT.md](VIEWER_EXPORT.md). Invert OFF inicial/Reset permanece.

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

**Mapeamento de mouse implementado localmente nesta entrega:** esquerdo = Pan
por padrão (medição selecionada assume temporariamente; novo clique a desativa),
direito = Window/Level e roda = Zoom, sem navegação do stack. Setas continuam a
navegação. Bindings são limpos/reaplicados por ToolGroup de viewport; o menu de
contexto é bloqueado só no viewport. Requer validação real controlada antes de
ser marcado como validado em produção.

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

### Exportação V5 presente no código — validação real específica não confirmada

Somente a imagem atual do viewport ativo; PNG sem perda, JPEG qualidade 0,95 e
PDF com `pdf-lib` 1.17.1 carregado sob demanda. A captura usa
`StackViewport.getCanvas()` e composição temporária separada, preservando imagem,
WL, zoom, pan, rotação, flip, invert, layout, ferramentas e Cine sem alterar DICOM
ou estado do Viewer. Overlays HTML e annotations SVG não entram no arquivo.
Modo identificado usa tags naturalizadas da instância corrente no cache
Cornerstone: PatientName, PatientID, StudyDate, Modality, StudyDescription,
InstitutionName, SeriesNumber e InstanceNumber. O nome DICOM é normalizado para
apresentação; ausências não são inventadas. PNG/JPEG identificados reservam faixa
fora da anatomia; PDF organiza imagem e texto. Arquivo tem nome neutro, sem
nome/ID/UID. Objetos temporários permanecem em memória e Object URLs são revogadas.

Modo sem identificação contém somente pixels renderizados, sem texto adicional;
requer `BurnedInAnnotation=NO` e confirmação visual do operador. **Não é
anonimização**: PHI gravada nos pixels pode permanecer, e a tag pode estar errada.
`POST /api/viewer/exports` exige sessão, papel clínico e CSRF. O backend fixa
`VIEWER_IMAGE_EXPORTED`, ator da sessão e apenas formato/booleano identified;
não recebe UID/PHI/filename. O evento confirma preparação/solicitação, não o
salvamento efetivo pelo navegador. Falha de auditoria/expiração segue a política
em [VIEWER_EXPORT.md](VIEWER_EXPORT.md). Não afirmar validação real de V5 sem
confirmação posterior do responsável.

## Banco e migrations — 0001–0004 aplicadas segundo o responsável

Runner em `internal/database/migrate.go`, SQL embutido, checksum SHA-256,
`schema_migrations`, uma transação por migration, somente avanço. **No código
atual, `serve` em produção apenas verifica checksums/versões e exige role runtime
sem DDL; `migrate` exige `MIGRATION_DATABASE_URL` separada.** A efetiva implantação
da separação de roles/grants no servidor não foi confirmada nesta consolidação.
Em desenvolvimento, `serve` ainda aplica migrations pendentes.
**0001, 0002, 0003 e 0004 já estão aplicadas no ambiente atual**,
conforme confirmação do responsável; 0002/0003 também foram validadas funcionalmente.
São imutáveis: não editar esses arquivos nem seus checksums. Qualquer alteração
futura de schema deve utilizar nova migration. Auditoria V1 acrescentou 0004, também aplicada e validada posteriormente no ambiente real conforme o responsável.

Role runtime: CONNECT no banco, USAGE no schema, DML apenas nas tabelas da
aplicação, USAGE/SELECT na sequência de auditoria e SELECT em
`schema_migrations`; sem DDL/superuser nem herança da role de migration.
O Compose fornece `DATABASE_URL` ao `app` via `APP_DATABASE_URL` e não injeta a
credencial de migration no serviço contínuo. Para mudança futura de schema:
criar **nova** migration, revisar backup/rollback, aplicar em janela controlada
com `migrate` e role separada, reaplicar grants mínimos aos objetos novos e só
então iniciar/atualizar `serve`. Nunca executar migrations automaticamente em
produção nem reproduzir DSNs ou passwords na documentação.

| Migration | Objetivo |
| --- | --- |
| `0001_init.sql` | Cria units, users, sessions, audit_events e app_settings, constraints e índices |
| `0002_units_management.sql` | Valida nomes legados e acrescenta CHECK/índice case-insensitive a units, sem alterar usuários/vínculos |
| `0003_users_management.sql` | Cria user_units, migra vínculos não-ADMIN, acrescenta must_change_password default false e restringe username sem renomear legado |
| `0004_audit_query.sql` | Índice parcial de alvo de usuário em audit_events; **aplicada no ambiente real conforme o responsável** |

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
- Exportação V5 local: VIEWER_IMAGE_EXPORTED, apenas formato e modo identificado.

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
editar 0001–0003. **0004 foi aplicada no ambiente real**, conforme confirmação posterior do responsável.
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

## Hardening — código versionado; implantação de controles não verificada aqui

Ver [HARDENING.md](HARDENING.md) para configuração operacional e revisão individual
dos avisos npm. O Git contém o hardening, mas não houve verificação aqui de sua
aplicação integral no servidor. `serve` em produção verifica migrations/checksums
e privilégios da role runtime sem DDL; `migrate` exige credencial separada. O
Compose usa `APP_DATABASE_URL` para a role runtime. `PUBLIC_ORIGIN` HTTPS e
`TRUSTED_PROXY_CIDRS` explícitos são obrigatórios em produção. Rate limit falha
fechado na saturação, usa X-Forwarded-For somente de peers confiáveis, limita
Argon2 a dois logins simultâneos; parser PHC possui tetos. CSRF em produção
exige Origin/Referer da origem pública HTTPS. Logs omitem query/body/IDs de
rotas clínicas e erros internos; logout local aguarda confirmação de revogação
ou 401. Cookies de sessão são HttpOnly, Secure e SameSite=Lax. `.dockerignore`
cobre `.env`/variantes na raiz e subdiretórios. Secrets devem ficar fora do Git.
`fflate` 0.7.5 foi fixado sem upgrade major; inventário npm de 30/09/2026 caiu
de 12 para 10 pacotes reportados. Não executar `npm audit fix` automaticamente.

Ainda exigem confirmação/ação controlada antes da abertura definitiva: testar a
role runtime/grants efetivos; revisar NPM/HTTPS/proxy CIDR/Host/forwarded
headers e firewall; rotação controlada de credenciais/secrets anteriormente
expostos (incluindo regravação segura da credencial Orthanc sob nova chave);
revisão dos 10 avisos npm restantes com base no upstream. Rate limit é por
processo, portanto escalonamento horizontal requer controle adicional no proxy.
Rotação não foi executada por esta consolidação.

Permanecem como dívidas: deadlines de todas as queries, concorrência de edição
das configurações e entre runners de migration, política de retenção da auditoria,
eventos não transacionais best-effort (login/logout/teste PACS), warnings de
bundle Vite e revisão de logs/configuração no proxy. Viewer V5 requer validação
real específica antes de ser marcado como tal.

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
- Registros históricos podem descrever ausência de rede PACS; o Compose conecta
  o app à rede externa do Orthanc para a API interna. O PostgreSQL permanece separado.
- `HARDENING.md` e `BACKEND.md` ainda dizem que o Compose publica o app em
  `127.0.0.1`; `docker-compose.yml` atual vincula ao IP específico do host.
  Porta host depende de `APP_PORT`; 8082 foi informada operacionalmente, não
  verificada neste arquivo. O `README.md` também contém instruções antigas de
  auto-migration em produção; prevalece o código atual de `serve`/`migrate`.
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

## Deploy, reboot e backups — informações operacionais fornecidas

Workflow informado: `desenvolvimento/Codex → Git → GitHub privado → servidor →
git pull --ff-only → build/deploy controlado`. Não alterar código diretamente
no servidor. `.env` real fica fora do Git. Para validar Compose, usar apenas
`docker compose config --quiet`; `docker compose config` sem `--quiet` pode
imprimir secrets. Alterações em produção devem ocorrer uma de cada vez, com
teste/rollback definidos. Esta consolidação não fez commit, push ou deploy.
Histórico Git recente conferido: `bbdc7d5` (bind do Compose ao host PACS,
30/09/2026), `c9ba867` (hardening, 30/09), `84d05a6` (exportação V5,
30/09), `862c668` (Auditoria V1, 29/09), `7a5e1b2` (Usuários, 29/09).
Esses commits comprovam estado versionado, não execução operacional.

O responsável informou **reboot controlado validado com sucesso**: mounts NFS
do storage DICOM e de backup retornaram automaticamente; storage Orthanc e backup
disponíveis; Orthanc e seu PostgreSQL voltaram; PACS Web e PostgreSQL da aplicação
voltaram healthy; stack legado voltou; regra de firewall da porta PACS Web
persistiu; NPM alcançou health/readiness e HTTPS funcionou; login, Worklist,
abertura de estudo no Viewer e carregamento de imagens funcionaram; DICOM 4242
voltou e C-ECHO respondeu Success. **A data exata dessa validação não consta no
repositório nem no histórico disponível a esta sessão; confirmar no registro
operacional antes de atribuir uma data.** Esses testes não confirmam por si sós
deploy/validação real específicos da exportação V5 ou grants da role runtime.

Backup do PostgreSQL PACS Web, conforme informado: `pg_dump` em formato custom
testado; `pg_restore` em banco isolado, estrutura/tabelas conferidas, banco de
teste removido; backup armazenado no NFS de backup. O script externo ao
repositório `/usr/local/sbin/pacs-web-db-backup.sh` está **em andamento**. O último
passo confirmado foi criar/testar a proteção que exige `/mnt/pacs-backup`
montado. Uma versão posterior com `pg_dump` estava em preparação/teste; **não
afirmar que backup automatizado do PACS Web esteja concluído**. O responsável
também informou backup completo da VM por Proxmox e job automático configurado;
retenção e resultados recentes não estão confirmados aqui.

ClearCanvas é o acervo legado; migração V5 foi informada como validada e V6
planejada. Não existe implementação/protocolo dessa migração neste repositório.
Preservar suas validações, manter migração separada de mudanças no Viewer e
não afetar C-STORE 4242. Não usar dados reais de pacientes em testes/docs.

## Controles do mouse — implementação local pendente de validação real

O padrão solicitado acima substitui o listener de wheel para stack. A mudança
foi isolada no frontend Viewer; não alterou exportação, backend, banco ou
dependências. O comportamento anterior de scroll para imagens em trechos
históricos de [VIEWER.md](VIEWER.md) não descreve mais o código atual.

### Tarefa posterior separada — NÃO implementar agora

Após validar o mouse, avaliar overlays DICOM **somente na exportação identificada
derivada**, distribuídos pelas bordas como em imagens radiológicas clínicas.
Mostrar apenas tags presentes: identificação, idade/sexo/data, data/hora,
série/imagem, lateralidade, parâmetros técnicos, instituição, WL/zoom e outros
metadados pertinentes; nunca inventar ausências. Modo não identificado não deve
incluir PHI e mantém a proteção atual contra identificação gravada nos pixels.
A imagem de referência discutida contém dados identificáveis: **não copiá-la ao
repositório, fixtures ou documentação, nem reproduzir seus dados**. Registra-se
apenas o padrão visual desejado. Não misturar esta tarefa com a do mouse.

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
- Ler este documento na próxima sessão; trabalhar incrementalmente, uma alteração
  por vez, testar antes de avançar, não implementar tarefas futuras junto da atual.
- Preservar o stack legado como rollback. Avaliar impacto antes de modificar
  NPM, firewall, Docker, redes, TrueNAS/NFS ou qualquer infraestrutura.

Referências detalhadas: [BACKEND.md](BACKEND.md),
[ORTHANC_CONNECTION.md](ORTHANC_CONNECTION.md), [WORKLIST.md](WORKLIST.md),
[VIEWER.md](VIEWER.md), [ADMINISTRATION.md](ADMINISTRATION.md), [USERS.md](USERS.md).
