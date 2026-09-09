#!/usr/bin/env bash
# Generate Go code from api/proto. Requires protoc + protoc-gen-go / -go-grpc.
set -euo pipefail
cd "$(dirname "$0")/.."

command -v protoc >/dev/null || { echo "protoc not found"; exit 1; }

protoc \
  --proto_path=api/proto \
  --go_out=. --go_opt=module=github.com/example/myapp \
  --go-grpc_out=. --go-grpc_opt=module=github.com/example/myapp \
  $(find api/proto -name '*.proto')

echo "generated."
