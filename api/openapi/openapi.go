// Package openapi embeds this repo's OpenAPI specs so they can be served (e.g.
// at /openapi.json) without depending on the filesystem layout at runtime.
//
// customerv1.swagger.json is generated from api/proto/customer/v1/customer.proto
// by `make proto` (buf + protoc-gen-openapiv2) — the proto file is the single
// source of truth for both the gRPC contract and its REST/OpenAPI transcoding
// (see internal/customer/interfaces/grpc, which mounts the grpc-gateway).
package openapi

import _ "embed"

//go:embed customerv1.swagger.json
var CustomerV1 []byte
