#!/usr/bin/env bash
# Container entrypoint: bring the agent-framework server up in the background,
# then hand control to whatever command was asked for (bash, claude, ...).
#
# A failed server start is deliberately non-fatal — a broken build should still
# drop you into a shell where you can fix it and run `af-server start`.
set -uo pipefail

AF_AUTOSTART="${AF_AUTOSTART:-1}"
AF_LISTEN_PORT="${AF_LISTEN_PORT:-8080}"

if [[ "$AF_AUTOSTART" == "1" ]]; then
  if af-server start; then
    echo "sandbox: control plane UI -> http://localhost:${AF_HOST_PORT:-8000}/ui/chat.html"
  else
    echo "sandbox: continuing without the server; run 'af-server start' once fixed." >&2
  fi
  echo "sandbox: manage it with 'af-server start|stop|restart|status|logs'"
  echo
fi

exec "$@"
