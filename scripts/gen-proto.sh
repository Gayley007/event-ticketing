#!/bin/sh
# Regenerates the Go code in gen/ from the .proto files.
# Runs entirely inside a golang container, so protoc does not need to be
# installed locally. Usage (from the repository root):
#
#   docker run --rm -v "$PWD":/src -w /src golang:1.23-alpine sh scripts/gen-proto.sh
set -e

apk add --no-cache protobuf protobuf-dev >/dev/null
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.35.1
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1

MODULE=github.com/Gayley007/event-ticketing

protoc -I proto -I /usr/include \
  --go_out=. --go_opt=module=$MODULE \
  --go-grpc_out=. --go-grpc_opt=module=$MODULE \
  proto/event/event.proto proto/ticket/ticket.proto

echo "generated:" && find gen -name '*.go'
