.PHONY: run build test test-int lint sqlc migrate-new golden

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
	go tool sqlc generate

migrate-new:
	@test -n "$(name)" || (echo "usage: make migrate-new name=<desc>" && exit 1)
	go tool goose -dir migrations create $(name) sql

golden:
	go test ./internal/report/... ./internal/pdf/... -update
