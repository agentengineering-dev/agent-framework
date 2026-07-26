#!/usr/bin/env bash
# Build (if needed) and enter the agent-framework sandbox container.
#
# The agent-framework server is started automatically inside the container and
# published on http://localhost:$PORT — see docker-entrypoint.sh / af-server.
#
#   ./run-sandbox.sh                 # interactive shell in the sandbox
#   ./run-sandbox.sh claude          # start a Claude Code session directly
#   ./run-sandbox.sh go run main.go -env-file "$AF_ENV_FILE"
#
# Env overrides: AF_ENV_FILE, IMAGE, PORT, AF_AUTOSTART=0, AF_REBUILD=1
set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${AF_ENV_FILE:-$HOME/.afconfig/.env}"
IMAGE="${IMAGE:-agent-framework-sandbox}"
PORT="${PORT:-8000}"

if [[ ! -f "$ENV_FILE" ]]; then
  echo "error: secrets file not found: $ENV_FILE" >&2
  echo "       set AF_ENV_FILE=/path/to/.env to point elsewhere" >&2
  exit 1
fi

if [[ -n "${AF_REBUILD:-}" ]] || ! docker image inspect "$IMAGE" >/dev/null 2>&1; then
  docker build \
    --build-arg UID="$(id -u)" \
    --build-arg GID="$(id -g)" \
    -t "$IMAGE" "$REPO_DIR"
fi

# Named volumes keep the Go caches and the Claude Code login/config across runs.
docker volume create agent-framework-gocache >/dev/null
docker volume create agent-framework-claude  >/dev/null

# Published on 127.0.0.1 only: the container holds live API keys, so the server
# should not be reachable from other machines on the network.
exec docker run --rm -it \
  --name "agent-framework-sandbox-$$" \
  --env-file "$ENV_FILE" \
  -v "$ENV_FILE:/run/secrets/af.env:ro" \
  -v "$REPO_DIR:/workspace" \
  -v agent-framework-gocache:/home/dev/.cache/go-build \
  -v agent-framework-claude:/home/dev/.claude \
  -e "AF_AUTOSTART=${AF_AUTOSTART:-1}" \
  -e "AF_HOST_PORT=${PORT}" \
  -p "127.0.0.1:${PORT}:8080" \
  "$IMAGE" "${@:-bash}"