# Decisões — PACS Web Municipal

Registro incremental. Uma seção por entrega.

## Fonte de verdade visual

Projeto de design (claude.ai/design) `5f025800-0e0e-435a-be71-296c1210f96b` —
"UI mockups form design":

- `PACS Web Municipal - Telas.dc.html` — canvas de referência: enquadra as telas a 1440 × 900.
- `PacsPrototipo.dc.html` — o protótipo em si; **todas** as telas e estados saem daqui.
- `_ds/industry-…/styles.css` — sistema de design "Industry" (tokens + classes).
- `_ds/industry-…/readme.md` — regras do sistema (blueprint, marcas de registro, Lucide 1.5).

Telas catalogadas no canvas: 1a Login · 1b Exames/Worklist · 1c Visualizador 1×1 ·
2a Usuários · 2b Novo médico (modal) · 2c Auditoria · estados 3a–3g
(carregando, vazio, erro de lista, imagem carregando, erro de imagem, sistema
indisponível, sessão expirada).

## Entrega 1 — scaffold + tokens + Login (1a)

Data: 2026-09-25.

### O que foi feito

- Scaffold Vite + React 19 + TypeScript estrito (`strict`, `noUncheckedIndexedAccess`,
  `exactOptionalPropertyTypes`, `verbatimModuleSyntax`).
- `src/design-system/industry.css` — **vendorado literalmente** do projeto de design.
  Não editar à mão; re-vendorar quando o design mudar.
- `src/design-system/Blueprint.tsx` — as quatro marcas de registro `+`.
- `src/design-system/Icon.tsx` — ícones Lucide inline (paths do protótipo), traço 1.5,
  tamanhos 16/20/32. Evita dependência de biblioteca de ícones.
- `src/styles/app.css` — globais do protótipo: fundo de grade 56 px (`.pacs-grid-ground`)
  e as três animações (`pacsShimmer`, `pacsSpin`, `pacsBar`).
- `src/screens/LoginScreen.tsx` — tela 1a fiel ao design.
- `src/App.tsx` — troca de tela por estado local + um **andaime** temporário no lugar
  de Exames, claramente marcado como fora do design.

### Decisões

- **Estilos inline, como no design.** O protótipo define geometria por `style` inline;
  reproduzir assim mantém a fidelidade verificável linha a linha. Cor, fonte, espaço e
  sombra sempre via `var(--…)` do sistema.
- **Sem router nesta entrega.** Navegação real é Fase 2; estado local basta para 1a.
- **Sem dependências além de react/react-dom.** Ícones inline em vez de `lucide-react`.
- **`tsconfig.node.json` sem `types: ["node"]`** — `vite.config.ts` não usa APIs de Node,
  e assim não entra `@types/node`.

### Desvio consciente do design

- O protótipo mostra o campo de senha pré-preenchido (`"demonstracao"`). Aqui ele começa
  **vazio**: a regra do projeto proíbe senha em código, inclusive de demonstração. Único
  efeito visual é a ausência dos pontos no campo. O campo de usuário mantém
  `helena.marques` (identidade fictícia, não credencial).

### Validação executada

- `npm run typecheck` — sem erros.
- `npm run build` — build de produção OK (231,92 kB JS / 9,77 kB CSS).
- Verificação visual pelo usuário: pendente (`npm run dev`).

### Limitações conhecidas

- Nenhuma autenticação real: o botão "Entrar" apenas simula 700 ms de "Entrando…".
- Nenhum dado de paciente ainda; nenhum mock foi criado nesta entrega.
- Tela de Exames, visualizador, usuários, auditoria e os sete estados: não implementados.
- Fidelidade ainda não conferida a 1440 × 900 contra o canvas do design.

### Próxima etapa proposta

Fase 2, em passos: (1) shell da aplicação — sidebar Exames/Usuários/Auditoria + header com
identidade do médico; (2) mocks fictícios (pacientes "Exemplo"); (3) Exames/Worklist (1b)
com busca e filtros; (4) estados 3a–3c. Visualizador (1c) só depois, e ainda em CSS —
Cornerstone3D é Fase 5.

