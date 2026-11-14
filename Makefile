APP := goim-business
BIN_DIR := bin
VERSION ?= dev

# Windows 下可执行文件需要 .exe 后缀。
EXE :=
ifdef OS
  ifneq ($(filter Windows%,$(OS)),)
    EXE := .exe
  endif
endif

.PHONY: tidy build build-api build-migrate test vet run migrate-up migrate-down compose-up compose-down image

tidy:
	go mod tidy

build: build-api build-migrate

build-api:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(BIN_DIR)/api$(EXE) ./cmd/api

build-migrate:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/migrate$(EXE) ./cmd/migrate

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
