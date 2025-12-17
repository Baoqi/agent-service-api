#!/bin/bash
# Generate Go code from protobuf definitions
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "Generating protobuf code..."

# Generate Go code
protoc \
    --proto_path="$PROJECT_ROOT/api/v1" \
    --go_out="$PROJECT_ROOT/api/v1" \
    --go_opt=paths=source_relative \
    --go-grpc_out="$PROJECT_ROOT/api/v1" \
    --go-grpc_opt=paths=source_relative \
    "$PROJECT_ROOT/api/v1/agent_service.proto"

echo "Done! Generated files:"
ls -la "$PROJECT_ROOT/api/v1/"*.go 2>/dev/null || echo "No .go files generated yet"



