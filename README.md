# PACS Web Municipal

Nova aplicação web do PACS da Secretaria Municipal de Saúde de Francisco Beltrão.
Frontend React + TypeScript, backend Go, PostgreSQL próprio.

O PACS em produção (Orthanc, OHIF, Keycloak, oauth2-proxy, proxy, NFS/TrueNAS)
permanece independente. A integração READ-ONLY oferece o teste manual
`GET /system`, a Worklist paginada e o Viewer mínimo, usando a configuração salva pelo ADMIN.
Salvar a configuração não conecta ao Orthanc. Abrir Exames consulta a Worklist.

- Design: `docs/DECISOES.md` (decisões por entrega)
- Backend: `docs/BACKEND.md` (arquitetura, segurança, endpoints)
- Teste Orthanc: `docs/ORTHANC_CONNECTION.md` (fluxo, TLS, SSRF e testes fictícios)
- Worklist: `docs/WORKLIST.md` (contrato, paginação, modalidades e privacidade)
- Viewer: `docs/VIEWER.md` (gateway autenticado, Cornerstone, limites e testes)

## Rodar em desenvolvimento

Backend e frontend rodam **direto na máquina**. Só o PostgreSQL fica em
container, porque não há servidor PostgreSQL instalado localmente.

Precisa de: Go 1.26+, Node 22+, Docker.

### 1. PostgreSQL (uma vez, fica rodando)

```bash
docker compose -f docker-compose.dev.yml up -d
```

Sobe em `127.0.0.1:55432`, em rede e volume próprios deste projeto. Os dados
sobrevivem a reinícios. Para parar: `docker compose -f docker-compose.dev.yml stop`.

### 2. Configuração do backend

```bash
cd backend
cp .env.example .env     # ajuste DATABASE_URL se mudou a porta
go run ./cmd/server keygen   # copie a saída para PACS_MASTER_KEY no .env
```

`backend/.env` é ignorado pelo Git. Nunca comite `.env`.

### 3. Backend

```bash
cd backend
set -a && . ./.env && set +a      # carrega as variáveis no shell
go run ./cmd/server serve
```

Sobe em `http://127.0.0.1:8080`. As migrations são aplicadas na subida.

No zsh/bash o `set -a` é necessário porque o Go lê a configuração do ambiente,
não do arquivo. Em um só comando:

```bash
cd backend && env $(grep -v '^#' .env | grep -v '^$' | xargs) go run ./cmd/server serve
```

### 4. Primeiro administrador (uma vez por banco)

```bash
cd backend
set -a && . ./.env && set +a
go run ./cmd/server admin create -name "Seu Nome" -username seu_usuario
```

A senha é pedida no terminal, sem eco, com confirmação. Mínimo 12 caracteres.
Não existe flag de senha: argumento apareceria em `ps` e no histórico.

### 5. Frontend

Em outro terminal:

```bash
cd frontend
npm install     # só na primeira vez
npm run dev
```

Abra **http://localhost:5173**. O Vite faz proxy de `/api` e `/health` para o
backend, então navegador e API compartilham a origem — sem CORS, e o cookie de
sessão funciona normalmente.

Alterações em `frontend/src/` recarregam sozinhas. Alterações no Go exigem reiniciar o
`go run`.

## Verificações

```bash
# Backend
cd backend
gofmt -l .        # nenhuma saída = formatado
go vet ./...
go test ./...
go build ./...

# Frontend
cd frontend
npm run typecheck
node --test tests/*.test.mjs
npm run build
```

Os testes Go não precisam do PostgreSQL.

## Produção (ainda não definida)

Existem `Dockerfile` (imagem única com frontend e backend na mesma origem) e
`docker-compose.yml` (aplicação + banco), preparados mas **não em uso**. A decisão
de empacotamento para produção fica para depois da fase de desenvolvimento. Veja
"Empacotamento" em `docs/BACKEND.md`.

## Estrutura

```
frontend/            frontend React + TypeScript
  src/
    api/             cliente HTTP do backend
    auth/            sessão no cliente
    app/             shell da aplicação (sidebar + header)
    screens/         telas
    design-system/   tokens Industry, ícones, moldura blueprint
    mocks/           dados fictícios dos protótipos; não alimentam a Worklist
    dev/             andaime de desenvolvimento (só em DEV)
  index.html, vite.config.ts, tsconfig*.json, package.json

backend/             backend Go
  cmd/server/        serve | migrate | admin create | keygen
  internal/          config, database, user, auth, audit, settings, secrets, studies, viewer, orthanc, httpapi
  migrations/        SQL versionado, embutido no binário
  .env.example

docs/                documentação
docker-compose.dev.yml   PostgreSQL de desenvolvimento
Dockerfile               imagem única (para produção, futura)
docker-compose.yml       aplicação + banco (para produção, futura)
```
