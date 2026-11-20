---
name: development-guide
description: Guide goim-business changes through handler, service, domain, repository, migration, config, and cron. Use when adding APIs, tables, outbox actions, or file storage. Not for unrelated repos.
---

# goim-business 开发指南

按本指南修改代码。它与根目录 [AGENTS.md](../../../AGENTS.md) 配套，完整背景见
[docs/DEVELOPMENT.md](../../../docs/DEVELOPMENT.md)，接口契约以
[api/openapi.yaml](../../../api/openapi.yaml) 为准。

## 1. 开始前

1. 读相关代码与文档，不要只按需求字面改。
2. 判断改动属于哪个域：`account` / `file` / `clientconfig` / `im` / `common`。
3. 明确是同步路径、事务内写入、outbox 补偿，还是 cron 任务。
4. 需要新接口时先确认契约，再实现 handler、service、repository。
5. 需要新表/字段时先定迁移，再改 model/domain/repository。

## 2. 分层与依赖方向

| 层 | 位置 | 允许 | 禁止 |
|---|---|---|---|
| handler | `internal/handler/<domain>` | 绑定 DTO、取上下文、调 service、写响应 | 直接使用 `*gorm.DB`、写业务事务 |
| service | `internal/service/<domain>` | 业务编排、事务边界、外部端口调用 | 使用 `*gin.Context`、直接拼 SQL |
| domain | `internal/domain/<domain>` | 实体、值对象、领域错误、仓储端口 | 依赖 gin/gorm/redis/http |
| ports | `internal/service/ports` | 事务、缓存、外部客户端、outbox 端口 | 具体实现 |
| repository | `internal/repository/<domain>` | GORM 持久化、条件更新、错误映射 | 业务决策、远程调用 |
| client | `internal/client/<system>` | 外部系统协议与错误映射 | 把外部 DTO 泄漏到 handler |

- 依赖方向只能是 handler → service → domain/ports；repository 与 client 实现端口。
- 不跨域直接 import 对方的 service；跨域能力通过端口注入。
- 不把 gorm model 直接序列化给前端；响应使用 `internal/types` 中的 DTO。
- 手工构造函数注入，组合根在 `internal/app/app.go` 与 `internal/app/cron.go`。

## 3. HTTP 接口约定

- 业务前缀固定 `/biz-api/v1`，goim im-api 使用 `/api/v1`，禁止把业务路由挂到 `/api/v1`。
- 路由在 `internal/handler/routes.go` 注册；每域提供 `InitRoutes`。新增依赖要同步装配。
- 统一响应 `{code,msg,data}`；`response.OK/Abort` 都写 HTTP 200。
- 唯一例外：CORS 预检 204；`GET /client-config` 命中 `If-None-Match` 返回 304。
- 业务错误使用 `internal/pkg/errcode` 的 `*errcode.Error`，handler 用 `response.Abort`。
- 未注册路由由 `NoRoute` 返回 HTTP 200 + `1004`；不要用 404 表达业务错误。
- 鉴权：除注册/登录/刷新与公开资料外均需 `Authorization: Bearer`。
- 注册、登录、会话类接口必须带 `X-Device-Id`；平台与版本分别用 `X-Client-Platform`、
  `X-Client-Version`，解析统一走 `internal/pkg/httpx`。
- 新接口必须同时更新 `api/openapi.yaml`、`api/apifox.json`、`api/openapi_test.go` 与前端文档。
  未实现的 B 档能力只写契约，不注册路由。

## 4. 数据与迁移

- 迁移目录保持扁平：`<6 位版本>_<域>_<对象>.up.sql` / `.down.sql`，版本号严格递增。
- 一个迁移只做一件事；跨域变更拆版本；`up`/`down` 必须成对。
- 不建数据库外键；时间列 `DATETIME(3)`；应用层统一 UTC（`internal/pkg/timex`）。
- 已执行过的迁移只增不改；未发布阶段的修正要同步更新 `migrations/README.md` 索引。
- 禁止 `AutoMigrate`；schema 只由迁移管理。
- 仓储查询带 context；事务放入 context 后由 `repository.DBFromContext` 复用。
- 热点表避免 `SELECT *`；批量/列表查询显式 `Select`。
- 条件更新必须把前置状态写进 `WHERE`（如 `status <> deleted`），用 `RowsAffected` 判断是否命中。
- repository 把 `gorm.ErrRecordNotFound` / `gorm.ErrDuplicatedKey` 映射为领域错误。
- 类型转换统一走 `internal/pkg/convert`；不要在业务层手写时间/字符串转换。

## 5. 事务、outbox 与定时任务

