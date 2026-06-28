#!/usr/bin/env bash
set -Eeuo pipefail

# Deploy ONLY the new-api app container, connecting to an EXTERNAL database and
# Redis managed elsewhere (e.g. 1Panel's MySQL/PostgreSQL/Redis apps).
#
# Unlike bin/docker-local.sh, this script never starts its own PostgreSQL/Redis.
# You must provide SQL_DSN and REDIS_CONN_STRING pointing at your 1Panel services.

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${ROOT_DIR}"

# ============================ DEPLOYMENT CONFIG ============================
# Only the (non-secret) docker network lives here. The DB/Redis credentials
# are kept OUT of this script: put them in the secrets file (gitignored)
#
#   data/docker-1panel/.env.generated
#
# as SQL_DSN / REDIS_CONN_STRING. On a new host, either run once with those
# two passed as env vars (they get persisted automatically), or create the
# file by hand:
#
#   mkdir -p data/docker-1panel && umask 077
#   cat > data/docker-1panel/.env.generated <<'EOF'
#   SQL_DSN='postgresql://user:pass@DB_CONTAINER:5432/dbname'
#   REDIS_CONN_STRING='redis://:pass@REDIS_CONTAINER:6379/0'
#   EOF
#
# Each value can still be overridden per-invocation via the same-named env var.
DEPLOY_EXTERNAL_NETWORK="1panel-network"
DEPLOY_SQL_DSN=""
DEPLOY_REDIS_CONN_STRING=""
# ==========================================================================

PROJECT_NAME="${PROJECT_NAME:-new-api}"
IMAGE_NAME="${IMAGE_NAME:-new-api:1panel}"
CONTAINER_NAME="${CONTAINER_NAME:-${PROJECT_NAME}}"
APP_DATA_VOLUME="${APP_DATA_VOLUME:-${PROJECT_NAME}-app-data}"

# Derive a "-previous" tag from IMAGE_NAME so each rebuild keeps the prior image
# for one-command rollback (see the `rollback` action).
if [[ "${IMAGE_NAME}" == *:* ]]; then
  IMAGE_REPO="${IMAGE_NAME%:*}"
  IMAGE_TAG="${IMAGE_NAME##*:}"
else
  IMAGE_REPO="${IMAGE_NAME}"
  IMAGE_TAG="latest"
fi
PREVIOUS_IMAGE="${PREVIOUS_IMAGE:-${IMAGE_REPO}:${IMAGE_TAG}-previous}"

# Update behaviour
GIT_PULL="${GIT_PULL:-1}"            # `update` runs `git pull` first; set 0 to skip
GIT_REMOTE="${GIT_REMOTE:-origin}"
GIT_REF="${GIT_REF:-}"              # optional branch/tag to update to (default: current branch)
HEALTHCHECK="${HEALTHCHECK:-1}"     # poll /api/status after start; set 0 to skip
HEALTHCHECK_RETRIES="${HEALTHCHECK_RETRIES:-60}"

# Network to attach the app container to. Set this to the docker network that
# your 1Panel database/redis containers live on (commonly "1panel-network") so
# the app can reach them by container name in SQL_DSN / REDIS_CONN_STRING.
# Leave empty to create a dedicated network; in that case connect to the DB via
# a host-exposed port using host.docker.internal (mapped to host-gateway below).
EXTERNAL_NETWORK="${EXTERNAL_NETWORK:-${DEPLOY_EXTERNAL_NETWORK}}"
OWN_NETWORK_NAME="${OWN_NETWORK_NAME:-${PROJECT_NAME}-network}"

HOST_BIND_IP="${HOST_BIND_IP:-127.0.0.1}"
HOST_PORT="${HOST_PORT:-${PORT:-56781}}"
APP_PORT="${APP_PORT:-3000}"
LOCAL_TZ="${TZ:-Asia/Shanghai}"
ENV_FILE="${ENV_FILE:-}"
FOLLOW_LOGS="${FOLLOW_LOGS:-0}"
NO_CACHE="${NO_CACHE:-0}"
PLATFORM="${PLATFORM:-}"
ACTION="${1:-up}"

