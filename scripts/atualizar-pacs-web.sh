#!/usr/bin/env bash
# Atualiza o PACS Web na VM a partir do GitHub (branch main).
#
# Fluxo: verificações → git fetch → resumo das mudanças → confirmação →
# imagem atual marcada para rollback → git merge --ff-only → build do app →
# (migrations, somente com --com-migrations) → reinicia (recria) o container app →
# readiness. Se o app novo não ficar pronto, volta código e imagem anteriores.
#
# Reinicia somente containers DESTE projeto Compose (pacs-web). Nunca reinicia o
# serviço Docker da VM: isso derrubaria Orthanc/DICOM 4242, stack legado e NPM.
# Não altera volumes, redes, Orthanc, NPM nem firewall.
# Nunca executa `docker compose config` sem --quiet (imprimiria segredos).
#
# Uso (na VM, como usuário com acesso ao Docker):
#   scripts/atualizar-pacs-web.sh                  # interativo
#   scripts/atualizar-pacs-web.sh --sim            # sem perguntar (exceto migrations)
#   scripts/atualizar-pacs-web.sh --com-migrations # janela controlada com migrations
#   scripts/atualizar-pacs-web.sh --reiniciar      # reinicia o app mesmo sem versão nova
#                                                  # (ex.: após editar o .env)
#   scripts/atualizar-pacs-web.sh --reiniciar-tudo # reinicia app + PostgreSQL do PACS Web
#
# Variáveis opcionais:
#   PACS_WEB_DIR     diretório do repositório (padrão: pasta acima deste script)
#   PACS_WEB_BRANCH  branch remota (padrão: main)
#   HEALTH_TIMEOUT   segundos aguardando readiness (padrão: 120)
#   BACKUP_CMD       backup do banco antes de migrations
#                    (padrão: /usr/local/sbin/pacs-web-db-backup.sh)
#   MIGRATION_DATABASE_URL  DSN da role de migration; se ausente, é pedida sem eco
set -Eeuo pipefail

BRANCH="${PACS_WEB_BRANCH:-main}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-120}"
BACKUP_CMD="${BACKUP_CMD:-/usr/local/sbin/pacs-web-db-backup.sh}"
REPO="${PACS_WEB_DIR:-$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)}"
IMAGE="pacs-web"
ASSUME_YES=false
WITH_MIGRATIONS=false
RESTART=app   # app | all; --reiniciar força reinício mesmo sem versão nova
FORCE_RESTART=false

for arg in "$@"; do
  case "$arg" in
    --sim|-y) ASSUME_YES=true ;;
    --com-migrations) WITH_MIGRATIONS=true ;;
    --reiniciar) FORCE_RESTART=true ;;
    --reiniciar-tudo) FORCE_RESTART=true; RESTART=all ;;
    -h|--ajuda|--help) sed -n '2,31p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "Opção desconhecida: $arg (use --ajuda)" >&2; exit 2 ;;
  esac
done

log()  { printf '[%s] %s\n' "$(date '+%F %T')" "$*"; }
fail() { log "ERRO: $*" >&2; exit 1; }
confirm() {
  # $1 = pergunta; $2 = "sempre" para perguntar mesmo com --sim
  if $ASSUME_YES && [[ "${2:-}" != sempre ]]; then return 0; fi
  local answer
  read -r -p "$1 [s/N] " answer </dev/tty
  [[ "$answer" =~ ^[sS]$ ]]
}

cd "$REPO"
[[ -f docker-compose.yml && -d .git ]] || fail "repositório PACS Web não encontrado em $REPO"
[[ -f .env ]] || fail ".env ausente em $REPO; o Compose de produção depende dele"
command -v docker >/dev/null || fail "docker não encontrado"
docker compose version >/dev/null 2>&1 || fail "docker compose v2 não disponível"

# Uma execução por vez.
exec 9>"$REPO/.git/pacs-web-update.lock"
flock -n 9 || fail "outra atualização já está em andamento"

current_branch="$(git rev-parse --abbrev-ref HEAD)"
[[ "$current_branch" == "$BRANCH" ]] || fail "branch atual é '$current_branch'; esperado '$BRANCH'"
# Código não deve ser alterado direto no servidor.
[[ -z "$(git status --porcelain --untracked-files=no)" ]] || fail "há alterações locais em arquivos versionados; resolva antes (git status)"

wait_ready() {
  local deadline=$((SECONDS + HEALTH_TIMEOUT))
  while (( SECONDS < deadline )); do
    if docker compose exec -T app wget -qO- http://127.0.0.1:8080/health/ready >/dev/null 2>&1; then
      return 0
    fi
    sleep 3
  done
  return 1
}

