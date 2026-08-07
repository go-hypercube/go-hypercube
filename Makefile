# Test the codebase
test:
	@echo "Testing..."
	@go test ./... -v

# Check go code using go vet & staticcheck
chk:
	@echo "Runing go vet & staticcheck..."
	@go vet ./...
	@staticcheck ./...

# Format the codebase using gofumpt
fmt:
	@echo "Formating..."
	@gofumpt -w .
