.PHONY: fmt vet test build run check migrate migrate-status sqlc

fmt:
	go fmt ./...

vet:
	go vet ./...

test:
	go test ./...

build:
	go build -trimpath -o bin/api ./cmd/api
	go build -trimpath -o bin/migrate ./cmd/migrate

run:
	go run ./cmd/api

migrate:
	go run ./cmd/migrate up

migrate-status:
	go run ./cmd/migrate status

sqlc:
	docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.29.0 generate

check: fmt vet test build
