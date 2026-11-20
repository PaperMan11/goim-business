# AGENTS.md

本文件是 `goim-business` 的代理协作入口。做非平凡改动前，先读
[.trae/skills/development-guide/SKILL.md](.trae/skills/development-guide/SKILL.md)；
完整架构与数据模型以 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) 为准，接口以
[api/openapi.yaml](api/openapi.yaml) 为准。

## 项目事实

- Go 1.24 模块化单体：Gin + GORM/MySQL + Redis + zap + Prometheus；对象存储走 S3 协议（RustFS）。
- 三个进程入口：`cmd/api`（HTTP）、`cmd/cron`（定时任务）、`cmd/migrate`（迁移），同镜像不同 command。
- 业务接口前缀是 `/biz-api/v1`；goim im-api 占用 `/api/v1`，两者不得混用。
- 统一响应 `{code,msg,data}`，业务响应 HTTP 恒为 200；只有 CORS 预检 204 与 client-config 304 例外。
- 账号 userID 就是 IM userID，禁止任何映射或二次生成。
- 配置按「`etc/base.yaml` → `-c` 指定文件 → `GOIM_BIZ_*` 环境变量」合并；密钥只允许环境变量注入。
- 未配置 IM / 对象存储时对应能力自动关闭，服务仍可启动。

## 常用命令

| 目的 | 命令 |
|---|---|
| 编译 | `go build ./...` |
| 静态检查 | `go vet ./...` |
| 单测 | `go test ./... -count=1` |
| CI 等价 | `go test -race ./...` |
| 格式化 | `gofmt -w <files>` |
| 迁移 | `make migrate-up` / `go run ./cmd/migrate -c etc/dev.yaml up` |
| 前台运行 | `make run` / `make run-cron` |
| 后台启停 | `make start` / `make stop` |
| OpenAPI 守护 | `go test ./api/... -count=1` |

## 不可破坏的规则

1. **分层**：`handler` 不碰 gorm，只做绑定/响应；`service` 不出现 `gin.Context`，负责编排与事务；
   `domain` 不依赖框架；`repository` 只做持久化并把底层错误映射为领域错误；外部系统适配放 `client`。
2. **契约同步**：改接口必须同时更新 `api/openapi.yaml`、`api/apifox.json` 与 `api/openapi_test.go`；
   未实现的 B 档能力不注册路由。
3. **迁移**：`migrations/` 保持扁平，六位版本号严格递增，`up`/`down` 成对；一个迁移只做一件事；
   不建外键；时间列 `DATETIME(3)`；执行过的迁移只增不改；禁止 `AutoMigrate`。
4. **事务与异步**：跨表写用 `ports.TxManager.WithTx`；需要外部系统最终一致的写，在同一事务内写
   `outbox_events`，不要直接调用远程后就算完成。新增 action 必须补 worker 分支、退避与单测。
5. **IM 投影**：开通、资料同步、注销占位共用按用户的 Redis 锁；锁内重新读用户状态，注销后禁止再写
   IM 资料。outbox 落定按任务 ID 做条件状态迁移，不能把已完成任务重新打开。
6. **配置**：新增配置项要同时改结构体、`etc/base.yaml`、环境变量覆盖路径、启动期校验与文档；
   secret 在仓库中留空。
7. **测试**：领域/服务用手写 fake，Redis 用 `miniredis`；时间与随机数可注入；不依赖本机中间件。
   新增行为至少覆盖成功、边界与幂等/并发场景。
8. **隐私**：日志禁止出现密码、access/refresh token、IM token、admin secret；指标标签禁止高基数。
9. **交付**：先保证 `gofmt`、`go build ./...`、`go vet ./...`、`go test ./... -count=1` 通过，
   再更新文档。除非用户明确要求，不要提交 git；提交使用 `feat:` / `fix:` / `docs:` / `refactor:`
   前缀与中文说明。
