# Backend Go — PACS Web Municipal

Fundação de autenticação da nova aplicação da Secretaria Municipal de Saúde de
Francisco Beltrão. Esta etapa cobre usuários, perfis, senha, sessão, auditoria
de acesso e a ligação do login existente ao backend.

Orthanc, OHIF, Keycloak, oauth2-proxy, Nginx Proxy Manager, NFS e TrueNAS seguem
independentes. A integração READ-ONLY oferece o teste manual `GET /system` e a
Worklist paginada. A validação automatizada usa apenas servidores fictícios.

## Arquitetura

```
Frontend React (existente)
        │  HTTPS / JSON, mesma origem
        ▼
Backend Go  ──  net/http + pgx
        │
        ├── internal/config     configuração vinda do ambiente
        ├── internal/database   pool pgx + migrations versionadas
        ├── internal/user       modelo, perfis e regras de autorização
        ├── internal/auth       Argon2id, sessões, serviço de login
        ├── internal/audit      trilha de auditoria
        ├── internal/settings   configuração persistida e credencial cifrada
        ├── internal/orthanc    transporte seguro, teste de conexão e consulta de estudos/séries
        ├── internal/studies    contrato e validação da Worklist
        └── internal/httpapi    rotas, middlewares, DTOs, erros
        ▼
PostgreSQL do PACS Web (banco próprio, separado do banco do Orthanc)
```

Go 1.26.1.

### Dependências e por quê

| Módulo | Para quê |
| --- | --- |
| `github.com/jackc/pgx/v5` | Driver e pool PostgreSQL maduros do ecossistema Go, sem ORM. |
| `github.com/google/uuid` | Identificadores dos registros, compatíveis com `uuid` do PostgreSQL. |
| `golang.org/x/crypto` | Implementação do Argon2id (`argon2.IDKey`). |
| `golang.org/x/term` | Leitura de senha sem eco no `admin create`. |

Roteamento, JSON, logging (`log/slog`), embed das migrations e testes usam só a
biblioteca padrão. Não há framework HTTP, nem ORM, nem biblioteca de logging
externa, nem ferramenta externa de migration.

### Decisão a validar: runner de migrations próprio

As migrations são arquivos `NNNN_descricao.sql` embutidos no binário e aplicados
por um runner de ~120 linhas (`internal/database/migrate.go`): tabela de
controle `schema_migrations`, uma transação por migration, checksum SHA-256 para
impedir edição retroativa de migration já aplicada, e só avanço — nenhuma
reversão, nada destrutivo automático.

Motivo: evita trazer uma ferramenta com CLI e drivers próprios para um problema
que a biblioteca padrão resolve com clareza. Como os arquivos são SQL simples e
numerados, migrar depois para `goose` ou `golang-migrate` é trocar o runner sem
tocar nas migrations. **Se preferir uma ferramenta consolidada desde já, é uma
troca pequena — decisão sua.**

## Banco

Migration `0001_init`:

| Tabela | Conteúdo | Restrições e índices relevantes |
| --- | --- | --- |
| `units` | Unidades de saúde | `slug` único com formato validado; `kind` restrito a UPA / UNIDADE_18H / UBS / HOSPITAL / OUTRO |
| `users` | Usuários | `username` único e com formato validado; `role` restrito a ADMIN / GESTOR / MEDICO; FK para `units` com `ON DELETE SET NULL`; índices por unidade e por perfil |
| `sessions` | Sessões server-side | `token_hash` único; FK para `users` com `ON DELETE CASCADE`; índices por usuário e por expiração |
| `audit_events` | Trilha de auditoria | índices por data (desc), por evento e por ator |
| `app_settings` | Configuração operacional (chave/valor JSONB) | conexão Orthanc, credencial cifrada e última verificação |

Nenhuma unidade é criada por migration: unidades serão administráveis pela
interface, e nada da Prefeitura está fixado em código.

## API