## Entrega 2 — front completo (todas as telas e estados)

Data: 2026-09-26.

### O que foi feito

Todo o canvas `PACS Web Municipal - Telas.dc.html` em React, com dados fictícios:

- `src/app/AppShell.tsx` — sidebar (Exames · Administração: Usuários, Auditoria) e header
  com a identidade do médico sempre visível, menu do usuário e botão Sair.
- `src/screens/ExamesScreen.tsx` — tela 1b: pesquisa, período (Hoje/Ontem/7 dias/Período),
  filtros de modalidade e unidade, linha de 62 px clicável, "Abrir ›" só no hover.
  Inclui os estados 3a (skeleton), 3b (vazio) e 3c (erro PACS-503).
  Exporta o `Dropdown` reutilizado pela tela de usuários.
- `src/screens/UsuariosScreen.tsx` — tela 2a e o modal 2b "Novo médico".
- `src/screens/AuditoriaScreen.tsx` — tela 2c, badges por tipo de evento.
- `src/screens/ViewerScreen.tsx` — tela 1c: toolbar com as 5 ferramentas de mouse
  (exclusivas, borda accent-500 na ativa), as 4 ações (só Inverter mantém estado),
  menu "Mais", layouts 1×1/1×2/2×2, painel de séries, sobreposições mono nos quatro
  cantos, marcadores R/L, navegador de imagens e tela cheia.
  Inclui os estados 3d (carregando) e 3e (erro de imagem).
- `src/screens/EstadoSistemaScreens.tsx` — 3f (sistema indisponível) e 3g (sessão expirada).
- `src/components/Toast.tsx` — confirmação "Médico criado…".
- `src/mocks/dados.ts` — 12 exames, 9 usuários, 12 eventos, todos fictícios.
- `src/dev/PainelDeTelas.tsx` — andaime só em `import.meta.env.DEV` para abrir qualquer
  uma das 13 telas/estados direto. Fora do design e fora do bundle de produção.
- `src/App.tsx` — máquina de estados da demonstração; o andaime `PendingScreen` saiu.

### Decisões

- **Hover em classe, não inline.** O protótipo usa `style-hover`, atributo do runtime de
  design. Em React viraram classes `.pacs-hover-*` em `app.css`, com exatamente as mesmas
  misturas de cor.
- **Radiografia em CSS.** A imagem é o mesmo desenho em gradientes do protótipo, marcada
  como demonstrativa. É o placeholder da área do viewport Cornerstone3D (Fase 5); nenhum
  renderizador próprio foi escrito.
- **`data-cornerstone-viewport="true"`** mantido em cada viewport, como no design, para o
  StackViewport se ancorar depois.
- **Interações do viewport portadas fielmente**: arraste altera W/L, zoom, pan ou rolagem
  conforme a ferramenta ativa; a roda do mouse troca de imagem.
- **Nomes dos mocks mantidos como no design.** São fictícios. Se a preferência for a forma
  literal "Paciente Exemplo NNN", é uma troca de lista em `src/mocks/dados.ts`.

### Validação executada

- `npm run typecheck` — sem erros.
- `npm run build` — OK (287,95 kB JS / 10,49 kB CSS); `PainelDeTelas` ausente do bundle
  (verificado por grep em `dist/assets/*.js`).
- Verificação visual pelo usuário: pendente (`npm run dev`, painel "telas" no canto
  inferior esquerdo).

### Limitações conhecidas

- Sem atalhos de teclado (W/Z/P/S/M, I/R/Esc/F, F11): o protótipo também só reage ao mouse.
- Medição, espelhamento, lupa e ângulo aparecem na UI mas não medem nada — Fase 5.
- Filtros da auditoria são estáticos; "Exportar CSV" não exporta — Fase 8.
- Sem autenticação, sem backend, sem Orthanc, sem DICOMweb.
- Fidelidade a 1440 × 900 ainda não conferida tela a tela contra o canvas.

### Próxima etapa proposta

