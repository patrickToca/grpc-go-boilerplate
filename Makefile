generate_buf: ## Generate protobuf
	@echo "Generating protobuf"
	@go run github.com/bufbuild/buf/cmd/buf generate

generate: generate_buf

clean_protobuf: ## Delete generated protobuf files
	@echo "Deleting protobuf generated code"
	@rm -rf gen/proto

clean: clean_protobuf

lint: ## Lint protobuf and files
	@echo "Linting protobuf"
	@go run github.com/bufbuild/buf/cmd/buf lint
	@echo "Linting code"
	@golangci-lint run --timeout 5m0s ./...