| Método | Rota | Autenticação | Resposta |
| --- | --- | --- | --- |
| POST | `/api/auth/login` | pública, com rate limit e CSRF | 200 com o usuário; 400 / 401 / 403 / 429 |
| GET | `/api/auth/me` | sessão | 200 com o usuário; 401 |
| POST | `/api/auth/logout` | sessão + CSRF | 204 |
| GET | `/api/studies` | sessão + ADMIN, GESTOR ou MEDICO | página de estudos; 400; 401; 403; 502; 503; 504 |
| GET | `/api/admin/ping` | sessão + perfil ADMIN | 204; 401; 403 |
| GET | `/api/admin/settings/orthanc` | sessão + perfil ADMIN | 200 com a configuração; 401; 403 |
| PUT | `/api/admin/settings/orthanc` | sessão + perfil ADMIN + CSRF | 200 com a configuração salva; 400; 401; 403; 503 |
| POST | `/api/admin/settings/orthanc/test` | sessão + perfil ADMIN + CSRF | resultado sanitizado do teste |
| GET | `/health` | pública | 200 `{"status":"ok"}` |
| GET | `/health/ready` | pública | 200, ou 503 se o PostgreSQL não responder |

`/api/admin/ping` é temporária: existe para comprovar a autorização por perfil e
será substituída pelos endpoints reais de administração.

Erros seguem um envelope único:

```json
{ "error": { "code": "INVALID_CREDENTIALS", "message": "Usuário ou senha inválidos." } }
```

Códigos: `INVALID_REQUEST`, `INVALID_CREDENTIALS`, `ACCOUNT_INACTIVE`,
`ACCOUNT_EXPIRED`, `UNAUTHENTICATED`, `FORBIDDEN`, `NOT_FOUND`, `CONFLICT`,
`RATE_LIMITED`, `CSRF_INVALID`, `SERVICE_UNAVAILABLE`, `INTERNAL_ERROR`.
Nenhuma resposta traz stack trace, SQL, caminho de arquivo ou secret.

## Segurança

### Senha

Argon2id com parâmetros embutidos no próprio hash, no formato PHC:
`$argon2id$v=19$m=65536,t=3,p=2$<sal>$<chave>` — 64 MiB, 3 passes, 2 lanes, sal
de 16 bytes, chave de 32 bytes (RFC 9106, perfil de uso interativo). Comparação
em tempo constante. Política mínima: 12 caracteres.

Como os parâmetros ficam gravados, um hash antigo continua validando e é
**recalculado automaticamente no próximo login** quando os parâmetros atuais são
mais fortes. O hash nunca sai pela API (as respostas usam DTOs próprios) e nunca
entra em log.

### Sessão

Sessão server-side, sem JWT: não há necessidade de arquitetura stateless, e
sessão no servidor permite revogar de imediato.

- Token de 32 bytes aleatórios; o navegador recebe só ele, no cookie.
- O banco guarda **apenas o SHA-256** do token. Um vazamento do banco não
  entrega cookies reutilizáveis.
- Expiração absoluta (`SESSION_ABSOLUTE_TTL`, padrão 12 h) e por inatividade
  (`SESSION_IDLE_TTL`, padrão 30 min, renovada a cada requisição válida).
- Revogação por sessão e por usuário; logout revoga no servidor.
- Conta desativada ou com validade vencida perde acesso mesmo com sessão aberta.
- Limpeza periódica das sessões que já não servem.

Cookies: `pacs_session` com `HttpOnly`, `SameSite=Lax`, `Path=/` e `Secure` em
produção (não é possível desligar `COOKIE_SECURE` com `APP_ENV=production`).
Nada sensível vai para `localStorage`.

### CSRF

Como a sessão é por cookie, a proteção é em camadas, aplicada a **todo** método
que altera estado, login incluído:

1. `SameSite=Lax` no cookie de sessão;
2. conferência de `Origin` (ou `Referer`) contra o próprio host e as origens
   explicitamente configuradas;
3. double submit: o cookie `pacs_csrf` — legível por script de propósito — tem
   de ser igual ao header `X-CSRF-Token`, comparados em tempo constante.

Um site terceiro não consegue ler o cookie para montar o header, então não
consegue forjar a requisição. O backend emite o cookie em qualquer resposta da
API; o cliente do frontend busca `/api/auth/me` antes da primeira operação e
repete uma vez quando o token venceu.

### Rate limiting

Janela fixa por IP em `POST /api/auth/login` (`LOGIN_RATE_ATTEMPTS`, padrão 10,
em `LOGIN_RATE_WINDOW`, padrão 5 min). Um acerto libera a contagem daquele
endereço. Resposta 429 com `Retry-After`.

