# Sandbox image for the agent-framework project.
#
# Contains everything needed to build and run the project (Go 1.24, git) plus
# the Claude Code CLI, so a Claude session can be run entirely inside the
# container with the host filesystem untouched apart from the mounted repo.

FROM golang:1.24-bookworm

ARG NODE_MAJOR=22
ARG CLAUDE_CODE_VERSION=latest

ARG UID=1000
ARG GID=1000
ARG USERNAME=dev

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates \
        curl \
        gnupg \
        git \
        openssh-client \
        less \
        jq \
        ripgrep \
        procps \
        sudo \
        unzip \
        vim \
    && rm -rf /var/lib/apt/lists/*

# Node is not needed by the Go build, but Claude Code ships as an npm package
# and most MCP servers are launched through npx.
RUN curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && rm -rf /var/lib/apt/lists/*

RUN npm install -g "@anthropic-ai/claude-code@${CLAUDE_CODE_VERSION}" \
    && npm cache clean --force

# Non-root user: Claude Code refuses --dangerously-skip-permissions as root,
# which is the mode that makes a throwaway sandbox useful.
RUN if getent group "${GID}" >/dev/null; then groupmod -n "${USERNAME}" "$(getent group "${GID}" | cut -d: -f1)"; else groupadd -g "${GID}" "${USERNAME}"; fi \
    && if getent passwd "${UID}" >/dev/null; then usermod -l "${USERNAME}" -d "/home/${USERNAME}" -m "$(getent passwd "${UID}" | cut -d: -f1)"; else useradd -u "${UID}" -g "${GID}" -m -s /bin/bash "${USERNAME}"; fi \
    && echo "${USERNAME} ALL=(ALL) NOPASSWD:ALL" > "/etc/sudoers.d/${USERNAME}" \
    && chmod 0440 "/etc/sudoers.d/${USERNAME}"

# Writable caches for the non-root user.
ENV GOPATH=/home/${USERNAME}/go \
    GOCACHE=/home/${USERNAME}/.cache/go-build \
    GOMODCACHE=/home/${USERNAME}/go/pkg/mod \
    PATH=/home/${USERNAME}/go/bin:/usr/local/go/bin:$PATH
RUN mkdir -p "/home/${USERNAME}/go/bin" "/home/${USERNAME}/.cache/go-build" "/home/${USERNAME}/.claude" /workspace \
    && chown -R "${UID}:${GID}" "/home/${USERNAME}" /workspace

# Login shells re-set PATH from /etc/profile, which would drop the Go toolchain.
RUN printf 'export PATH=/home/%s/go/bin:/usr/local/go/bin:$PATH\n' "${USERNAME}" \
        > /etc/profile.d/agent-framework.sh

# Server lifecycle scripts. Copied before the repo so editing Go code does not
# invalidate this layer.
COPY docker-entrypoint.sh af-server /usr/local/bin/
RUN chmod 0755 /usr/local/bin/docker-entrypoint.sh /usr/local/bin/af-server

WORKDIR /workspace
USER ${USERNAME}

# Warm the module cache at build time so the first `go build` in the container
# is fast and works offline. Only go.mod/go.sum invalidate this layer.
COPY --chown=${UID}:${GID} go.mod go.sum ./
RUN go mod download

# The repo itself is expected to be bind-mounted over /workspace at run time;
# this copy just makes the image usable standalone.
COPY --chown=${UID}:${GID} . .

# Secrets are mounted here read-only (see run-sandbox.sh). main.go's -env-file
# flag reads the file directly; --env-file additionally injects the same vars.
ENV AF_ENV_FILE=/run/secrets/af.env

# Server autostart. AF_LISTEN_PORT is the port inside the container and must
# match the container side of the port mapping; AF_HOST_PORT is cosmetic, used
# only to print a URL you can click. Set AF_AUTOSTART=0 to skip the autostart.
ENV AF_AUTOSTART=1 \
    AF_LISTEN_PORT=8080 \
    AF_HOST_PORT=8000 \
    AF_SERVER_LOG=/tmp/af-server.log

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/docker-entrypoint.sh"]
CMD ["/bin/bash"]