Fase 2 fechada. Antes da Fase 3 (autenticação própria), vale uma passada de fidelidade
com o canvas aberto ao lado, corrigindo divergências de espaçamento que só aparecem na
comparação direta.

## Entrega 3 — fundação do backend Go (autenticação)

Data: 2026-09-26. Documentação detalhada em [BACKEND.md](BACKEND.md).

### O que foi feito

Módulo Go em `backend/` (Go 1.26.1), com PostgreSQL próprio da aplicação:
configuração por ambiente, pool pgx, migrations versionadas embutidas, modelo de
usuário com os perfis ADMIN/GESTOR/MEDICO, Argon2id, sessões server-side,
auditoria de LOGIN_SUCCESS / LOGIN_FAILURE / LOGOUT, os três endpoints de
autenticação, middlewares de sessão e de perfil, health check, comando seguro de
bootstrap do primeiro ADMIN, Dockerfile multi-stage e `docker-compose.dev.yml`
isolado.

No frontend, só a ligação — nenhuma tela redesenhada: proxy `/api` e `/health` no
Vite, cliente de API (`src/api/client.ts`), provider de sessão
(`src/auth/SessionProvider.tsx`), Login enviando ao backend e exibindo a
mensagem devolvida, Sair chamando o logout real, e `/api/auth/me` decidindo quem
está autenticado na carga da aplicação.

### Decisões

- **Sessão server-side em vez de JWT.** Não há necessidade de arquitetura
  stateless, e sessão no servidor permite revogar de imediato.
- **Só o SHA-256 do token vai para o banco.** Vazamento do banco não entrega
  cookie reutilizável.
- **CSRF em três camadas** (SameSite=Lax, conferência de Origin, double submit
  cookie+header), aplicadas a todo método que altera estado, login incluído.
- **Mesma origem em vez de CORS.** Proxy do Vite em desenvolvimento, `STATIC_DIR`
  ou proxy da infraestrutura em produção. Nunca `*`.
- **Runner de migrations próprio** (~120 linhas, com checksum e uma transação por
  migration) em vez de ferramenta externa. Justificado em BACKEND.md; **aguarda
  sua validação** — trocar por goose ou golang-migrate é barato.
- **`internal/httpapi`** em vez de `internal/http`, para não sombrear o nome do
  pacote da biblioteca padrão.
- **Sem flag de senha no `admin create`.** Argumentos aparecem em `ps` e no
  histórico; a senha vem do terminal sem eco ou da entrada padrão.
- **`/api/admin/ping` provisória**, só para comprovar a autorização por perfil.

### Divergência do design

O header mostra perfil e unidade onde o design mostra o CRM
(`CRM-PR 34.512 · UPA Francisco Beltrão`): o CRM ainda não existe no modelo de
usuário. Acrescentá-lo é uma migration e um campo — decisão sua.

### Validação executada

- `gofmt -l .` sem pendências; `go vet ./...`, `go test ./...` e `go build ./...`
  limpos.
- Frontend: `npm run typecheck` e `npm run build` limpos.
- Fluxo real contra PostgreSQL 18 em container isolado: migrations aplicadas,
  ADMIN criado pelo comando, login recusado com senha errada e com usuário
  inexistente (mesma mensagem), logout sem CSRF recusado, login correto com
  cookie HttpOnly, `/api/auth/me` mantendo a sessão, rota ADMIN liberada para
  ADMIN e recusada para MEDICO, conta desativada recusada, logout invalidando a
  sessão, auditoria gravada e nenhuma senha, hash, token ou cookie em log.

### Limitações conhecidas

Ver a seção correspondente em BACKEND.md. Em resumo: só autenticação; rate limit
em memória e por processo; `X-Forwarded-For` ignorado; sem Orthanc, DICOMweb ou
Cornerstone3D; worklist, usuários e auditoria da interface continuam com dados
fictícios.

## Entrega 4 — imagem única (frontend + backend)

Data: 2026-09-26.

### O que foi feito

- `Dockerfile` na raiz, multi-stage: Node compila o SPA → Go compila o binário →
  Alpine carrega os dois. Imagem final ~31 MB, usuário sem privilégios,
  `HEALTHCHECK` em `/health`, `STATIC_DIR=/srv/frontend` embutido.