STATE_DIR="${STATE_DIR:-${ROOT_DIR}/data/docker-1panel}"
LOG_DIR="${LOG_DIR:-${ROOT_DIR}/logs/docker-1panel}"
SECRETS_FILE="${SECRETS_FILE:-${STATE_DIR}/.env.generated}"

# External services — taken from the DEPLOYMENT CONFIG block above; overridable by env.
SQL_DSN="${SQL_DSN:-${DEPLOY_SQL_DSN}}"
REDIS_CONN_STRING="${REDIS_CONN_STRING:-${DEPLOY_REDIS_CONN_STRING}}"
LOG_SQL_DSN="${LOG_SQL_DSN:-}"

SESSION_SECRET="${SESSION_SECRET:-}"
CRYPTO_SECRET="${CRYPTO_SECRET:-}"
NODE_NAME="${NODE_NAME:-${PROJECT_NAME}-node-1}"
BUILD_ON_UP="${BUILD_ON_UP:-1}"
FRONTEND_BUILD_GC_HEAP_SIZE="${FRONTEND_BUILD_GC_HEAP_SIZE:-536870912}"
if [[ -z "${FRONTEND_THEME+x}" ]]; then
  FRONTEND_THEME="classic"
fi

usage() {
  cat <<'USAGE'
Usage:
  bash bin/docker-1panel.sh [up|update|rollback|build|run|stop|logs|status|clean|help]

Actions:
  up        Build the app image, then start ONLY new-api, connected to your
            external (1Panel-managed) database and Redis. (default)
  update    Source-deploy update: git pull -> rebuild image -> recreate app.
            Reuses the persisted DSN/secrets; no env needed after first run.
            The previous image is kept for rollback. DB schema migrations run
            automatically on app startup.
  rollback  Restore the previous image (kept from the last build) and restart.
  build     Build the app image only.
  run       (Re)create the app container from the existing image.
  stop      Remove the app container (external DB/Redis untouched).
  logs      Follow app logs.
  status    Show the app container.
  clean     Remove the app container/image (and volume with KEEP_VOLUMES=0).

This script does NOT start PostgreSQL/MySQL or Redis. Provide these once and
they are persisted in ./data/docker-1panel/.env.generated:

  SQL_DSN              (required) connection string to your 1Panel database
    PostgreSQL: postgresql://user:pass@HOST:5432/new-api
    MySQL:      user:pass@tcp(HOST:3306)/new-api?parseTime=true
  REDIS_CONN_STRING    (required) e.g. redis://:pass@HOST:6379/0

Where HOST is:
  - the 1Panel DB/redis container name, if you set EXTERNAL_NETWORK to the
    network those containers are on (recommended); or
  - host.docker.internal, if the DB/redis ports are exposed on the host and
    you leave EXTERNAL_NETWORK empty.

Common environment overrides:
  EXTERNAL_NETWORK=1panel-network   Attach app to this existing docker network
  PROJECT_NAME=new-api              Prefix for container/network/volume
  IMAGE_NAME=new-api:1panel         Docker image tag for the app
  HOST_BIND_IP=127.0.0.1            Host bind address (use a reverse proxy)
  HOST_PORT=56781                   Host port for new-api (reverse proxy upstream)
  BUILD_ON_UP=0                     Skip docker build during up
  NO_CACHE=1                        Build without Docker cache
  PLATFORM=linux/amd64             Optional docker build --platform value
  FOLLOW_LOGS=1                     Follow app logs after starting
  ENV_FILE=.env.local               Optional extra env file for the app
  FRONTEND_THEME=classic            Frontend theme baked at build (default|classic; empty to skip)
  LOG_SQL_DSN=...                   Optional separate logs database DSN

Update overrides:
  GIT_PULL=0                        Skip `git pull` during update (rebuild current source)
  GIT_REF=v1.2.3                    Branch/tag to update to (default: current branch)
  GIT_REMOTE=origin                 Git remote to fetch/pull from
  HEALTHCHECK=0                     Skip the post-start /api/status health check
  HEALTHCHECK_RETRIES=60            Health-check attempts (2s apart)

Advanced overrides:
  SESSION_SECRET=...                Override generated session secret
  CRYPTO_SECRET=...                 Override generated crypto secret

Examples:
  EXTERNAL_NETWORK=1panel-network \
  SQL_DSN='postgresql://newapi:pass@new-api-postgres:5432/new-api' \
  REDIS_CONN_STRING='redis://:pass@new-api-redis:6379/0' \
  bash bin/docker-1panel.sh up

  # subsequent runs reuse persisted secrets/DSN:
  bash bin/docker-1panel.sh up
  bash bin/docker-1panel.sh logs
  bash bin/docker-1panel.sh stop
  bash bin/docker-1panel.sh clean

  # version update (source deploy) and rollback:
  bash bin/docker-1panel.sh update
  GIT_REF=v1.2.3 bash bin/docker-1panel.sh update
  bash bin/docker-1panel.sh rollback
USAGE
}

