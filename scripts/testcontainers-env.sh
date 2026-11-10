#!/usr/bin/env bash
# Prints env assignments that let testcontainers-go find Docker, for use as a
# command prefix:  $(scripts/testcontainers-env.sh) go test ...
#
# testcontainers-go does not read Docker CLI contexts, so with colima (or any
# non-default context) it can't find the daemon. This derives DOCKER_HOST from
# the active context, unless DOCKER_HOST is already set. For colima it also
# points Ryuk (the reaper container) at the socket path inside the VM.
# Portable bash 3.2. Prints nothing when no change is needed.
set -u

if [ -n "${DOCKER_HOST:-}" ] || ! command -v docker >/dev/null 2>&1; then
	exit 0
fi

host="$(docker context inspect --format '{{.Endpoints.docker.Host}}' 2>/dev/null || true)"
if [ -z "$host" ] || [ "$host" = "unix:///var/run/docker.sock" ]; then
	exit 0
fi

out="DOCKER_HOST=$host"
case "$host" in
*colima*) out="$out TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=/var/run/docker.sock" ;;
esac
echo "$out"