- `docker-compose.yml` com aplicação + PostgreSQL, em rede e volume próprios,
  publicando só em `127.0.0.1` (quem expõe é o proxy existente).
- `.env.compose.example` com placeholders.
- `backend/Dockerfile` removido: a imagem única o substitui.

### Decisões

- **Mesma origem por padrão.** O backend serve o SPA, então CORS deixa de existir
  no caminho normal e o cookie de sessão continua `SameSite=Lax`.
- **`ALLOWED_ORIGINS` vazio no compose**, justamente porque não há segunda origem.
- **`POSTGRES_PASSWORD` obrigatória** no compose (`:?`): sobe com erro claro em
  vez de subir com senha padrão.

### Validação executada

Imagem construída; container rodando na rede isolada do compose de
desenvolvimento e reportado como `healthy`. Verificados na mesma origem:
`/health` e `/health/ready` em 200, `index.html` em 200, rota do SPA (`/exames`)
caindo no index em 200, asset JS em 200, e o fluxo completo de autenticação —
login 200, `/api/auth/me` 200, `/api/admin/ping` 204 para ADMIN, logout 204 e
`/api/auth/me` 401 depois dele. `docker compose config` recusa subir sem
`POSTGRES_PASSWORD`, como esperado.

### Fatos do domínio registrados nesta conversa

- **O raio-X é feito somente na UPA.** As outras unidades consultam os exames,
  não os produzem.
- **O acesso será por unidade: um usuário para cada unidade**, não um usuário por
  médico.
- **CRM não é necessário** no modelo de usuário.

Consequências a tratar quando as telas de usuários e auditoria forem ligadas ao
backend — nada disso foi implementado ainda:

1. Com conta por unidade, a auditoria identifica **a unidade e a estação**, não a
   pessoa. `audit_events` já guarda `unit_id` e `origin`; o que muda é a leitura
   do que a trilha significa.
2. Senha compartilhada entre os profissionais de uma unidade: a troca de senha
   passa a ser um procedimento da unidade, e a expiração por inatividade
   (30 min) fica mais importante, porque a estação é de uso coletivo.
3. O header do design mostra o nome de um médico ("Dra. Helena Marques"); com
   conta por unidade o natural é mostrar o nome da unidade. Ajuste pequeno, e
   depende da sua decisão.
4. O filtro "Unidade" da worklist deixa de significar "onde o exame foi feito"
   (sempre UPA) e passa a ser, se for útil, "unidade solicitante".

### Ajuste — execução local passou a ser toda em container

Data: 2026-09-26. A pedido, nada mais roda direto na máquina: o backend compilado
e o Vite foram encerrados, e o projeto compose `pacs-web` passou a servir
aplicação (`pacs-web-app-1`, porta 8080 no loopback) e banco
(`pacs-web-postgres-1`). `.env` local com `APP_ENV=development` e
`COOKIE_SECURE=false`, porque em `localhost` não há HTTPS e um cookie `Secure`
impediria o login.

O container do projeto `pacs-web-dev` (só PostgreSQL, do `docker-compose.dev.yml`)
não existe mais, mas **o volume `pacs-web-dev_pacsweb-postgres` foi preservado**:
`docker compose -f docker-compose.dev.yml up -d` o recria com os dados de antes.
Esse é o caminho para trabalhar com recarga automática do frontend, que a imagem
única não oferece.

## Entrega 5 — Configurações → PACS / Orthanc

Data: 2026-09-26.

### O que foi feito

Backend:

- `internal/secrets` — AES-256-GCM sobre a biblioteca padrão, com contexto
  autenticado por propósito, `ParseKey` (base64 ou hex, 32 bytes) e `GenerateKey`.
- `internal/settings` — modelo da conexão com o Orthanc, validação rigorosa da URL
  e store em `app_settings` com a credencial cifrada dentro do JSON.
- `GET` e `PUT /api/admin/settings/orthanc`, só ADMIN, com auditoria
  `ORTHANC_SETTINGS_CHANGED`.