log() {
  printf '\033[1;34m==>\033[0m %s\n' "$*"
}

warn() {
  printf '\033[1;33mWARN\033[0m %s\n' "$*" >&2
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1" >&2
    exit 1
  fi
}

random_secret() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 32
  else
    LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 64
    printf '\n'
  fi
}

quote_env_value() {
  printf "%s" "$1" | sed "s/'/'\\''/g"
}

mask_secrets() {
  # Hide passwords in DSN-like strings for logging.
  sed -E 's#(://[^:/@]*:)[^@]*@#\1***@#g; s#(password=)[^ ]*#\1***#g' <<<"$1"
}

load_secrets_file() {
  if [[ -f "${SECRETS_FILE}" ]]; then
    # shellcheck disable=SC1090
    source "${SECRETS_FILE}"
  fi
}

save_secrets_file() {
  mkdir -p "${STATE_DIR}"
  umask 077
  cat >"${SECRETS_FILE}" <<EOF
SQL_DSN='$(quote_env_value "${SQL_DSN}")'
REDIS_CONN_STRING='$(quote_env_value "${REDIS_CONN_STRING}")'
LOG_SQL_DSN='$(quote_env_value "${LOG_SQL_DSN}")'
SESSION_SECRET='$(quote_env_value "${SESSION_SECRET}")'
CRYPTO_SECRET='$(quote_env_value "${CRYPTO_SECRET}")'
EOF
}

ensure_secrets() {
  # Env-provided values take priority; otherwise fall back to persisted file.
  local env_sql_dsn="${SQL_DSN}"
  local env_redis="${REDIS_CONN_STRING}"
  local env_log_dsn="${LOG_SQL_DSN}"
  load_secrets_file
  SQL_DSN="${env_sql_dsn:-${SQL_DSN:-}}"
  REDIS_CONN_STRING="${env_redis:-${REDIS_CONN_STRING:-}}"
  LOG_SQL_DSN="${env_log_dsn:-${LOG_SQL_DSN:-}}"

  if [[ -z "${SQL_DSN}" || -z "${REDIS_CONN_STRING}" ]]; then
    echo "SQL_DSN and REDIS_CONN_STRING are required (pass via env on first run)." >&2
    echo "See: bash bin/docker-1panel.sh help" >&2
    exit 1
  fi

  SESSION_SECRET="${SESSION_SECRET:-$(random_secret)}"
  CRYPTO_SECRET="${CRYPTO_SECRET:-$(random_secret)}"
  save_secrets_file
}

