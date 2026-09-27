# PACS Web Municipal — imagem única com frontend e backend.
#
# O frontend é compilado e servido como arquivos estáticos pelo próprio backend
# Go, na mesma origem da API. Assim não há CORS, o cookie de sessão fica simples
# (SameSite=Lax) e existe um só container para publicar.
#
# Este container é isolado: não se conecta a nenhuma rede do PACS em produção.

# ── 1. Frontend ──────────────────────────────────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /app

# Dependências primeiro, para aproveitar cache de camada.
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

COPY frontend/tsconfig.json frontend/tsconfig.app.json frontend/tsconfig.node.json ./
COPY frontend/vite.config.ts frontend/index.html ./
COPY frontend/src ./src

# Roda typecheck e build (npm run build = tsc --build && vite build).
RUN npm run build

# ── 2. Backend ───────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS backend

WORKDIR /src

COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ ./

# Binário estático, sem informação de debug. As migrations vão embutidas.
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/pacs-server ./cmd/server

# ── 3. Imagem final ──────────────────────────────────────────────────────────
FROM alpine:3.22

# Certificados para chamadas HTTPS de saída; tzdata para os horários locais.
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 pacs

COPY --from=backend  /out/pacs-server /usr/local/bin/pacs-server
COPY --from=frontend /app/dist       /srv/frontend

# STATIC_DIR faz o backend servir o SPA na mesma origem da API.
# As demais variáveis (DATABASE_URL, APP_ENV, COOKIE_SECURE…) vêm do ambiente
# em tempo de execução — nunca da imagem.
ENV STATIC_DIR=/srv/frontend \
    HTTP_ADDR=0.0.0.0:8080

USER pacs
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/health >/dev/null 2>&1 || exit 1

ENTRYPOINT ["/usr/local/bin/pacs-server"]
CMD ["serve"]
