.PHONY: run build test test-int lint sqlc migrate-new golden

# Dev tools run via `go run` at pinned versions to keep them out of go.mod.
SQLC_VERSION  := v1.31.1
GOOSE_VERSION := v3.30.2

run:
	set -a && . ./.env && set +a && go run ./cmd/bot

build:
	CGO_ENABLED=0 go build -o bin/bot ./cmd/bot

test:
	go test ./...

test-int:
	go test -tags=integration ./...

lint:
	golangci-lint run

sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=<desc>" && exit 1)
	go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION) -dir migrations -s create $(name) sql

golden:
	go test ./internal/report/... ./internal/pdf/... -update
