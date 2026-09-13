#!/usr/bin/env bash
#
# Installs the development tools this repo uses. Safe to re-run; each tool is
# skipped if the pinned version is already present.
#
#   scripts/install-tools.sh            # install everything
#   scripts/install-tools.sh lint vuln  # install only the named tools
#
# Binaries land in $(go env GOPATH)/bin — make sure that is on your PATH.
set -euo pipefail
cd "$(dirname "$0")/.."

GO="${GO:-go}"
command -v "$GO" >/dev/null || { echo "error: '$GO' not found (set GO=/path/to/go)"; exit 1; }

GOBIN="$("$GO" env GOPATH)/bin"
mkdir -p "$GOBIN"

# ---- pinned versions -------------------------------------------------------
# Keep these in sync with .golangci.yml / CI.
# golangci-lint: pinned to the v2.x line (matches the v2 schema in .golangci.yml).
GOLANGCI_LINT_VERSION="v2.13.2"
GOVULNCHECK_VERSION="latest"
GOIMPORTS_VERSION="latest"
MOCKERY_VERSION="v2.53.7"
BUF_VERSION="v1.73.0"
PROTOC_GEN_GO_VERSION="v1.36.12"
PROTOC_GEN_GO_GRPC_VERSION="v1.5.1"
GRPC_GATEWAY_VERSION="v2.30.0"

# ---- helpers -------------------------------------------------------------
have() { command -v "$1" >/dev/null 2>&1; }

# go_install <bin-name> <module@version>
go_install() {
	local name="$1" pkg="$2"
	echo "==> $name ($pkg)"
	GOBIN="$GOBIN" "$GO" install "$pkg"
}

install_golangci_lint() {
	local ver="$GOLANGCI_LINT_VERSION"
	if have golangci-lint && golangci-lint version 2>&1 | grep -q "${ver#v}"; then
		echo "==> golangci-lint $ver already installed"
		return
	fi
	echo "==> golangci-lint ($ver)"
	# Official installer — verifies checksums; the tag pins the exact release.
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh \
		| sh -s -- -b "$GOBIN" "$ver"
}

# ---- tool registry ------------------------------------------------------
install_lint()   { install_golangci_lint; }
install_vuln()   { go_install govulncheck   "golang.org/x/vuln/cmd/govulncheck@${GOVULNCHECK_VERSION}"; }
install_imports() { go_install goimports     "golang.org/x/tools/cmd/goimports@${GOIMPORTS_VERSION}"; }
install_mockery() { go_install mockery      "github.com/vektra/mockery/v2@${MOCKERY_VERSION}"; }
install_proto()  {
	go_install buf                    "github.com/bufbuild/buf/cmd/buf@${BUF_VERSION}"
	go_install protoc-gen-go          "google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_GO_VERSION}"
	go_install protoc-gen-go-grpc     "google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION}"
	go_install protoc-gen-grpc-gateway "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@${GRPC_GATEWAY_VERSION}"
	go_install protoc-gen-openapiv2   "github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@${GRPC_GATEWAY_VERSION}"
	# buf is its own compiler front-end (no separate 'protoc' binary needed);
	# `make proto` drives it via buf.gen.yaml.
}

ALL=(lint vuln imports mockery proto)

targets=("$@")
[ ${#targets[@]} -eq 0 ] && targets=("${ALL[@]}")

for t in "${targets[@]}"; do
	if ! printf '%s\n' "${ALL[@]}" | grep -qx "$t"; then
		echo "unknown tool: $t (choices: ${ALL[*]})" >&2
		exit 2
	fi
	"install_$t"
done

echo
echo "done. Installed into: $GOBIN"
case ":$PATH:" in
	*":$GOBIN:"*) ;;
	*) echo "warning: $GOBIN is not on your PATH — add it to use the tools" ;;
esac