Limitações assumidas: vale por processo (com várias instâncias, o limite é por
instância), zera ao reiniciar, e o mapa tem teto de 10 000 entradas — ao
estourar, janelas vencidas são descartadas e, se ainda estiver cheio, a
requisição passa, porque negar acesso ao sistema por pressão de memória nossa
seria pior. Com mais de uma instância, mover o contador para o PostgreSQL ou
para o proxy.

### Autorização

Verificada no servidor, sempre. `internal/user/role.go` concentra as regras:

- **MEDICO** — funções clínicas autorizadas.
- **GESTOR** — administra apenas MEDICO: não cria GESTOR nem ADMIN, e não
  promove ninguém a esses perfis. Não altera configuração crítica.
- **ADMIN** — administração completa.

A barra lateral esconde a seção Administração de quem não administra, mas isso é
conveniência visual: a decisão é do backend, que recusa a rota com 403.

### Enumeração de usuários

Usuário inexistente e senha errada devolvem exatamente o mesmo erro. No caso de
usuário inexistente o backend ainda calcula um hash descartável, para o tempo de
resposta não revelar a diferença. Conta desativada e validade vencida só viram
erro específico **depois** de a senha ser comprovada.

### Logs

`log/slog` em JSON. Registra método, rota, status, duração e IP. Não registra
corpo, query string, cookie, header de autorização, senha, hash nem token.

## Configuração

Todas por ambiente; veja `backend/.env.example`. Nunca comite `.env`.

| Variável | Para quê |
| --- | --- |
| `APP_ENV` | `development` ou `production` |
| `HTTP_ADDR` | endereço de escuta |
| `LOG_LEVEL` | `debug`/`info`/`warn`/`error` |
| `DATABASE_URL` | PostgreSQL **da nova aplicação** |
| `SESSION_ABSOLUTE_TTL`, `SESSION_IDLE_TTL` | prazos de sessão |
| `COOKIE_SECURE` | obrigatório em produção |
| `ALLOWED_ORIGINS` | origens de CORS, explícitas; vazio quando a origem é a mesma |
| `STATIC_DIR` | build do frontend servido pelo próprio backend |
| `LOGIN_RATE_ATTEMPTS`, `LOGIN_RATE_WINDOW` | proteção do login |
| `PACS_MASTER_KEY` | cifra as credenciais do Orthanc; obrigatória em produção |
| `SHUTDOWN_TIMEOUT` | prazo do encerramento gracioso |

`PACS_MASTER_KEY` é obrigatória quando `APP_ENV=production`. Em desenvolvimento
pode ficar vazia: a aplicação sobe, avisa no log e recusa gravar credencial.

### CORS

Não há `Access-Control-Allow-Origin: *`. Em produção o arranjo previsto é mesma
origem — `STATIC_DIR` apontando para o `dist/`, ou o proxy da infraestrutura
servindo os dois no mesmo host. Em desenvolvimento, o Vite faz proxy de `/api` e
`/health` para o backend, então o navegador também vê uma única origem e
`ALLOWED_ORIGINS` fica vazio. Se algum arranjo exigir CORS, só origens absolutas
listadas explicitamente são liberadas.

### Servidor HTTP

`ReadHeaderTimeout` 5 s · `ReadTimeout` 15 s · `WriteTimeout` 30 s ·
`IdleTimeout` 60 s · `MaxHeaderBytes` 1 MiB. Encerramento gracioso por
`SIGINT`/`SIGTERM`: para de aceitar conexões, drena dentro de
`SHUTDOWN_TIMEOUT`, encerra a rotina de manutenção e fecha o pool.

## Configuração do PACS / Orthanc

Tela **Administração → Configurações → PACS / Orthanc** (só ADMIN, verificado no
servidor). Guarda nome da conexão, URL base, usuário, credencial, endpoint
DICOMweb, timeout e verificação TLS.

**Guardar a configuração não conecta a nada.** Depois de salvar, o botão
"Testar conexão" chama `POST /api/admin/settings/orthanc/test`. Somente esse
fluxo consulta `GET /system` no destino salvo. Exige sessão ADMIN e CSRF.
Detalhes, limites de SSRF, TLS e testes: [ORTHANC_CONNECTION.md](ORTHANC_CONNECTION.md).

### Credencial

