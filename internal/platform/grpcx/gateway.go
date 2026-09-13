package grpcx

import (
	"context"
	"net/http"
	"strconv"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/protobuf/proto"
)

// Outgoing gRPC metadata keys a unary handler can set (via grpc.SetHeader) to
// steer the REST transcoding grpc-gateway does — gRPC has no notion of "201
// Created" or a Location header, but REST callers of a resource-creating RPC
// expect both. See StatusCodeOption and OutgoingHeaderMatcher below.
const (
	HTTPStatusMetadataKey = "x-http-code"
	LocationMetadataKey   = "location"
)

// StatusCodeOption is a runtime.WithForwardResponseOption that honors
// HTTPStatusMetadataKey, overriding grpc-gateway's default 200 OK.
func StatusCodeOption(ctx context.Context, w http.ResponseWriter, _ proto.Message) error {
	md, ok := runtime.ServerMetadataFromContext(ctx)
	if !ok {
		return nil
	}
	vs := md.HeaderMD.Get(HTTPStatusMetadataKey)
	if len(vs) == 0 {
		return nil
	}
	code, err := strconv.Atoi(vs[0])
	if err != nil {
		return nil
	}
	w.WriteHeader(code)
	return nil
}

// OutgoingHeaderMatcher forwards LocationMetadataKey as a literal Location
// response header. HTTPStatusMetadataKey is deliberately not forwarded here —
// StatusCodeOption consumes it instead of exposing it as a header.
func OutgoingHeaderMatcher(key string) (string, bool) {
	if key == LocationMetadataKey {
		return "Location", true
	}
	return "", false
}
