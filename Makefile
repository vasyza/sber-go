GOLANGCI_LINT_VERSION := v2.14.0
GOLANGCI_LINT_DIR := $(CURDIR)/bin/golangci-lint-$(GOLANGCI_LINT_VERSION)
GOLANGCI_LINT := $(GOLANGCI_LINT_DIR)/golangci-lint

.PHONY: build test race vet fmt fmt-check lint lint-install check

build:
	go build ./...
	go build -o bin/sber ./cmd/sber

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

fmt: $(GOLANGCI_LINT)
	"$(GOLANGCI_LINT)" fmt

fmt-check:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; exit 1; }

$(GOLANGCI_LINT):
	mkdir -p "$(GOLANGCI_LINT_DIR)"
	curl --fail --silent --show-error --location \
		https://raw.githubusercontent.com/golangci/golangci-lint/$(GOLANGCI_LINT_VERSION)/install.sh \
		--output "$(GOLANGCI_LINT_DIR)/install.sh"
	sh "$(GOLANGCI_LINT_DIR)/install.sh" -b "$(GOLANGCI_LINT_DIR)" $(GOLANGCI_LINT_VERSION)

lint-install: $(GOLANGCI_LINT)

lint: $(GOLANGCI_LINT)
	"$(GOLANGCI_LINT)" config verify
	"$(GOLANGCI_LINT)" run

check: fmt-check lint vet race build
	go mod verify