- 跨表一致性用 `ports.TxManager.WithTx`；远程调用不放进数据库事务。
- 需要外部系统最终一致的写，在同一事务内插入 `outbox_events`：
  - action 常量集中定义，payload 只放定位所需的最小字段。
  - 注册开通用 `provision_im`，资料更新用 `profile_sync`，注销用 `purge_im_user`。
  - 不要在业务成功后“顺手异步调用一次”就当完成；失败必须可重试。
- worker（`internal/job/im`）处理任务时必须：
  - 先抢任务/按任务 ID 处理，成功或失败都通过 `Settle` 条件落定。
  - 锁内重新读取用户状态；已注销用户不得再写 IM 资料。
  - 开通、资料同步、注销占位共用按用户的 Redis 锁，避免外部投影乱序。
  - 已成功的任务不能被迟到的失败重新打开；状态迁移只允许合法的前置状态。
- 重试按指数退避并写 `last_error`；长期失败要能定位，不要静默吞错。
- 新增 cron 任务实现 `cron.Job` 的 `Name` / `Spec` / `Timeout` / `Run`，并在 `cmd/cron` 注册。
  `Timeout` 必须小于锁 TTL；任务失败只记日志，不 panic。
- 文件清理、账号清除等任务必须可重入、分批、可观测；失败保留状态供下一轮重试。

## 6. 账号生命周期

- 状态：`1` 正常、`2` 冻结、`3` 注销中、`4` 已注销；IM 状态 `0` 待开通、`1` 已开通、
  `2` 失败、`3` 已注销占位。
- 注销请求：事务内进入 `3` 并写 `purge_at`；立即撤销会话、清 Redis 活跃标记与 IM token 缓存。
- 注销后禁止登录/刷新；账号资料不得再同步到 IM。
- 冷却期结束由 `account_purge` 清除本地敏感数据：凭证、会话、设备、资料与文件。
- 当前 IM 注销是 placeholder 模式（昵称占位、faceURL 置空）；不要假设 goim 支持真删用户。
- 新增注销相关逻辑必须覆盖重复注销、并发登录/刷新、IM 不可用、outbox 迟到等场景。

## 7. 配置文件与对象存储

- 配置项必须同时改：配置结构体、`etc/base.yaml`、环境变量覆盖、启动期强校验、README/文档。
- 密钥（DSN、Redis 密码、JWT secret、admin secret、对象存储 AK/SK）只从环境变量读取，仓库留空。
- 可选能力用空值关闭（如 `im.base_url`、`storage.endpoint+bucket`），不能因未配置阻止启动。
- 对象 key 只能由 `domain.BuildObjectKey` 生成，禁止客户端指定任意 key。
- 公开资源只允许 `public/` 前缀；私有附件必须预签名访问。
- ≤8MB 附件走单文件 PUT（`purpose=attachment`），>8MB 才走分片；头像上限 5MB。
- 分片完成或置 ready 时清空 `upload_id`；回收对象时先 abort 未完成分片，再删除对象。
- 删除文件先软删并记录 `object_deleted_at` 为空，由清理任务确认回收；abort 成功也要继续删对象。

## 8. 测试与验证

- 单测用 `testify`；Redis 用 `miniredis`；仓储/缓存用 fake，不连本机中间件。
- 时间、随机数、外部客户端通过函数或接口注入，保证可重复断言。
- 必测：成功路径、参数边界、状态竞争、重复调用幂等、远程失败后的重试。
- 交付前依次执行：
  1. `gofmt -w` 改动文件；
  2. `go build ./...`；
  3. `go vet ./...`；
  4. `go test ./... -count=1`；
  5. 涉及契约时额外 `go test ./api/... -count=1`。
- CI 使用 `go test -race ./...`；涉及并发改动时本地也要跑 `-race`。

## 9. 日志、错误与安全

- 日志用 zap，固定字段 `service/env/version`；访问日志只由中间件打一条。
- 禁止记录密码、access/refresh token、IM token、admin secret、完整身份证/手机号。
- 外部调用失败记录 method、目标、错误摘要，但不记录请求体中的敏感字段。
- 错误码集中注册；新增错误码先确认语义与段位，不要复用不相关码。
- 指标标签禁止 userID/deviceID/IP 等高基数；路由用 `c.FullPath()`，404 归 `unmatched`。
- 限流失败关闭（Redis 异常返回 `9002`）；新增限流要带环境前缀并支持关闭。

## 10. 文档与交付

- 接口变化：OpenAPI + Apifox + 守护测试 + 前端对接文档。
- 行为变化：`docs/DEVELOPMENT.md` 的实现状态、流程与限制。
- 迁移变化：`migrations/README.md` 索引与说明。
- 不提交密钥、本地配置、构建产物与日志。
- 除非用户明确要求，不要执行 git commit；提交信息用 `feat:` / `fix:` / `docs:` / `refactor:`
  加中文描述。
- 完成后向用户说明：改了什么、为什么、验证命令与结果、未提交状态。
