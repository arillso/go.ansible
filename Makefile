# Set PROJECT_DIR to the CI-provided project directory if available; otherwise fallback to the current directory.
ifndef CI_PROJECT_DIR
	ifndef GITHUB_WORKSPACE
		PROJECT_DIR := $(shell pwd)
	else
		PROJECT_DIR := $(GITHUB_WORKSPACE)
	endif
else
	PROJECT_DIR := $(CI_PROJECT_DIR)
endif

.DEFAULT_GOAL := help

.PHONY: format-code format-all run-megalinter ansible-build tests help

format-code: ## Format code files using Prettier via Docker.
	@docker run --rm --name prettier -v $(PROJECT_DIR):$(PROJECT_DIR) -w /$(PROJECT_DIR) node:alpine npx prettier . --write

format-all: format-code ## Run both format-code and format-eclint.
	@echo "Formatting completed."

run-megalinter: ## Run Megalinter locally.
	@docker run --rm --name megalint -v $(PROJECT_DIR):/tmp/lint busybox rm -rf /tmp/lint/megalinter-reports
	@docker run --rm --name megalint -v $(PROJECT_DIR):/tmp/lint oxsecurity/megalinter:v8.4.2

tests: ## Run Go tests inside a Go container.
	@echo "Running Go tests..."
	@docker run --rm -v $(PROJECT_DIR):/go/src/app -w /go/src/app golang:alpine go test -v ./...

help: ## Show an overview of available targets.
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?##' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'
