GO ?= go
GOLANGCI_LINT_VERSION ?= v2.14.0

.PHONY: build test coverage vet lint fmt-check ci

build:
	$(GO) build ./...

test:
	$(GO) test ./...

coverage:
	$(GO) test ./... -coverprofile=coverage.out
	$(GO) tool cover -func=coverage.out | tail -1

vet:
	$(GO) vet ./...

lint:
	command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || $(GO) run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run

fmt-check:
	@test -z "$$($(GO) fmt ./...)" || (echo "go fmt made changes" && exit 1)

ci: fmt-check vet test lint
