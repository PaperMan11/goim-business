APP := goim-business
BIN_DIR := bin
LOG_DIR := logs
VERSION ?= dev
# 启动时使用的配置文件；env 由文件内的 env 字段声明。
CONFIG ?= etc/dev.yaml

# Windows 下可执行文件需要 .exe 后缀。
EXE :=
ifdef OS
  ifneq ($(filter Windows%,$(OS)),)
    EXE := .exe
  endif
endif

API_BIN := $(BIN_DIR)/api$(EXE)
CRON_BIN := $(BIN_DIR)/cron$(EXE)
API_PID := $(BIN_DIR)/api.pid
CRON_PID := $(BIN_DIR)/cron.pid

.PHONY: tidy build build-api build-migrate build-cron test vet run run-cron start stop clean migrate-up migrate-down compose-up compose-down image

tidy:
	go mod tidy

build: build-api build-migrate build-cron

build-api:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(API_BIN) ./cmd/api

build-migrate:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o $(BIN_DIR)/migrate$(EXE) ./cmd/migrate

build-cron:
	CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$(VERSION)" -o $(CRON_BIN) ./cmd/cron

test:
	go test ./...

vet:
	go vet ./...

run:
	go run ./cmd/api -c $(CONFIG)

run-cron:
	go run ./cmd/cron -c $(CONFIG)

# 后台启动 api 与 cron，pid 写入 bin/，日志写入 logs/。
start: build
	@mkdir -p $(BIN_DIR) $(LOG_DIR)
	@if [ -f $(API_PID) ] && kill -0 "$$(cat $(API_PID))" 2>/dev/null; then \
		echo "api 已在运行 (pid $$(cat $(API_PID)))"; \
	else \
		nohup ./$(API_BIN) -c $(CONFIG) >> $(LOG_DIR)/api.log 2>&1 & \
		echo $$! > $(API_PID); \
		echo "api 已启动 (pid $$(cat $(API_PID)), 日志 $(LOG_DIR)/api.log)"; \
	fi
	@if [ -f $(CRON_PID) ] && kill -0 "$$(cat $(CRON_PID))" 2>/dev/null; then \
		echo "cron 已在运行 (pid $$(cat $(CRON_PID)))"; \
	else \
		nohup ./$(CRON_BIN) -c $(CONFIG) >> $(LOG_DIR)/cron.log 2>&1 & \
		echo $$! > $(CRON_PID); \
		echo "cron 已启动 (pid $$(cat $(CRON_PID)), 日志 $(LOG_DIR)/cron.log)"; \
	fi

# 按 pid 文件停止 api 与 cron，并清理 pid 文件。
stop:
	@for pidfile in $(API_PID) $(CRON_PID); do \
		name=$$(basename $$pidfile .pid); \
		if [ ! -f $$pidfile ]; then \
			echo "$$name 未在运行"; \
			continue; \
		fi; \
		pid=$$(cat $$pidfile); \
		if kill -0 $$pid 2>/dev/null; then \
			kill $$pid 2>/dev/null || true; \
			echo "已停止 $$name (pid $$pid)"; \
		else \
			echo "$$name 未在运行"; \
		fi; \
		rm -f $$pidfile; \
	done

# 先停服务，再清理构建产物、日志与覆盖率文件（Windows 下运行中的 exe 无法删除）。
clean: stop
	@rm -rf $(BIN_DIR) $(LOG_DIR) coverage.out coverage.html coverage.txt
	@echo "已清理 $(BIN_DIR)/ $(LOG_DIR)/ 与覆盖率文件"

migrate-up:
	go run ./cmd/migrate -c $(CONFIG) up

migrate-down:
	go run ./cmd/migrate -c $(CONFIG) down

compose-up:
	docker compose -f deploy/compose.dev.yaml up -d

compose-down:
	docker compose -f deploy/compose.dev.yaml down

image:
	docker build -t $(APP):dev .