container_exists() {
  docker inspect "$1" >/dev/null 2>&1
}

container_running() {
  [[ "$(docker inspect -f '{{.State.Running}}' "$1" 2>/dev/null || true)" == "true" ]]
}

remove_container_if_exists() {
  local name="$1"
  if container_exists "${name}"; then
    log "Removing existing container ${name}"
    docker rm -f "${name}" >/dev/null
  fi
}

resolve_network() {
  if [[ -n "${EXTERNAL_NETWORK}" ]]; then
    if ! docker network inspect "${EXTERNAL_NETWORK}" >/dev/null 2>&1; then
      echo "EXTERNAL_NETWORK '${EXTERNAL_NETWORK}' does not exist." >&2
      echo "Check 'docker network ls' for the network your 1Panel DB/redis use." >&2
      exit 1
    fi
    NETWORK_NAME="${EXTERNAL_NETWORK}"
  else
    NETWORK_NAME="${OWN_NETWORK_NAME}"
    if ! docker network inspect "${NETWORK_NAME}" >/dev/null 2>&1; then
      log "Creating Docker network ${NETWORK_NAME}"
      docker network create "${NETWORK_NAME}" >/dev/null
    fi
  fi
}

tag_previous_image() {
  # Preserve the currently-active image as PREVIOUS_IMAGE so we can roll back
  # after a rebuild. No-op on the very first build.
  if docker image inspect "${IMAGE_NAME}" >/dev/null 2>&1; then
    log "Tagging current image as ${PREVIOUS_IMAGE} (for rollback)"
    docker tag "${IMAGE_NAME}" "${PREVIOUS_IMAGE}" >/dev/null
  fi
}

build_image() {
  require_cmd docker
  tag_previous_image

  local build_args=()
  if [[ "${NO_CACHE}" == "1" || "${NO_CACHE}" == "true" ]]; then
    build_args+=(--no-cache)
  fi
  if [[ -n "${PLATFORM}" ]]; then
    build_args+=(--platform "${PLATFORM}")
  fi
  build_args+=(--build-arg "FRONTEND_THEME=${FRONTEND_THEME}")
  build_args+=(--build-arg "FRONTEND_BUILD_GC_HEAP_SIZE=${FRONTEND_BUILD_GC_HEAP_SIZE}")

  log "Building Docker image ${IMAGE_NAME} (frontend theme: ${FRONTEND_THEME:-none}, heap: ${FRONTEND_BUILD_GC_HEAP_SIZE})"
  DOCKER_BUILDKIT="${DOCKER_BUILDKIT:-1}" docker build \
    "${build_args[@]}" \
    -f "${ROOT_DIR}/Dockerfile" \
    -t "${IMAGE_NAME}" \
    "${ROOT_DIR}"
}

wait_for_app() {
  if [[ "${HEALTHCHECK}" != "1" && "${HEALTHCHECK}" != "true" ]]; then
    return 0
  fi
  log "Waiting for app health (/api/status)"
  local i
  for i in $(seq 1 "${HEALTHCHECK_RETRIES}"); do
    if ! container_running "${CONTAINER_NAME}"; then
      warn "Container ${CONTAINER_NAME} exited during startup"
      docker logs --tail 50 "${CONTAINER_NAME}" >&2 || true
      return 1
    fi
    if docker exec "${CONTAINER_NAME}" wget -q -O - "http://localhost:${APP_PORT}/api/status" 2>/dev/null \
        | grep -qE '"success":[[:space:]]*true'; then
      log "App is healthy"
      return 0
    fi
    sleep 2
  done
  warn "App did not become healthy in ${HEALTHCHECK_RETRIES} attempts. Recent logs:"
  docker logs --tail 50 "${CONTAINER_NAME}" >&2 || true
  return 1
}