# Recria containers deste projeto para aplicar imagem e .env atuais.
restart_services() {
  if [[ "$RESTART" == all ]]; then
    log "Reiniciando app e PostgreSQL do PACS Web (indisponibilidade breve)…"
    docker compose up -d --no-build --force-recreate
  else
    log "Reiniciando o container app…"
    docker compose up -d --no-deps --no-build --force-recreate app
  fi
}

log "Buscando atualizações de origin/${BRANCH}…"
git fetch --quiet origin "$BRANCH"
OLD="$(git rev-parse HEAD)"
NEW="$(git rev-parse "origin/$BRANCH")"
# Commit cuja imagem foi implantada por este script; detecta `git pull` manual sem build.
DEPLOYED_FILE="$REPO/.git/pacs-web-deployed-commit"
DEPLOYED="$(cat "$DEPLOYED_FILE" 2>/dev/null || true)"
if [[ "$OLD" == "$NEW" && "$DEPLOYED" != "$OLD" ]]; then
  if [[ -n "$DEPLOYED" ]] && git cat-file -e "$DEPLOYED^{commit}" 2>/dev/null && git merge-base --is-ancestor "$DEPLOYED" "$OLD"; then
    log "Código (${OLD:0:7}) está à frente da última implantação (${DEPLOYED:0:7}): a imagem será reconstruída."
    OLD="$DEPLOYED"
  else
    log "Sem registro da última implantação: a imagem será reconstruída a partir de ${OLD:0:7}."
    log "Migrations pendentes, se houver, fazem o app novo recusar subir e o rollback é automático."
    FORCE_REBUILD=true
  fi
fi
if [[ "$OLD" == "$NEW" && "${FORCE_REBUILD:-false}" != true ]]; then
  log "Já está na versão mais recente (${OLD:0:7})."
  $FORCE_RESTART || { log "Nada a fazer. Use --reiniciar para reiniciar mesmo assim."; exit 0; }
  if [[ "$RESTART" == all ]]; then confirm "Reiniciar app e PostgreSQL do PACS Web agora?" || fail "cancelado pelo operador"; fi
  docker compose config --quiet || fail "docker-compose.yml/.env inválidos"
  restart_services
  log "Aguardando readiness (até ${HEALTH_TIMEOUT}s)…"
  if wait_ready; then log "Reinício concluído: ${OLD:0:7} em execução e pronta."; exit 0; fi
  docker compose logs --tail=40 app || true
  fail "app não ficou pronto após reiniciar; verifique .env e logs (docker compose logs app)"
fi
git merge-base --is-ancestor "$OLD" "$NEW" || fail "origin/$BRANCH não é fast-forward do HEAD local; intervenção manual necessária"

echo
log "Versão atual: ${OLD:0:7} → nova: ${NEW:0:7}"
git --no-pager log --oneline --no-decorate "$OLD..$NEW"
echo
git --no-pager diff --stat "$OLD" "$NEW" | tail -n 1
echo

# Migrations aplicadas são imutáveis; novas exigem janela controlada.
changed_migrations="$(git diff --name-status "$OLD" "$NEW" -- backend/migrations/ | grep '\.sql$' || true)"
if [[ -n "$changed_migrations" ]] && grep -qv '^A' <<<"$changed_migrations"; then
  fail "migration existente foi alterada/removida — migrations aplicadas são imutáveis:
$changed_migrations"
fi
if [[ -n "$changed_migrations" ]]; then
  log "Novas migrations nesta atualização:"
  echo "$changed_migrations"
  $WITH_MIGRATIONS || fail "rode novamente com --com-migrations, em janela controlada, com backup e plano de rollback"
elif $WITH_MIGRATIONS; then
  log "--com-migrations informado, mas não há migrations novas; seguindo sem migrations."
  WITH_MIGRATIONS=false
fi

infra_changes="$(git diff --name-only "$OLD" "$NEW" -- docker-compose.yml Dockerfile .env.compose.example .dockerignore)"
if [[ -n "$infra_changes" ]]; then
  log "ATENÇÃO: mudanças de infraestrutura nesta atualização:"
  echo "$infra_changes"
  echo "Revise com: git diff ${OLD:0:7} ${NEW:0:7} -- $(tr '\n' ' ' <<<"$infra_changes")"
  confirm "As mudanças de infraestrutura foram revisadas e autorizadas?" sempre || fail "cancelado pelo operador"