- `PACS_MASTER_KEY` na configuração: obrigatória em produção, opcional em
  desenvolvimento (sem ela, a credencial não pode ser guardada).
- Subcomando `pacs-server keygen` para sortear a chave.

Frontend:

- `src/screens/ConfiguracoesScreen.tsx` — tela nova, desenhada no sistema Industry
  seguindo o padrão das telas administrativas: título de 40 px, cartão blueprint
  com marcas de registro, campos `.field`/`.input`, badge de status, bloco de erro
  com os tokens de erro e ações no rodapé do cartão.
- Item **Configurações** na barra lateral, visível só para ADMIN (a autorização
  real continua no backend).
- `api.getOrthancSettings` / `api.putOrthancSettings` no cliente.
- Item `2d` no painel de telas de desenvolvimento.

### Decisões

- **Guardar não conecta.** Nenhuma requisição ao Orthanc foi implementada; o botão
  "Testar conexão" está na tela e desabilitado, e a API informa
  `connectionTestAvailable: false`.
- **Credencial só de ida.** A API nunca a devolve; a tela mostra que existe e
  exige ação explícita para alterar ou remover. Salvar o resto não a toca.
- **Validação de URL tratada como SSRF** desde já, mesmo sem o cliente HTTP.
- **Sem tela de referência no design**, a construção seguiu o sistema Industry e o
  padrão das telas existentes, com sua autorização.

### Validação executada

`gofmt`, `go vet`, `go test ./...` e `go build ./...` limpos; frontend com
`typecheck` e build limpos. No container: GET sem configuração devolve os padrões,
PUT com credencial salva, PUT sem o campo `credential` altera timeout e TLS **sem
apagar a credencial**, URL com credencial embutida recusada com 400. No banco, o
campo `credentialSealed` está cifrado e o texto claro não aparece; a auditoria
registrou as duas alterações descrevendo os campos, sem valores; nem a credencial
nem a chave mestra aparecem em log.

### Pendências

Teste de conexão real, `OrthancClient`, política de rede para SSRF, rotação de
chave mestra e `admin reset-password`.

### Ajuste — volta ao desenvolvimento local

Data: 2026-09-26. A pedido, a execução em container foi desfeita para a fase de
desenvolvimento: backend e frontend voltam a rodar direto na máquina, e só o
PostgreSQL continua em container (não há servidor PostgreSQL instalado
localmente, apenas o cliente `psql`).

- Containers do projeto `pacs-web` removidos; imagens `pacs-web:dev` e
  `pacs-web:latest` apagadas. Os dois volumes foram **preservados**.
- `docker-compose.dev.yml` (só PostgreSQL, porta 55432) voltou a ser o ambiente de
  trabalho. Seu volume já tinha o `admin.teste` e a auditoria das sessões
  anteriores.
- `.env` da raiz (usado só pelo compose) removido; criado `backend/.env`, com a
  mesma chave mestra, apontando para a porta 55432.
- `Dockerfile` e `docker-compose.yml` **ficaram no repositório**, prontos e sem
  uso, para quando a produção for definida.
- `README.md` criado com o passo a passo de execução local.

### Ajuste — frontend movido para `frontend/`

Data: 2026-09-26. A pedido, o repositório passou a ter as duas partes em pastas
irmãs: `frontend/` e `backend/`. Foram movidos `index.html`, `package.json`,
`package-lock.json`, os `tsconfig*.json`, `vite.config.ts`, `src/` e
`node_modules/`; o `dist/` antigo foi descartado e é regerado em
`frontend/dist/`.

Ajustados: os `COPY` do estágio de frontend no `Dockerfile`, o `.dockerignore`, o
`.gitignore` (`frontend/node_modules`, `frontend/dist`), o nome no
`package.json` (`pacs-web-frontend`) e os caminhos citados na documentação. Os
comandos `npm` agora rodam dentro de `frontend/`.

As entradas anteriores deste documento citam caminhos como `src/App.tsx`: leia-os
como `frontend/src/App.tsx`.