run_container() {
  require_cmd docker
  ensure_secrets
  resolve_network
  mkdir -p "${LOG_DIR}"

  remove_container_if_exists "${CONTAINER_NAME}"

  local env_args=(
    -e "TZ=${LOCAL_TZ}"
    -e "PORT=${APP_PORT}"
    -e "SQL_DSN=${SQL_DSN}"
    -e "REDIS_CONN_STRING=${REDIS_CONN_STRING}"
    -e "SESSION_SECRET=${SESSION_SECRET}"
    -e "CRYPTO_SECRET=${CRYPTO_SECRET}"
    -e "ERROR_LOG_ENABLED=${ERROR_LOG_ENABLED:-true}"
    -e "BATCH_UPDATE_ENABLED=${BATCH_UPDATE_ENABLED:-true}"
    -e "MEMORY_CACHE_ENABLED=${MEMORY_CACHE_ENABLED:-true}"
    -e "SYNC_FREQUENCY=${SYNC_FREQUENCY:-60}"
    -e "NODE_NAME=${NODE_NAME}"
  )
  if [[ -n "${LOG_SQL_DSN}" ]]; then
    env_args+=(-e "LOG_SQL_DSN=${LOG_SQL_DSN}")
  fi

  local pass_env_vars=(
    RELAY_TIMEOUT
    STREAMING_TIMEOUT
    CHANNEL_UPDATE_FREQUENCY
    GENERATE_DEFAULT_TOKEN
    FRONTEND_BASE_URL
    TRUSTED_REDIRECT_DOMAINS
  )
  local name
  for name in "${pass_env_vars[@]}"; do
    if [[ -n "${!name:-}" ]]; then
      env_args+=(-e "${name}=${!name}")
    fi
  done

  if [[ -n "${ENV_FILE}" ]]; then
    if [[ ! -f "${ENV_FILE}" ]]; then
      echo "ENV_FILE does not exist: ${ENV_FILE}" >&2
      exit 1
    fi
    env_args+=(--env-file "${ENV_FILE}")
  fi

  log "Starting app ${CONTAINER_NAME} on http://${HOST_BIND_IP}:${HOST_PORT} (network: ${NETWORK_NAME})"
  log "Using SQL_DSN: $(mask_secrets "${SQL_DSN}")"
  log "Using REDIS_CONN_STRING: $(mask_secrets "${REDIS_CONN_STRING}")"
  docker run -d \
    --name "${CONTAINER_NAME}" \
    --restart unless-stopped \
    --network "${NETWORK_NAME}" \
    --add-host "host.docker.internal:host-gateway" \
    -p "${HOST_BIND_IP}:${HOST_PORT}:${APP_PORT}" \
    -v "${APP_DATA_VOLUME}:/data" \
    -v "${LOG_DIR}:/app/logs" \
    "${env_args[@]}" \
    "${IMAGE_NAME}" \
    --log-dir /app/logs >/dev/null

  log "Secrets file: ${SECRETS_FILE}"
  log "App data volume: ${APP_DATA_VOLUME}"
  log "Logs dir: ${LOG_DIR}"
  log "Open locally: http://127.0.0.1:${HOST_PORT}"
  log "Reverse proxy upstream: http://127.0.0.1:${HOST_PORT}"
  if [[ -n "${FRONTEND_THEME}" ]]; then
    log "Frontend theme '${FRONTEND_THEME}' is baked into the image; select the active theme in the admin UI if needed."
  fi

  if ! wait_for_app; then
    warn "Startup health check failed."
    if docker image inspect "${PREVIOUS_IMAGE}" >/dev/null 2>&1; then
      warn "Roll back to the previous image with: bash bin/docker-1panel.sh rollback"
    fi
    return 1
  fi

  if [[ "${FOLLOW_LOGS}" == "1" || "${FOLLOW_LOGS}" == "true" ]]; then
    docker logs -f "${CONTAINER_NAME}"
  fi
}

