.DEFAULT_GOAL := help

.PHONY: help lint lint-go lint-yaml format test coverage build clean

## Linting
lint: lint-go lint-yaml ## Run all linters

lint-go: ## Run Go linter (golangci-lint)
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not found, skipping"; \
	fi

lint-yaml: ## Run YAML linter
	@if command -v yamllint >/dev/null 2>&1; then \
		yamllint .; \
	else \
		echo "yamllint not found, skipping"; \
	fi

## Formatting
format: ## Format Go code
	gofmt -s -w .
	@if command -v goimports >/dev/null 2>&1; then \
		goimports -w .; \
	else \
		echo "goimports not found, skipping"; \
	fi

## Testing
test: ## Run Go tests
	go test -v ./...

coverage: ## Generate test coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

## Building
build: ## Build Go module
	go build -v ./...

## Cleanup
clean: ## Remove build artifacts
	go clean
	rm -f coverage.out coverage.html
	rm -rf build/ dist/ megalinter-reports/

## Help
help: ## Show available targets
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