- Cifrada em repouso com **AES-256-GCM** (criptografia autenticada da biblioteca
  padrão), com nonce aleatório por gravação e um contexto autenticado
  (`settings:orthanc:credential`) que impede reaproveitar o valor cifrado em
  outro propósito.
- A chave mestra vem de `PACS_MASTER_KEY`, só do ambiente: não fica no
  PostgreSQL, não vai para o frontend, não é hardcoded, não entra no Git e não
  aparece em log.
- **A credencial nunca volta ao navegador.** A resposta traz apenas
  `credentialConfigured: true/false`; a tela mostra `••••••••••••` com as ações
  "Alterar credencial" e "Remover".
- **Salvar as outras configurações não apaga a credencial**: o campo `credential`
  ausente no PUT preserva o que está guardado; string vazia remove.
- Sem `PACS_MASTER_KEY`, a configuração ainda é salva, mas gravar credencial
  responde 503 com mensagem sanitizada — a resposta não cita o nome da variável.
- Trocar a chave torna ilegíveis as credenciais já cifradas: precisam ser
  cadastradas de novo.

Gerar uma chave:

```bash
go run ./cmd/server keygen          # ou: docker compose run --rm app pacs-server keygen
```

### URL e SSRF

A URL é fornecida pelo administrador e será acessada pelo backend, então é
tratada como risco de SSRF desde já. A validação rejeita esquema diferente de
http/https, ausência de host, credencial embutida na URL, query string,
fragmento, porta inválida e travessia de caminho no endpoint DICOMweb. Não existe
proxy genérico, e só ADMIN grava e testa. O cliente valida os IPs resolvidos antes
de conectar, permite redes privadas e bloqueia destinos especiais, redirects e
proxies de ambiente. Consulte a política detalhada no documento do teste.

### Auditoria

Toda gravação registra `ORTHANC_SETTINGS_CHANGED` com o autor, a origem e **quais
campos mudaram** — nunca valores de credencial. Alterar a configuração zera o
status de verificação anterior.

O teste registra `ORTHANC_CONNECTION_TESTED` com uma categoria operacional
sanitizada. Status e data/hora são persistidos no JSONB existente, sem migration.

## Empacotamento

Uma imagem única (`Dockerfile` na raiz) com frontend e backend: o build compila
o SPA, compila o binário Go e a imagem final carrega os dois. O backend serve os
arquivos estáticos na mesma origem da API, então não há CORS, o cookie de sessão
fica simples e existe um só container para o proxy da Prefeitura encaminhar.

- Build multi-stage: Node (build do SPA) → Go (binário estático) → Alpine.
- Imagem final de ~31 MB, rodando como usuário sem privilégios (uid 10001).
- `STATIC_DIR=/srv/frontend` e `HTTP_ADDR=0.0.0.0:8080` já vêm na imagem; o resto
  da configuração vem do ambiente em execução — nunca da imagem.
- `HEALTHCHECK` consulta `/health`.
- As migrations vão embutidas no binário e são aplicadas na subida.

```bash
docker build -t pacs-web:latest .

# Aplicação + banco, em rede e volume próprios deste projeto
cp .env.compose.example .env      # preencha POSTGRES_PASSWORD
docker compose up -d --build

# Primeiro administrador, dentro do container
docker compose exec app pacs-server admin create -name "<nome>" -username <username>
```

`docker-compose.yml` publica a aplicação apenas em `127.0.0.1`: quem expõe para a
rede é o proxy já existente, que também termina o TLS. **Não conecte este compose
à rede do PACS em produção** e não aponte `DATABASE_URL` para o PostgreSQL
interno do Orthanc.

## Execução local

Para desenvolver com recarga automática do frontend, o caminho é o Vite com
proxy (a imagem única serve o build, não o dev server):

```bash
# 1. PostgreSQL de desenvolvimento, isolado (porta 55432, rede e volume próprios)
docker compose -f docker-compose.dev.yml up -d

# 2. Configuração do backend
cd backend && cp .env.example .env    # ajuste DATABASE_URL

# 3. Migrations
go run ./cmd/server migrate

# 4. Primeiro administrador (a senha é pedida no terminal, sem eco)
go run ./cmd/server admin create -name "Administrador Teste" -username admin.teste

# 5. Backend
go run ./cmd/server serve

# 6. Frontend, em outro terminal
cd frontend && npm run dev      # http://localhost:5173, com proxy para o backend
```