git_pull() {
  if [[ "${GIT_PULL}" != "1" && "${GIT_PULL}" != "true" ]]; then
    log "Skipping git pull (GIT_PULL=${GIT_PULL})"
    return 0
  fi
  require_cmd git
  if [[ ! -d "${ROOT_DIR}/.git" ]]; then
    warn "${ROOT_DIR} is not a git repository; skipping git pull"
    return 0
  fi
  if [[ -n "$(git -C "${ROOT_DIR}" status --porcelain 2>/dev/null)" ]]; then
    warn "Working tree has local changes; pulling with rebase may fail. Commit/stash first if it does."
  fi
  local ref="${GIT_REF}"
  if [[ -z "${ref}" ]]; then
    ref="$(git -C "${ROOT_DIR}" rev-parse --abbrev-ref HEAD)"
  fi
  log "Updating source: git fetch ${GIT_REMOTE} && checkout ${ref}"
  git -C "${ROOT_DIR}" fetch --prune "${GIT_REMOTE}"
  git -C "${ROOT_DIR}" checkout "${ref}"
  git -C "${ROOT_DIR}" pull --ff-only "${GIT_REMOTE}" "${ref}"
  log "Now at: $(git -C "${ROOT_DIR}" rev-parse --short HEAD) $(git -C "${ROOT_DIR}" log -1 --pretty=%s)"
}

update_deploy() {
  require_cmd docker
  git_pull
  build_image
  run_container
}

rollback() {
  require_cmd docker
  if ! docker image inspect "${PREVIOUS_IMAGE}" >/dev/null 2>&1; then
    echo "No previous image (${PREVIOUS_IMAGE}) to roll back to." >&2
    exit 1
  fi
  log "Rolling back: retag ${PREVIOUS_IMAGE} -> ${IMAGE_NAME}"
  docker tag "${PREVIOUS_IMAGE}" "${IMAGE_NAME}" >/dev/null
  BUILD_ON_UP=0 run_container
}

stop_container() {
  require_cmd docker
  remove_container_if_exists "${CONTAINER_NAME}"
}

show_logs() {
  require_cmd docker
  docker logs -f "${CONTAINER_NAME}"
}

show_status() {
  require_cmd docker
  docker ps -a --filter "name=^/${CONTAINER_NAME}$"
}

clean_all() {
  stop_container
  log "Removing image ${IMAGE_NAME} if it exists"
  docker image rm "${IMAGE_NAME}" >/dev/null 2>&1 || true

  # The database and Redis are external (1Panel) and are never touched here.
  if [[ "${KEEP_VOLUMES:-1}" == "0" || "${KEEP_VOLUMES:-1}" == "false" ]]; then
    warn "Removing app data volume and generated secrets (external DB/Redis untouched)"
    docker volume rm "${APP_DATA_VOLUME}" >/dev/null 2>&1 || true
    rm -f "${SECRETS_FILE}"
  else
    log "Keeping app data volume. Set KEEP_VOLUMES=0 bash bin/docker-1panel.sh clean to remove it."
  fi
  if [[ -z "${EXTERNAL_NETWORK}" ]]; then
    docker network rm "${OWN_NETWORK_NAME}" >/dev/null 2>&1 || true
  fi
}

case "${ACTION}" in
  up)
    if [[ "${BUILD_ON_UP}" == "1" || "${BUILD_ON_UP}" == "true" ]]; then
      build_image
    fi
    run_container
    ;;
  build)
    build_image
    ;;
  run)
    run_container
    ;;
  update|upgrade)
    update_deploy
    ;;
  rollback)
    rollback
    ;;
  stop)
    stop_container
    ;;
  logs)
    show_logs
    ;;
  status)
    show_status
    ;;
  clean)
    clean_all
    ;;
  help|-h|--help)
    usage
    ;;
  *)
    echo "Unknown action: ${ACTION}" >&2
    usage
    exit 1
    ;;
esac
