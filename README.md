# goim-business

goim 业务中台服务：账号与登录态、IM 凭证换发、用户资料、设备管理、客户端配置，以及基于 RustFS / S3 兼容存储的文件上传。

- 语言与框架：Go 1.24、Gin、MySQL、Redis、AWS SDK v2（S3 协议）
- 对外接口：`/api/v1`，OpenAPI 描述见 [api/openapi.yaml](api/openapi.yaml)
- 对接 IM：goim `im-api`（开通账号、同步资料、换发 IM token），异步失败通过 outbox 补偿

## 功能

| 模块 | 内容 |
| --- | --- |
| 账号 | 注册、登录、刷新、登出、会话列表/撤销、修改密码、登录失败锁定与限流 |
| IM | 账号开通与资料同步、IM token 兑换（带缓存与续期）、手动重试、outbox 定时补偿 |
| 资料 | 昵称/头像读写、批量资料、公开资料查询；头像以 `objectKey` 为事实、`avatarUrl` 为派生值 |
| 设备 | 设备列表、按设备踢出 |
| 客户端配置 | 配置与 schema 下发，支持 `ETag` / 304 |
| 文件 | 预签名 PUT、分片上传（申请/补签名/完成/abort）、访问 URL、删除；`public/` 前缀匿名可读，其余走预签名 |
| 定时任务 | IM outbox 补偿、过期 pending 文件清理（先切状态再回收对象，避免误删刚完成的上传） |

## 目录结构

```
cmd/            api / cron / migrate 三个进程入口
internal/
  app/          进程装配（App、CronApp、任务注册、装配门禁）
  handler/      HTTP 路由与参数绑定
  service/      业务编排（account / file / clientconfig）
  domain/       实体、值对象与仓储端口（不依赖框架）
  repository/   MySQL 仓储实现
  client/goim/  goim im-api 客户端
  job/          im outbox 补偿、file 过期清理
  middlewares/  鉴权、日志、恢复、限流、指标
  config/       配置加载与启动期强校验
  pkg/          日志、token、存储、锁、限流等基础设施
migrations/     golang-migrate SQL（000001~000005）
etc/            base.yaml 基线 + dev/prod 环境配置
api/            OpenAPI（YAML）与 Apifox 可导入 JSON
scripts/        联调辅助脚本
deploy/         本地 compose
docs/           开发、前端对接与联调验收文档
```

## 快速开始

### 1. 依赖

- Go 1.24
- MySQL 5.7+ 或 8.x
- Redis 6+
- 可选：RustFS / 任意 S3 兼容存储（文件功能）、goim `im-api`（IM 功能）

### 2. 配置

配置文件通过启动参数指定，默认 `etc/dev.yaml`：

```bash
cp etc/dev.yaml etc/dev.local.yaml   # 按需修改并避免提交本地密钥
```

加载顺序为「同目录 `base.yaml` 基线 → 指定文件覆盖 → `GOIM_BIZ_*` 环境变量覆盖」，`env` 取自文件内的 `env` 字段（`dev` / `staging` / `prod`）。未接入 IM 或对象存储时，对应能力自动关闭，不影响其他模块启动。

密钥建议用环境变量注入，例如 `GOIM_BIZ_MYSQL__DSN`、`GOIM_BIZ_REDIS__PASSWORD`、`GOIM_BIZ_AUTH__JWT_SECRET`、`GOIM_BIZ_STORAGE__SECRET_KEY`、`GOIM_BIZ_IM__ADMIN_SECRET`，参考 [.env.example](.env.example)。

### 3. 初始化与启动

```bash
make tidy
make migrate-up          # 建表并迁移到最新版本
make start               # 构建后后台启动 api 与 cron
curl http://127.0.0.1:18181/readyz
```

默认端口：业务 `18180`、探针 `18181`（`/healthz`、`/readyz`）、指标 `19191`（`/metrics`）。

```bash
make stop                # 按 bin/*.pid 停止 api 与 cron
make clean               # 先停止，再删除 bin/ 与 logs/
make run                 # 前台运行 api（开发调试）
make run-cron            # 前台运行 cron
make start CONFIG=etc/prod.yaml
```

二进制均支持 `-c` / `-config` 指定配置文件：

```bash
./bin/api -c etc/dev.yaml
go run ./cmd/migrate -c etc/dev.yaml up
```

### 4. 注册联调账号

```bash
bash scripts/register-test-user.sh -l alice -n "Alice"
```

脚本会注册账号并校验 IM 开通状态与 IM token 兑换；`--skip-im-check` 可跳过 IM 校验。

## 接口文档

- [api/openapi.yaml](api/openapi.yaml)：OpenAPI 3.0.3，覆盖全部业务接口
- [api/apifox.json](api/apifox.json)：由上述规范转换，可直接在 Apifox「导入数据 → OpenAPI」使用
- 鉴权：除注册/登录/刷新与公开资料外均需 `Authorization: Bearer <accessToken>`；注册、登录与登录态接口需带 `X-Device-Id`，客户端标识用 `X-Client-Platform`、`X-Client-Version`

## 开发与测试

```bash
make test                # go test ./...
make vet                 # go vet ./...
go test -race ./...      # CI 使用
```

- 全量开发说明、数据模型、错误码与路线图：[docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)
- 前端对接（登录态、IM、资料、文件）：[docs/FRONTEND_INTEGRATION.md](docs/FRONTEND_INTEGRATION.md)
- P0 联调验收清单：[docs/P0-1_INTEGRATION_ACCEPTANCE.md](docs/P0-1_INTEGRATION_ACCEPTANCE.md)

## 部署

```bash
make image               # docker build -t goim-business:dev .
```

镜像基于 distroless 非 root 运行，入口 `/app/api`；cron 复用同一镜像，通过 `--entrypoint /app/cron` 覆盖启动命令：

```bash
docker run --rm goim-business:dev -c /app/etc/prod.yaml
docker run --rm --entrypoint /app/cron goim-business:dev -c /app/etc/prod.yaml
```

生产环境务必替换全部开发密钥，并通过环境变量注入敏感配置；`etc/prod.yaml` 中 DSN、Redis 地址与 JWT secret 均为空，启动校验会拒绝缺失项。

## 当前状态

已完成：账号/会话、IM 开通与凭证换发、资料与头像、设备、客户端配置、文件上传（含分片与过期/删除态对象回收）、账号注销（冷却期 + IM placeholder 同步 + 异步清除）、Makefile 运维目标、OpenAPI/Apifox 文档。

待补充：goim 真删账号接口接入（当前 placeholder 占位资料）、解冻与数据导出、仓储层集成测试、压测与告警规则、画像/推送/管理端等 B 档能力。详见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) 的路线图。