## Bootstrap do primeiro administrador

```
pacs-server admin create -name "<nome>" -username <username> [-email <e-mail>] [-unit <slug>] [-valid-until AAAA-MM-DD]
```

- A senha é pedida no terminal, **sem eco e com confirmação**. Em automação,
  pode vir pela entrada padrão (`... < arquivo`).
- **Não existe flag de senha**, de propósito: argumentos aparecem em `ps` e no
  histórico do shell.
- Mínimo de 12 caracteres. Nenhuma credencial padrão é criada em nenhum momento.
- O comando **recusa** se já houver um ADMIN: daí em diante, novos usuários
  passam pelos fluxos autorizados da aplicação.
- A senha escolhida não é registrada em log, nesta documentação nem em lugar
  algum do repositório.

## Testes

```bash
cd backend
gofmt -l .        # nenhuma saída = tudo formatado
go vet ./...
go test ./...
go build ./...
```

Cobertura desta etapa: Argon2id (hash, verificação, sal, formato PHC, rehash,
hash corrompido, política de senha), sessão (token e hash correspondentes,
revogação, prazo absoluto, inatividade), serviço de login (sucesso, senha
errada, usuário inexistente, conta inativa, validade expirada, ausência de
enumeração, atualização de hash, logout idempotente, sessão de conta desativada),
perfis (matriz de criação e gestão), validação de usuário, configuração,
carregamento de migrations, e a API por `httptest` (fluxo completo com cookies e
CSRF, atributos do cookie, respostas sem dado sensível, 401 sem sessão, 403 por
perfil, CSRF ausente/divergente/origem externa, rate limit, health, 404 e
cabeçalhos de segurança).

Os testes não precisam de PostgreSQL: usam as implementações em memória de
`internal/authtest`.

## Limitações atuais

- A Worklist READ-ONLY está descrita em [WORKLIST.md](WORKLIST.md); não oferece viewer ou acesso a pixels.
- `/api/admin/ping` é provisória.
- O teste confirma somente a resposta de `/system`; não valida acesso a exames.
- Não há rotação de chave mestra: trocar `PACS_MASTER_KEY` exige recadastrar as
  credenciais.
- Não há comando de redefinição de senha (`admin reset-password`).
- Rate limit por processo e em memória (ver acima).
- `X-Forwarded-For` não é considerado: atrás de proxy, o IP registrado é o da
  conexão. Quando houver proxy, isso precisa de configuração explícita.
- A auditoria de logout não registra origem (o IP não é propagado até ali).
- Sem DICOMweb ou Cornerstone3D.
- Sem golangci-lint: `gofmt` e `go vet` bastam para este tamanho.

## Próxima etapa (aguardando validação)

Revisar a Worklist READ-ONLY e validar manualmente os filtros/paginação no
ambiente controlado pelo responsável pelo PACS. Nenhuma chamada real foi feita
durante o desenvolvimento desta entrega.
As demais pendências da auditoria permanecem fora desta entrega.

## Execução em container (preparado, fora de uso na fase de desenvolvimento)

Durante o desenvolvimento, backend e frontend rodam direto na máquina e só o
PostgreSQL fica em container — veja o `README.md`. O arranjo abaixo existe para
quando a produção for definida:

```bash
cp .env.compose.example .env      # preencha POSTGRES_PASSWORD
docker compose up -d --build
docker compose exec -T app pacs-server admin create -name "<nome>" -username <username> < arquivo-com-a-senha
```

A aplicação fica em `http://127.0.0.1:8080` — frontend e API na mesma origem.
Para desenvolvimento local sem HTTPS, o `.env` precisa de `APP_ENV=development` e
`COOKIE_SECURE=false`; com `production`, o cookie exige HTTPS e o login não
completa em `http://localhost`.

Neste arranjo o frontend servido é o **build**: alterar `frontend/src/` exige
`docker compose up -d --build`. Para recarga automática, use o
`docker-compose.dev.yml` (só o PostgreSQL) com `npm run dev` e o backend fora do
container.

Operação:

```bash
docker compose ps
docker compose logs -f app
docker compose restart app
docker compose down          # para tudo, preservando o volume do banco
docker compose down -v       # APAGA o banco — irreversível
```
