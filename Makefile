.PHONY: proto deps clean

# Generate protobuf code
proto:
	@echo "Generating protobuf code..."
	@chmod +x scripts/gen-proto.sh
	@./scripts/gen-proto.sh

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod tidy
	go mod download

# Clean generated files
clean:
	@echo "Cleaning generated files..."
	rm -f api/v1/*.pb.go

# Install protobuf tools
tools:
	@echo "Installing protobuf tools..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest



