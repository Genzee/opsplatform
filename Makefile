GO ?= go

.PHONY: check fmt-check test vet build demo

check: fmt-check test vet

fmt-check:
	@test -z "$$(gofmt -l cmd internal pkg)" || { gofmt -l cmd internal pkg; exit 1; }

test:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

build:
	$(GO) build -o bin/infra ./cmd/infra
	$(GO) build -o bin/core ./cmd/core

demo:
	$(GO) run ./cmd/infra -fixture examples/shared-storage.json impact storage-01
