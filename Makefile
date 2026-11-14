APP := goim-business

.PHONY: tidy build test vet run migrate-up migrate-down compose-up compose-down

tidy:
	go mod tidy

build:
	go build ./...

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/api

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

compose-up:
	docker compose -f deploy/compose.dev.yaml up -d

compose-down:
	docker compose -f deploy/compose.dev.yaml down

image:
	docker build -t $(APP):dev .
