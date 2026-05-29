SHELL := /bin/bash
GO ?= go
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)
PKG := ./...

.PHONY: build test lint vuln quality secrets-scan precommit-install run fmt clean

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/feedflow ./cmd/feedflow

test:
	$(GO) test --count=1 --shuffle=on -race -cover $(PKG)

lint:
	$(GO) vet $(PKG)
	staticcheck $(PKG)
	golangci-lint run --timeout 5m $(PKG)

vuln:
	govulncheck $(PKG)

quality:
	./scripts/quality-gate.sh

secrets-scan:
	gitleaks detect --no-git --source . --redact --no-banner --config .gitleaks.toml

precommit-install:
	pre-commit install

fmt:
	$(GO) fmt $(PKG)

run: build
	./bin/feedflow

clean:
	rm -rf bin coverage.out