fi

confirm "Aplicar a atualização agora?" || fail "cancelado pelo operador"

docker compose config --quiet || fail "docker-compose.yml/.env inválidos"

# Marca a imagem em execução para rollback.
ROLLBACK_TAG=""
if docker image inspect "$IMAGE:latest" >/dev/null 2>&1; then
  ROLLBACK_TAG="rollback-${OLD:0:7}"
  docker image tag "$IMAGE:latest" "$IMAGE:$ROLLBACK_TAG"
  log "Imagem atual preservada como $IMAGE:$ROLLBACK_TAG"
else
  log "Aviso: imagem $IMAGE:latest não encontrada; rollback de imagem indisponível."
fi

MIGRATED=false
# Antes de reiniciar o app: volta código e tag latest; o container atual nunca parou.
undo_build() {
  git reset --quiet --keep "$OLD"
  if [[ -n "$ROLLBACK_TAG" ]]; then docker image tag "$IMAGE:$ROLLBACK_TAG" "$IMAGE:latest"; fi
}
rollback() {
  log "Iniciando rollback para ${OLD:0:7}…"
  git reset --quiet --keep "$OLD" || log "Falha ao voltar o código; verifique git status manualmente."
  if $MIGRATED; then
    log "ATENÇÃO: migrations já foram aplicadas. A versão anterior recusa schema desconhecido;"
    log "o rollback exige restaurar o backup do banco. O app NÃO foi reiniciado automaticamente."
    return 1
  fi
  if [[ -n "$ROLLBACK_TAG" ]]; then
    docker image tag "$IMAGE:$ROLLBACK_TAG" "$IMAGE:latest"
    docker compose up -d --no-deps --no-build app
    if wait_ready; then log "Rollback concluído: versão ${OLD:0:7} em execução."; else log "Rollback executado, mas o app ainda não respondeu pronto. Verifique: docker compose logs --tail=100 app"; fi
  fi
}

log "Atualizando código (fast-forward)…"
git merge --quiet --ff-only "$NEW"

log "Construindo imagem (frontend + backend)…"
if ! docker compose build app; then
  undo_build
  fail "build da nova versão falhou; código voltou para ${OLD:0:7}"
fi

if $WITH_MIGRATIONS; then
  log "Migrations: backup obrigatório antes de aplicar."
  [[ -x "$BACKUP_CMD" ]] || { undo_build; fail "script de backup não encontrado/executável: $BACKUP_CMD"; }
  "$BACKUP_CMD" || { undo_build; fail "backup falhou; nada foi migrado"; }
  confirm "Backup concluído e conferido. Aplicar migrations agora?" sempre || { undo_build; fail "cancelado antes das migrations"; }
  if [[ -z "${MIGRATION_DATABASE_URL:-}" ]]; then
    read -r -s -p "MIGRATION_DATABASE_URL (role de migration, sem eco): " MIGRATION_DATABASE_URL </dev/tty; echo
  fi
  export MIGRATION_DATABASE_URL
  # Valor passado pelo ambiente (-e NOME), nunca na linha de comando.
  if ! docker compose run --rm --no-deps -e MIGRATION_DATABASE_URL app migrate; then
    unset MIGRATION_DATABASE_URL
    undo_build
    fail "migrate falhou (cada migration roda em transação). App atual segue em execução; confira schema_migrations."
  fi
  unset MIGRATION_DATABASE_URL
  MIGRATED=true
  log "Migrations aplicadas. Reaplique os grants mínimos da role runtime aos objetos novos (docs/HARDENING.md)."
  confirm "Grants da role runtime reaplicados? Iniciar a nova versão?" sempre || fail "parado após migrations; nova versão NÃO iniciada. Inicie com: docker compose up -d --no-deps --no-build app"
fi

restart_services

log "Aguardando readiness (até ${HEALTH_TIMEOUT}s)…"
if wait_ready; then
  echo "$NEW" >"$DEPLOYED_FILE"
  log "Atualização concluída: ${NEW:0:7} em execução e pronta."
  log "Conferir manualmente: login, Worklist e abertura de um estudo no Viewer."
  if [[ -n "$ROLLBACK_TAG" ]]; then log "Rollback manual, se necessário: imagem $IMAGE:$ROLLBACK_TAG + git reset --keep ${OLD:0:7}"; fi
else
  log "Nova versão não ficou pronta em ${HEALTH_TIMEOUT}s. Últimas linhas do log:"
  docker compose logs --tail=40 app || true
  rollback || true
  fail "atualização revertida"
fi
