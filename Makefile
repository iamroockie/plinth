-include .env
export

GOLANGCI_VERSION := $(shell cat .golangci-lint-version)

BIN_DIR := bin
GOLANGCI := $(BIN_DIR)/golangci-lint

export PATH := $(PATH):$(CURDIR)/$(BIN_DIR)

.PHONY: test
test:
	@go test ./...

.PHONY: coverage
coverage:
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out | awk '/^total:/ {print $3}'
	@go tool cover -html=coverage.out
	@rm coverage.out

.PHONY: lint
lint: $(GOLANGCI)
	@$(GOLANGCI) run

.PHONY: format
format: $(GOLANGCI)
	@$(GOLANGCI) fmt

$(GOLANGCI): .golangci-lint-version
	@mkdir -p $(BIN_DIR)
	@curl -sSfL https://golangci-lint.run/install.sh | \
        sh -s -- -b $(BIN_DIR) v$(GOLANGCI_VERSION)
	@touch $@
