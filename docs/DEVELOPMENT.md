# goim 业务后端技术选型与架构方案

| 项 | 值 |
|---|---|
| 文档类型 | 技术选型 + 架构设计（含实现状态标注） |
| 版本 | v1.1 |
| 日期 | 2026-11-14 |
| 适用客户端 | Web 聊天室、桌面端、后续 iOS/Android |
| 服务代号 | `goim-business` |
| 前置结论 | IM 连接与消息通道继续由 goim 承担；本服务只做业务后端 |

**实现状态图例**：✅ 已实现（当前仓库代码可验证）｜⏳ 规划中（仅有设计/契约，代码未落地）｜❌ 不做（明确不进入当前实现范围）。

> 立项阶段的工作区客户端契约取证材料未随本仓库归档。本文涉及 goim 侧行为的描述（如 admin token、`/im/token`、消息与会话接口）属于蓝图与外部依赖约定，落地前需按 goim 仓库实际接口复核；本文涉及本仓库的部分一律以当前代码为准。

---

## 1. 结论摘要

### 1.1 一句话方案

新建一个 **Go + gin + gorm/MySQL + Redis + zap + Prometheus 的模块化单体服务 `goim-business`**，作为账号与业务身份的唯一事实源。**业务 userID 直接复用为 IM userID，不做任何映射**；通过服务端持有的 admin 凭证向 goim 兑换 IM token，让 Web/桌面端用 `LoginWithToken` 接入，彻底移除前端内置的 `adminSecret`。

**当前落地状态**：账号域（注册、登录、刷新轮换、登出、会话列表、踢会话、改密）、access JWT 鉴权、Redis 会话活跃标记、登录/注册限流、日志/request_id/recovery/metrics、配置加载、数据库迁移、构建与 CI **已实现**；IM token 兑换与账号开通、用户资料、设备、客户端配置、文件、通知/推送、管理端、OAuth **仍为规划中**。本文保留完整蓝图，并逐章标注状态。

### 1.2 实现状态总览

| 能力域 | 状态 | 代码依据 / 说明 |
|---|---|---|
| 账号与会话 | ✅ 已实现 | `internal/handler/account`、`internal/service/account`、三张 `account_*` 表 |
| 鉴权 | ✅ 已实现 | HS256 access JWT + Redis 会话活跃标记 + refresh 轮换/重放检测 |
| 限流 | ✅ 已实现 | 登录/注册按 IP 固定窗口限流，失败关闭；登录失败按账号锁定 |
| 可观测 | ✅ 已实现 | zap JSON、request_id、Recovery、Prometheus 指标、`/healthz`、`/readyz` |
| 交付 | ✅ 已实现 | Makefile、Dockerfile、compose、GitHub Actions、migrations + `cmd/migrate` |
| IM 凭证兑换 / 账号开通 | ⏳ 规划中 | 无 goim client、无 outbox、无 `/im` 路由 |
| 用户资料 | ⏳ 规划中 | 表字段已预留（`avatar_key`/`bio`），无 `/profile` 路由与同步逻辑 |
| 设备 | ⏳ 规划中 | 无 `devices` 表与路由 |
| 客户端配置 | ⏳ 规划中 | 无配置表、无 `/client-config` 路由 |
| 文件 | ⏳ 规划中 | 无 RustFS client、无 `/files` 路由 |
| 通知 / 推送 | ⏳ 规划中 | 无 `notifications` 表、无投递逻辑 |
| 管理端 / OAuth | ⏳ 规划中 | 无管理端路由与角色体系 |
| 多租户 | ❌ 不做 | 首版单租户，依赖 goim 多租户改造后再评估 |

### 1.3 首版目标范围与当前落地状态

**划分原则**：首版只实现“Web/桌面能安全登录、能聊天、资料能显示”的最小闭环。凡是不阻塞这条链路的业务域，接口契约一次铺全，实现留到后续版本。

| 能力 | 首版做法 | 为什么是核心 | 状态 |
|---|---|---|---|
| 账号与鉴权 | 自建 access JWT（默认 15 分钟）+ 不透明 refresh token（默认 30 天，轮换 + 重放检测） | goim 只有 24h access token，无 refresh；终端必须有自己的登录态 | ✅ 已实现 |
| IM 凭证兑换 | `POST /api/v1/im/token`，服务端缓存 admin token 并向 goim 换取 user token | 不解决这一条，前端就只能继续内置 adminSecret，属于阻塞项 | ⏳ 规划中 |
| IM 账号开通 | 注册时通过 outbox 调用 goim `/user/user_register`，资料变更同步 `/user/update_user_info` | 没有 IM user 投影，资料/会话/推送都查不到人 | ⏳ 规划中 |
| 用户资料（最小） | 昵称、头像 fileId、bio；MySQL 为事实源，goim 为投影 | 聊天界面必须显示昵称/头像 | ⏳ 规划中（字段已预留） |
| 设备注册 | deviceID、平台、版本、最后活跃；支撑多端会话与踢出 | 多端登录是 Web + 桌面的现实场景 | ⏳ 规划中（会话层面已记录 deviceID） |
| 客户端配置 | IM 地址、平台映射、上传限制、功能开关；版本 + ETag | 客户端换环境/开关不能靠重新发版 | ⏳ 规划中 |
| 文件（仅头像） | 复用 RustFS：预签名 PUT + complete 校验 + 现签 GET | 头像上传是资料闭环的一部分；聊天附件继续走 goim `/third/*` | ⏳ 规划中 |
| 可观测与交付 | 日志、request_id、Prometheus、健康检查、迁移、单测、compose、CI、镜像、优雅启停 | 上线门槛，不属于业务功能但不能省 | ✅ 已实现（详见 §4.11~§4.13） |

### 1.4 只定义契约、首版不实现

| 业务域 | 首版状态 | 后续补齐的扩展点 |
|---|---|---|
| 会话列表 / 会话扩展属性 | goim 已实现主体，业务侧只给 BFF/扩展契约 | `conversation-ext`：置顶分组、标签、业务备注；仍以 goim 会话为权威 |
| 消息历史 / 搜索 / 归档 | goim 已实现 `/msg/pull_msg_by_seq`、`/msg/search_msg` | 管理端检索、长期归档索引、合规导出 |
| 好友 / 群组管理 | goim 已实现，业务侧不重复 | 业务侧仅做“组织架构/企业通讯录”映射（如未来需要） |
| 通知 / 公告 / 推送 | 只给契约，首版不投递 | 站内信列表、公告、厂商推送（JPush/FCM）、免打扰与折叠 |
| 管理后台 | 只给契约与 OpenAPI，首版不实现（可选先做登录） | 用户/会话/文件/配置/审计/统计，按角色逐步开放 |
| OAuth / 短信登录 | 只给契约与表结构预留 | 第三方身份绑定、验证码服务商接入 |
| 多租户 | ❌ 不做 | 依赖 goim 多租户改造落地后再引入 tenant 维度 |

其余后置项：独立 Kafka/异步队列、APNs/FCM 业务推送、管理后台前端、全链路 OTel Collector、CDN、灰度平台、数据仓库与埋点。

### 1.5 立项时识别的既有问题（⏳ 待核实，当前仓库不包含客户端代码）

1. **`goim-web-chat/.env.local` 明文内置 `VITE_GOIM_ADMIN_SECRET`**，且 `useGoim.ts` 用 `client.login(userID)` 走 admin bootstrap。任何访问者都可为任意 userID 签发 IM token。上线前必须轮换该 secret，并改为业务后端下发 user token。
2. **Web 端平台 ID 错用 1（iOS）**，应为 5（Web）。否则多端会话、token key、推送 token 全部串平台。
3. **goim 是 MongoDB，业务是 MySQL**：用户资料必须单向同步（业务 → goim），禁止双写。

> 上述问题来自立项取证材料，该材料未随仓库归档；本仓库只提供业务后端，相关修复需在客户端仓库完成并在联调阶段复核。

---

## 2. 现状与边界

### 2.1 客户端真实链路（目标蓝图，⏳ IM 部分未实现）

```
Web / 桌面
  │ 1. 业务登录（账号密码 / 刷新令牌）
  ▼
goim-business  ── admin token ──▶ im-api /auth/get_user_token
  │                                      │
  │ 2. 返回 {accessToken, imToken, wsUrl} │
  ▼                                      ▼
客户端 SDK ── LoginWithToken(userID, imToken) ──▶ WS 50001
  │
  ├─ 收发消息：WS 1001/1002/1003 + 2001 推送
  ├─ 会话列表：HTTP /conversation/get_all_conversations
  ├─ 历史消息：本地 SQLite/IndexedDB + /msg/pull_msg_by_seq
  └─ 文件上传：/third/* 预签名 → 直传 RustFS
```

**当前已可用的前半段**：客户端调用 `POST /api/v1/account/register|login|refresh` 获得业务 access token；IM token 兑换与 `LoginWithToken` 尚未接入（⏳）。

### 2.2 职责边界（明确做/不做）

| 能力 | goim-business | goim | 说明 | 状态 |
|---|---|---|---|---|
| 账号密码、注册、登录、refresh、登出 | ✅ 唯一事实源 | ❌ | goim 的 user 是 IM 投影 | ✅ 已实现 |
| IM token 签发 | ✅ 兑换并缓存 | ✅ 实际签发 | 业务不持有 goim JWT 签名密钥，避免越权 | ⏳ 规划中 |
| 用户资料 | ✅ 写 MySQL | ✅ 读投影 | 单向同步 | ⏳ 规划中 |
| 好友/群/会话/消息 | ❌ 不落库 | ✅ | 业务只在管理端做只读检索/聚合 | ❌ 不做（B/C 档） |
| 文件字节 | ❌ 不代理 | ✅ RustFS 直传 | 业务只做签名与元数据 | ⏳ 规划中 |
| 设备与多端会话 | ✅ | ✅ IM 连接维度 | 业务管账号会话，goim 管连接 | 业务会话 ✅ / 设备表 ⏳ |
| 业务通知/公告 | 首版只给契约 | ✅ `/msg/send_business_notification` 投递 | 不在业务侧重造投递通道 | ⏳ 规划中 |
| 管理后台 | 首版只给契约 | 部分只读复用 | 转正后业务后台不直接写 Mongo | ⏳ 规划中 |
| 离线推送 | 首版不接厂商 | ✅ im-push | 后置按业务通知需要再评估 | ⏳ 规划中 |

### 2.3 业务后端必须补充的“现状未覆盖职责”

1. **IM 账号开通器（User Provisioning）**（⏳）：注册/注销时同步 goim user，失败可重试，状态可查询。**业务 userID 即 IM userID**，开通时原样写入，不生成第二套 ID。当前只预留了 `account_users.im_status/im_synced_at` 字段，无同步逻辑。
2. **IM Token Broker**（⏳）：admin token 缓存、按 `(userID, platformID)` 复用 user token、限频。
3. **设备注册表**（⏳）：deviceID、平台、版本、推送 token、最后活跃时间；支撑多端踢出与推送。当前会话表已记录 `device_id`，但没有独立设备表。
4. **业务文件资产**（⏳）：首版只做头像；后续扩展反馈截图、后台素材的对象 key、配额、审计；与聊天附件分桶/分前缀。
5. **配置中心**（⏳）：客户端配置、功能开关、IM 地址与平台映射，版本化下发。
6. **审计日志**（⏳）：管理端写操作留痕（谁、何时、对谁、做了什么、request_id）；B 档转正时启用。
7. **账号生命周期**（⏳）：注销、冻结、解冻、数据导出（合规模块可后置，但表结构首版预留）。当前只有状态常量与冻结登录检查，无注销接口。

### 2.4 业务域全景与实现状态

> 业务域按完整范围设计；实现按“是否阻塞登录聊天闭环”分三档：
> **A 档（原首版实现）** / **B 档（契约先行，仅文档，不注册路由）** / **C 档（委托 goim，业务侧不建实现）**。
> 本表新增“当前状态”列：A 档不等于已实现，以代码为准。

| 业务域 | 分档 | 当前状态 | 首版行为 | 扩展点 |
|---|---|---|---|---|
| 账号 / 会话 / 令牌 | A | ✅ 已实现 | 注册、登录、刷新、登出、会话列表、踢会话、改密 | OAuth、短信、2FA、注销 |
| IM 凭证 | A | ⏳ 规划中 | admin token 缓存 + user token 兑换 + 状态查询（尚未注册路由） | 多租户 issuer、按 App 的 secret |
| 用户资料 | A（最小） | ⏳ 规划中 | 昵称/头像/bio；批量公开资料（尚未注册路由） | 扩展字段、敏感词、资料审核 |
| 设备 | A（最小） | ⏳ 规划中 | 注册、列表、移除（尚未注册路由） | 推送 token、设备风控 |
| 客户端配置 | A | ⏳ 规划中 | 版本化配置 + ETag + schema（尚未注册路由） | 灰度、按用户/租户覆盖、A/B |
| 文件 | A（仅头像） | ⏳ 规划中 | presign、complete、访问 URL（尚未注册路由） | 通用附件、配额、审核、CDN |
| 会话 | C + B | ❌ 不做（委托 goim） | 契约给 BFF/扩展；实现委托 goim | 置顶分组、标签、业务备注 |
| 消息历史/搜索 | C + B | ❌ 不做（委托 goim） | 契约给管理端检索；客户端走 SDK | 归档、全文检索、合规导出 |
| 好友/群组 | C | ❌ 不做 | 不建实现，契约标注委托 | 企业通讯录映射 |
| 通知/公告/推送 | B | ⏳ 规划中 | 只给契约，不注册路由 | 站内信、厂商推送、免打扰 |
| 管理后台 | B | ⏳ 规划中 | 只给契约，首版不实现 | RBAC、审计、统计、批量操作 |
| 关系链风控/举报 | B | ⏳ 规划中 | 只给契约 | 举报、拉黑联动、内容审核 |

分档的落地约束：

- **A 档**目标要求完整实现、单测与迁移；当前仅账号域达到该标准。
- **B 档**只在文档/契约中定义 path/method/schema；**首版不注册路由、不做 9003 占位**，避免无意义的路由与测试开销。
- **C 档**在文档中标注委托 goim，业务后端**不注册同名路由**，避免与 SDK 路径冲突。
- 新增/转正一个 B 档域时，再补 handler 与契约状态标记，不改契约路径。

---

## 3. 总体架构

### 3.1 部署拓扑（目标蓝图 ⏳；当前进程形态 ✅）

```
                    ┌───────────────┐
   Web / 桌面 / App │  Nginx / GW   │  统一入口，按路径分流
                    └───┬───────┬───┘
        /api/v1/account │       │ /api/v1/auth|user|msg|conversation|third
        /api/v1/profile │       │ /ws
        /api/v1/im      ▼       ▼
        /api/v1/files  ┌──────────────┐      ┌──────────────┐
        /api/v1/admin  │ goim-business│      │   im-api     │
                       │  (本服务)    │─────▶│  (18880)     │
                       └──┬───┬───┬───┘      └──┬───────────┘
                          │   │   │             │ gRPC/Etcd
                       MySQL │  Redis          ▼
                          │   │            auth/user/msg/...
                          │   └── RustFS（预签名，业务桶）
                          └── outbox worker（同进程）
```

- **单进程、单镜像**：API + 内置 worker 同二进制，靠配置开关拆分部署。首版不引入服务网格、不引入独立队列。当前只有 API，worker 为规划项 ⏳。
- **路径隔离**：业务命名空间与 SDK 占用路径不重叠，因此 `BaseURL` 仍可保持 `/api/v1`，无需改 SDK 常量。
- **无状态**：会话状态在 MySQL + Redis，实例可水平扩容。

### 3.2 实际请求处理链（✅ 已实现）

```
gin.New()
  → SetTrustedProxies(server.trusted_proxies)
  → Recovery
  → RequestID
  → AccessLog
  → Metrics
  → CORS
  → MaxBytesReader（server.max_body_bytes > 0 时）
  → /api/v1 路由组
       ├─ POST /account/register → RateLimit.Register → Handler
       ├─ POST /account/login    → RateLimit.Login    → Handler
       ├─ POST /account/refresh  → Handler
       └─ 认证子组 → Authenticator → Handler
  → Service → Repository(gorm/MySQL) / Redis
```

原则：中间件只做横切；业务判断全部在 service/domain；repository 不返回 HTTP 语义；外部系统适配在后续 `client` 层（⏳ 尚未创建）。

### 3.3 当前运行时拓扑（✅ 已实现）

单个 `cmd/api` 进程启动三个 HTTP Server，由 `internal/pkg/run.Group` 统一管理生命周期：

| Server | 地址配置 | 默认端口 | 用途 |
|---|---|---|---|
| API | `server.http_addr` | 8080 | `/api/v1` 业务接口 |
| Probe | `server.probe_addr` | 8081 | `/healthz`、`/readyz` |
| Metrics | `server.metrics_addr` | 9091 | `/metrics`（Prometheus 抓取） |

启动顺序：加载配置 → 初始化 zap → 连接 MySQL/Redis（各 5s Ping 超时）→ 装配 `app.App` → 启动三个 Server。任一 Server 异常退出都会触发其他 Server 的优雅关闭（SIGINT/SIGTERM）。

---

## 4. 技术选型决策

> 每项只给一个推荐结论与理由，不铺开候选清单。标注“当前版本”的项以 `go.mod` / 配置文件为准。

### 4.1 语言 / Web 框架 / ORM（✅ 已实现）

| 项 | 当前结论 | 理由 |
|---|---|---|
| 语言 | Go **1.24.0**（`go.mod` 指令） | 与当前工具链一致；实际依赖按 go 1.24 解析 |
| Web | gin **v1.10.0** | 与 im-api 一致，团队零迁移成本；中间件生态成熟 |
| ORM | gorm **v1.25.12** + `gorm.io/driver/mysql` **v1.5.7** | 用 Repository 层隔离，不把 gorm 泄漏到 handler/service |
| MySQL | MySQL 8.x（本地 compose 使用 **mysql:8.4**），`utf8mb4_0900_ai_ci`，InnoDB | 事务、唯一约束满足账号域需求 |
| Redis | Redis 7.x（compose 使用 **redis:7-alpine**），`go-redis/v9` **v9.7.0** | 会话活跃标记、限流、登录失败计数 |

**分层形态**：模块化单体（modular monolith）。当前是“领域模型 + 应用服务 + 仓储实现”的轻量分层：`internal/domain/account` 定义实体/值对象/仓储端口，`internal/service/ports` 定义缓存与事务端口，`internal/repository/account` 提供 gorm 实现。没有 `internal/usecase`，也不采用 DDD 四层重型模板。

### 4.2 配置管理（✅ 已实现，多环境 staging 仅保留支持）

**结论：`spf13/viper` v1.19.0 + 分层 YAML + 环境变量覆盖 + 启动期强校验。**

真实文件与加载顺序：

1. `etc/base.yaml` —— 所有环境共享默认值。
2. `etc/{env}.yaml` —— 环境差异；不存在时忽略（当前仓库只有 `dev.yaml`、`prod.yaml`）。
3. 环境变量 —— 前缀 `GOIM_BIZ_`，层级分隔符 `.` 替换为 `__`（例如 `GOIM_BIZ_MYSQL__DSN`）。
4. `Config.Validate()` 强校验，缺失/越界直接退出。

配置键（以 `internal/config/config.go` 为准）：

| 段 | 键 | 说明 |
|---|---|---|
| `server` | `http_addr`、`probe_addr`、`metrics_addr`、`read_timeout`、`write_timeout`、`idle_timeout`、`shutdown_timeout`、`max_body_bytes`、`trusted_proxies` | `trusted_proxies` 为 IP/CIDR 列表，空表示不信任 `X-Forwarded-For`/`X-Real-IP` |
| `log` | `level`、`format`、`service` | `format=json`（默认）或 `console` |
| `mysql` | `dsn`、`max_open_conns`、`max_idle_conns`、`conn_max_lifetime`、`conn_max_idle_time`、`slow_threshold` | DSN 必填，生产由环境变量注入 |
| `redis` | `addr`、`password`、`db`、`key_prefix` | 键前缀再拼 `:{env}` 形成实际前缀 |
| `auth` | `jwt_secret`、`issuer`、`audience`、`access_ttl`、`refresh_ttl`、`max_sessions_per_user`、`max_sessions_per_platform`、`login_lock_threshold`、`login_lock_window` | 默认值见下 |
| `ratelimit` | `enabled`、`login_per_minute`、`register_per_hour` | 仅登录/注册 IP 限流 |
| `cors` | `allowed_origins` | 列表，`*` 表示回显任意 Origin |

`etc/base.yaml` 关键默认值（原文一致）：

```yaml
server:
  http_addr: "0.0.0.0:8080"
  probe_addr: "0.0.0.0:8081"
  metrics_addr: "0.0.0.0:9091"
  read_timeout: 15s
  write_timeout: 30s
  idle_timeout: 60s
  shutdown_timeout: 15s
  max_body_bytes: 8388608
  trusted_proxies: []

auth:
  jwt_secret: ""                 # 必须由环境变量注入，长度 >= 16
  issuer: "goim-business"
  audience: "goim-business-api"
  access_ttl: 15m
  refresh_ttl: 720h              # 30 天
  max_sessions_per_user: 5
  max_sessions_per_platform: 3
  login_lock_threshold: 10
  login_lock_window: 15m

ratelimit:
  enabled: true
  login_per_minute: 5
  register_per_hour: 3

cors:
  allowed_origins: []            # `["*"]` 表示回显任意 Origin；dev 白名单见 etc/dev.yaml
```

`etc/dev.yaml` 提供本地 DSN、开发 JWT secret 与 `http://localhost:5173` 白名单；`etc/prod.yaml` 只覆盖日志级别、把 `dsn`/`redis.addr` 置空并清空白名单，密钥必须由环境变量注入。

`.env.example` 列出 `GOIM_BIZ_ENV`、`GOIM_BIZ_CONFIG_DIR`、`GOIM_BIZ_MYSQL__DSN`、`GOIM_BIZ_REDIS__ADDR`、`GOIM_BIZ_REDIS__PASSWORD`、`GOIM_BIZ_AUTH__JWT_SECRET`、`GOIM_BIZ_AUTH__COOKIE_SECURE`。其中 `COOKIE_SECURE` 预留给后续 HttpOnly Cookie 方案，当前配置结构体尚未消费。

启动校验规则（✅）：

- `env` 只接受 `dev`、`staging`、`prod`。
- `server.http_addr/probe_addr/metrics_addr` 非空，`max_body_bytes > 0`，`trusted_proxies` 每项必须是合法 IP 或 CIDR。
- `mysql.dsn` 必填；`redis.addr`、`redis.key_prefix` 必填。
- `auth.jwt_secret` 长度 ≥ 16；`issuer/audience` 非空；`refresh_ttl > access_ttl`；`max_sessions_per_platform <= max_sessions_per_user`；`login_lock_threshold/login_lock_window > 0`。

后续规划（⏳）：`im`、`storage`、`features` 等配置段尚未进入结构体；基于 `app_configs` 表 + Redis 的业务可变配置也未实现。密钥类（`mysql.dsn`、`redis.password`、`auth.jwt_secret`、未来的 `im.admin_secret`、对象存储 AK/SK）始终只允许环境变量/K8s Secret，仓库中留空。

### 4.3 依赖注入与项目分层（✅ 已实现）

**结论：手写构造函数注入（`internal/app/app.go` 的 `NewApp`），不引入 `uber/fx` 或 `google/wire`。**

`App` 作为组合根持有配置、logger、`*gorm.DB`、`*sql.DB`、Redis client、仓储、`TxManager`、`SessionCache`、`TokenIssuer`、`RateLimiter` 与 `AccountService`；`cmd/api` 负责加载配置、构造 `App`、启动 Server，`App.Close()` 释放 MySQL/Redis。

分层规则（可作为 code review checklist）：

1. `handler` 只做：绑定/校验 DTO、取上下文、调 service、写响应；不出现 `*gorm.DB`。
2. `service` 只做业务编排与事务边界；不出现 `*gin.Context`。
3. `domain` 定义实体、值对象、领域策略与仓储端口；`service/ports` 定义缓存/事务等应用端口；实现在 `repository`。
4. `repository` 只做持久化；错误统一映射为领域错误（`ErrNotFound`/`ErrDuplicate`）。
5. 跨层使用 `internal/types` 的请求/响应契约与 `internal/domain` 的领域对象；禁止把 gorm model 直接序列化给前端。
6. 未来的 `client`（goim/RustFS）尚未创建（⏳）。

### 4.4 数据库迁移与 gorm 实践（✅ 已实现）

**结论：`golang-migrate/migrate` v4.18.1 + SQL 版本文件 + 独立 `cmd/migrate`，生产禁用 `AutoMigrate`。**

- 迁移文件：`migrations/000001_init.up.sql` / `.down.sql`，由 `migrations/embed.go` 的 `embed.FS` 打包进二进制。
- `cmd/migrate` 支持 `up`、`down`、`version` 三个子命令；自动把 gorm DSN 转成 `mysql://` URL 并补 `multiStatements=true`。
- 规则：
  - 只前进（forward-only）；`down` 仅用于本地回滚（当前 `down` 直接删除三张表）。
  - 一次迁移只做一件事；大表变更拆成“加列 → 回填 → 加约束”多次发布。
  - 禁止在同一次发布中“删列/改类型”与“代码切换”同时发生。
  - 回填脚本独立、可重入、带 batch 与进度日志。
  - 每个迁移在 staging 用生产规模样本演练。

gorm 实际配置（`internal/repository/db.go`）：

```go
db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
    PrepareStmt:            true,
    SkipDefaultTransaction: true,
    TranslateError:         true, // 唯一键冲突 → gorm.ErrDuplicatedKey
    Logger:                 gormLog,
})
sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)
```

- gorm 日志级别为 `Warn`，只记录慢查询/错误，输出到 zap（`sql` 字段）。
- 事务在 service 层通过 `ports.TxManager.WithTx` 显式开启，事务放入 context，由 `repository.DBFromContext` 自动复用。
- 所有查询带 `WithContext(ctx)`；热点表禁止 `SELECT *`（后续列表/批量查询按需显式 `Select`）。
- 时间统一 UTC、`DATETIME(3)`；应用层使用 `internal/pkg/timex`（`Now` 返回 UTC）。
- 类型转换统一使用 `internal/pkg/convert`。
- 不使用 gorm hook 做远程调用。

### 4.5 接口鉴权方案（✅ 账号令牌已实现；⏳ IM token 规划中）

**结论：业务自建双令牌体系；IM token 由业务服务端兑换，客户端只用 `LoginWithToken`。**

#### 令牌设计（以当前代码为准）

| 令牌 | 形态 | 默认 TTL | 存储/传递 | 用途 | 状态 |
|---|---|---|---|---|---|
| access | JWT HS256，含 `sid/pid/did/roles` | 15 分钟（`auth.access_ttl`） | 客户端内存；`Authorization: Bearer` | 调业务 API | ✅ 已实现 |
| refresh | 32 字节安全随机数，Base64 RawURL 编码（不透明） | 30 天（`auth.refresh_ttl=720h`，固定过期，非滑动） | 响应体 `refreshToken`；刷新请求体字段；DB 存 SHA-256 hex | 换新 access/refresh | ✅ 已实现 |
| IM token | goim JWT | 24h（goim 配置） | SDK TokenStore | WS/IM HTTP | ⏳ 规划中 |

access claims（实际签发；`RegisteredClaims` 补 `iss/aud/sub/iat/exp/jti`）：

```json
{
  "iss": "goim-business",
  "aud": "goim-business-api",
  "sub": "123",
  "sid": "456",
  "pid": 5,
  "did": "web-abc",
  "roles": ["user"],
  "iat": 1763000000,
  "exp": 1763000900,
  "jti": "456-1763000000123456789"
}
```

> `sub`/`sid` 是数据库 BIGINT 主键的十进制字符串；`jti` 由 `sid + "-" + 纳秒时间戳` 生成。

鉴权中间件实际流程（`internal/middlewares/auth.go`）：

1. 读取 `Authorization: Bearer <token>`，缺失/空值 → `1002`。
2. `token.Issuer.Parse` 校验 HS256 签名、`iss`、`aud`、`exp`（强制要求过期时间），失败 → `2004`（过期）或 `1002`。
3. Redis `EXISTS {prefix}:sess:active:{sid}`：查询失败 → `9002`；标记不存在 → `1002`（登录态已失效）。
4. 将 `userID`、`sessionID`（int64）、`platform`、`deviceID` 注入 gin.Context；`sub/sid` 解析为 0 时返回 `1001`。

refresh 规则（✅ 已实现）：

- 只在 `account_sessions.refresh_hash` 存 `sha256(token)`，不存明文。
- **每次刷新都轮换**：同一事务内 `MarkRotated` 原子更新旧行 `rotated_at`，再插入新行并继承 `family_id`，`rotated_from=旧 sid`。
- **重放检测**：收到已轮换的旧 token（`rotated_at` 非空）或并发刷新中 `MarkRotated` 返回 false，立即 `RevokeFamily` 撤销该 `family_id` 下全部未撤销会话，并清理 Redis 活跃标记，返回 `2005`。
- 会话维度：`(user_id, device_id, platform)` 仅作为记录，不设唯一约束；策略 `max_sessions_per_user=5`、`max_sessions_per_platform=3`，超限按 `created_at` 从旧到新踢出（当前会话除外）。
- 登出 = 标记当前 session `revoked_at` + 删除 Redis 活跃标记；改密 = 更新哈希 + 撤销当前会话之外的全部会话并清标记。注销（撤销全部会话）未提供接口（⏳）。
- `last_seen_at` 字段已建表但当前代码不更新（无活跃上报接口，⏳）。

**Cookie 方案**：原蓝图设想 Web 用 HttpOnly + Secure + SameSite=Lax Cookie；当前实现统一通过 JSON 响应体下发 `refreshToken`、请求体回传，Cookie 下发/读取未实现（⏳）。`X-Client-Platform` 只影响新会话的平台记录，不区分 refresh 的传递方式。

#### IM token 兑换（⏳ 规划中，未注册路由）

**userID 约束：业务 userID 就是 IM userID，禁止再做映射或二次生成。** 注册落库后把 `account_users.id`（BIGINT）转字符串原样调用 `/user/user_register`；`/auth/get_user_token` 直接传该 userID；响应中的 IM 用户标识也用同一个 `userId`。

```
POST /api/v1/im/token   （业务 access token 鉴权）
  → 校验 user 状态、平台取值合法
  → Redis 命中 im:token:{userID}:{platformID} 且剩余 > 10min：直接返回
  → 否则 singleflight：
       1) 取 admin token（Redis 缓存，提前 5 分钟刷新）
          POST /auth/get_admin_token  {secret, userID:"imAdmin"}
       2) POST /auth/get_user_token {userID, platformID}
          Header: token=<adminToken>, operationID=<uuid>
       3) 写 Redis，TTL = expire - 10min
  → 返回 {userId, token, expireTimeSeconds, apiBaseUrl, wsUrl, platformId}
```

关键点：

- **必须按 `(userID, platformID)` 缓存 IM token**。`allow_multi` 下每平台最多 3 个连接，每次登录都换新 token 会挤掉旧端。
- goim 所有请求必须带 `operationID` 头（否则 1001），必须用 `token` 头而非 `Authorization`。
- admin token 用 singleflight 防击穿；secret 仅存在环境变量。
- 客户端 SDK 配 `TokenProvider`：IM token 临近过期时回调业务 `/im/token`，拿新 token 后 `UpdateToken`/刷新。
- 业务不自行签发 goim JWT，避免与 goim 的 `issuer`/多租户改造冲突。

#### 平台 ID 约定（✅ 服务端解析已实现；IM 下发 ⏳）

| 客户端 | platformID |
|---|---|
| iOS | 1 |
| Android | 2 |
| Windows | 3 |
| macOS | 4 |
| Web | 5 |
| HarmonyOS | 11 |

`internal/pkg/httpx/platform.go` 读取 `X-Client-Platform`（大小写不敏感），支持 `ios/android/windows|win/macos|mac|osx/web|browser/harmony|harmonyos` 以及数字编码；缺省为 Web(5)，非法值返回 `1001`。平台值在注册/登录时写入会话与 access JWT，后续不允许客户端改写会话平台。

### 4.6 参数校验与统一错误码（✅ 已实现）

**结论：`go-playground/validator/v10` v10.23.0 + DTO `binding` 标签 + 集中错误码注册表。**

- 请求 DTO 位于 `internal/types/account.go`，不直接绑定 model；`httpx.BindJSON` 调用 `ShouldBindJSON`，失败统一转 `1001`，并在 `data` 中返回字段级明细（Go 结构体字段名 → 校验 tag，例如 `LoginName: required`）。
- 领域值对象负责业务校验：登录名去空格转小写后必须匹配 `^[a-z0-9_]{3,64}$`；密码 8–64 字节且同时包含字母与数字；昵称去空格，空则回退登录名，最长 64 字符；`deviceId` 非空。
- 业务错误统一 `*errcode.Error{Code, Message, Details}`；handler 用 `response.Abort/Error` 输出。
- **响应 HTTP 状态码恒为 200**（`response.OK`/`Error` 都写 `http.StatusOK`），前端统一按 body 的 `code` 判断；唯一例外是 CORS 预检返回 204。原蓝图“400/401/403/404/409/429 语义化状态码”未实现（⏳）。
- 未注册路由由 `NoRoute` 返回 HTTP 200 + `1004`（“接口不存在”）。

当前错误码枚举（`internal/pkg/errcode/errcode.go`，已实现）：

| 段 | 码 | 含义 |
|---|---|---|
| 成功 | 0 | 成功 |
| 通用 | 1001 / 1002 / 1003 / 1004 / 1005 / 1029 | 参数错误 / 未认证 / 无权限 / 不存在 / 冲突 / 限流 |
| 账号 | 2001 / 2002 / 2003 / 2004 / 2005 / 2006 | 账号已存在 / 凭证错误 / 账号冻结 / access 过期 / refresh 重放 / 会话超限 |
| 内部 | 9001 / 9002 | 内部错误 / 依赖不可用 |

说明：`1003`、`1005`、`2006` 已定义但当前链路未使用（会话超限通过踢出实现，不报错）。`3001+`（资料/设备）、`4001+`（文件）、`5001+`（配置/通知）、`6001+`（管理端）、`9003`（B 档占位）均为**规划中**，未写入代码。

### 4.7 请求链路追踪与日志规范（✅ 已实现基础；trace/OTel ⏳）

**结论：首版用 `request_id + operationID` 打通业务与 goim；OTel 全链路后置。**

- `RequestID` 中间件读取 `X-Request-ID`，兼容 SDK 的 `operationID`；为空或长度 >128 时用 UUIDv7 生成，并回写响应头 `X-Request-ID`。
- 调 goim 时同时带 `X-Request-ID` 与 `operationID`（goim 日志按 operationID 串联）——⏳ 尚未创建 goim client。
- 跨服务透传 W3C `traceparent`（若未来接 OTel collector）——⏳ 未实现。
- 日志：zap JSON（`format=json`），固定字段 `service`、`env`、`version`；访问日志字段为 `request_id`、`user_id`、`method`、`route`、`status`、`error_code`、`latency_ms`、`client_ip`。正常请求以 Debug 级别记录；`status >= 500` 或 `error_code >= 9000` 以 Error 级别记录。
- 每个请求只打一条访问日志（`AccessLog` 中间件）；service 只记录决策点/外部依赖失败，不重复打印访问日志。
- 禁止记录：密码、refresh/access token、IM token、admin secret、完整身份证/手机号；token 只记 `sid`/`jti`。
- `panic` 由 Recovery 捕获，日志包含 `panic`、`request_id`、`route`、堆栈（**最多 15 行**），返回 HTTP 200 + `9001`。
- gorm 慢查询（>200ms，`mysql.slow_threshold`）单独走 Warn 日志，带 SQL 摘要。
- 采样策略（debug/info 全量、健康检查降级）尚未配置（⏳）。

### 4.8 限流与防刷（✅ 登录/注册 IP 限流；其余 ⏳）

**结论：当前实现为 Redis 固定窗口计数（INCR + EXPIRE），按 IP 维度；不是滑动窗口。**

| 场景 | 当前维度 | 默认限额 | 失败策略 | 状态 |
|---|---|---|---|---|
| 登录 | IP | 5 次/分钟（`ratelimit.login_per_minute`） | 失败关闭：Redis 异常返回 `9002` | ✅ |
| 注册 | IP | 3 次/小时（`ratelimit.register_per_hour`） | 失败关闭：Redis 异常返回 `9002` | ✅ |
| 登录失败锁定 | 账号（loginName 归一化） | 连续失败 10 次，窗口 15 分钟（`auth.login_lock_*`） | 达到阈值返回 `1029`；登录成功清零 | ✅ |
| 刷新令牌 | session | 30 次/小时 | — | ⏳ |
| 发验证码 | 账号 + IP | 1 次/60s、5 次/天 | — | ⏳ |
| 业务 API 读 | user | 300 次/分 | — | ⏳ |
| 上传签名 | user | 20 次/分 | — | ⏳ |
| 管理端 | admin + IP | 60 次/分 | — | ⏳ |

限流实现要点：

- Redis 键：`{key_prefix}:{env}:rl:{scope}:{key}`；登录失败键：`{key_prefix}:{env}:login:fail:{loginName}`。
- 超限时回写 `Retry-After`（剩余 TTL 秒数）并返回 `1029`。
- `ratelimit.enabled=false` 或限额 ≤0 时直接放行。
- 限流键带环境前缀，避免多环境共用 Redis 时互相影响。

尚未实现（⏳）：图形验证码、`Idempotency-Key`、请求体大小以外的 URL/查询长度限制与 Content-Type 校验、上传 MIME/扩展名校验、管理端 IP allowlist、高危操作二次确认与审计。

### 4.9 接口文档（✅ 最小 OpenAPI 已落地）

**结论：原计划 OpenAPI 3 设计先行（`api/openapi.yaml`）+ `oapi-codegen` 生成 gin 路由与 DTO。**

当前已在 `api/openapi.yaml` 落地账号域 7 个接口的最小 OpenAPI 3.0.3 契约（路由、DTO、统一响应、错误码与鉴权均按现有实现描述），并由 `api/openapi_test.go` 做守护测试；仍不引入 Swagger UI 与代码生成，`go.mod` 也没有 `oapi-codegen`/`kin-openapi` 依赖。后续全量契约的落地流程保持不变：

1. 先改 `api/openapi.yaml`，PR 中评审契约；只写已实现 operation。
2. `go generate ./...` 重新生成 handler 绑定代码。
3. handler 实现生成接口；B/C 档不生成路由。
4. CI 校验生成物无 diff。
5. 非生产环境挂 Swagger UI；生产关闭。

### 4.10 单元测试与本地依赖编排（✅ 单测/本地 compose；集成测试 ⏳）

**结论：`testify` v1.10.0 + `miniredis/v2` v2.40.0 做单元测试；`docker compose` 编排本地 MySQL/Redis。**

测试分层（实际）：

| 层 | 方式 | 当前状态 |
|---|---|---|
| 配置/工具/缓存 | 表驱动单测，`miniredis` 模拟 Redis | ✅ 已有测试 |
| domain/service | 手写 fake repository/cache 的单测 | ✅ 已有 `service_test.go` |
| handler | `httptest` + gin engine | ⏳ 尚无 |
| repository 集成 | `testcontainers-go` + 真实 MySQL + migrations | ⏳ 未引入依赖 |
| 端到端 | compose + 真实 im-api 冒烟 | ⏳ 未实现 |

约定：

- 覆盖率目标只卡“新增代码 ≥ 60%”，核心是鉴权、令牌轮换、限流。
- 时间、随机数通过接口/函数注入，保证可断言。
- 集成测试禁止依赖开发者本机中间件；统一 `docker compose -f deploy/compose.dev.yaml up -d`（当前只有 MySQL/Redis，RustFS/goim 由各自仓库启动）。

本地编排（`deploy/compose.dev.yaml`，✅）：

```yaml
services:
  mysql:
    image: mysql:8.4
    environment:
      MYSQL_DATABASE: goim_business
      MYSQL_ROOT_PASSWORD: devpass
    ports: ["3306:3306"]
    command: ["--character-set-server=utf8mb4", "--collation-server=utf8mb4_0900_ai_ci"]
    healthcheck: { test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1", "-udevpass", "-pdevpass"], interval: 5s, timeout: 3s, retries: 20 }
  redis:
    image: redis:7-alpine
    ports: ["6379:6379"]
    healthcheck: { test: ["CMD", "redis-cli", "ping"], interval: 5s, timeout: 3s, retries: 10 }
```

### 4.11 构建、CI 与容器化部署（✅ 已实现基础流水线；镜像扫描/发布 ⏳）

**构建（Makefile）**：

- `make build` = `build-api` + `build-migrate`，产出 `bin/api` 与 `bin/migrate`；在 Windows 下自动追加 `.exe`（`bin/api.exe`、`bin/migrate.exe`）。
- 两个二进制均使用 `CGO_ENABLED=0`、`-trimpath`、`-ldflags "-s -w"`；`api` 额外注入 `-X main.version=$(VERSION)`。
- 其他目标：`tidy`、`test`（`go test ./...`）、`vet`（`go vet ./...`）、`run`、`migrate-up`、`migrate-down`、`compose-up`、`compose-down`、`image`。

**Dockerfile（✅）**：`golang:1.24-alpine` 构建，产出 `/out/api` 与 `/out/migrate`；运行阶段基于 `gcr.io/distroless/static-debian12:nonroot`，复制 `etc/` 到 `/app/etc` 并设 `GOIM_BIZ_CONFIG_DIR=/app/etc`，非 root 运行，暴露 8080/8081/9091，入口 `/app/api`。

**CI（`.github/workflows/ci.yml`，✅）**：在 push 到 main/master 与 PR 时触发，使用 Go 1.24，依次执行 `go mod download`、`go build ./...`、`go vet ./...`、`go test -race ./...`。

尚未实现（⏳）：golangci-lint、OpenAPI diff、testcontainers 集成 job、镜像构建/推送、trivy 扫描、staging 部署、迁移 Job、冒烟与回滚流水线。

### 4.12 优雅启停与健康检查（✅ 基础实现；部分门禁 ⏳）

**结论：`run.Group` + `signal.NotifyContext` 风格信号处理 + 三 Server 协同关闭。**

当前行为：

1. 收到 `SIGINT`/`SIGTERM`（或任一 Server 异常退出）触发中断。
2. 对 API/Probe/Metrics 三个 `http.Server` 并发调用 `Shutdown`，超时取 `server.shutdown_timeout`（默认 15s）。
3. `App.Close()` 关闭 Redis 与 MySQL 连接，日志 flush。

健康检查：

| 端点 | 端口 | 语义 | 当前内容 |
|---|---|---|---|
| `GET /healthz` | 8081 | liveness | 直接返回 `ok` |
| `GET /readyz` | 8081 | readiness | MySQL `Ping` + Redis `Ping`，任一失败返回 503 |
| `GET /metrics` | 9091 | 指标 | `promhttp.Handler()` |

尚未实现（⏳）：`pre_stop_delay`、readyz 中的迁移版本匹配、独立 worker 的停机等待、goim 可用性指标与降级逻辑。

### 4.13 Prometheus 指标落地（✅ 基础指标；业务指标 ⏳）

**结论：`prometheus/client_golang` v1.20.5 + 手工注册（`sync.Once`），独立端口暴露。**

| 指标 | 类型 | 标签 | 状态 |
|---|---|---|---|
| `goim_business_http_requests_total` | Counter | `method`、`route`、`status` | ✅ |
| `goim_business_http_request_duration_seconds` | Histogram | `method`、`route` | ✅ |
| `http_in_flight_requests`、请求/响应大小 | Gauge/Histogram | — | ⏳ |
| `db_pool_*` | Gauge/Histogram | — | ⏳ |
| `redis_command_duration_seconds` | Histogram | `cmd` | ⏳ |
| `auth_login_total`、`auth_refresh_total` | Counter | `result`、`platform` | ⏳ |
| `im_token_issue_total`、`goim_dependency_duration_seconds` | Counter/Histogram | `result`、`api` | ⏳ |
| `outbox_events_total`、`outbox_pending` | Counter/Gauge | `topic` | ⏳ |
| `file_presign_total` | Counter | `purpose`、`result` | ⏳ |

规则：`route` 用 `c.FullPath()`，404 归为 `unmatched`；禁止把 userID/deviceID/IP 放进标签；Histogram 使用默认桶。Grafana 看板与告警规则尚未落地（⏳）。

### 4.14 消息队列 / 异步任务的必要性判断（⏳ 规划中，未实现）

**结论：首版不引入 Kafka；用 MySQL 事务性 Outbox + 同进程 worker。**

判断依据：

- 业务的异步需求只有四类：IM 用户开通/资料同步、审计异步落库、文件清理、通知生成。全部是低频、可重试、允许秒级延迟。
- goim 已有 Kafka，但那是消息扇出通道，业务服务接入会引入消费组、offset、运维与重复消费治理，收益不足。
- Redis Stream/asynq 也可行，但关键链路（账号创建 → IM 开通）需要“与业务事务同生共死”，MySQL outbox 最稳。

目标表结构（⏳ 未创建；主键沿用当前 BIGINT 约定）：

```sql
CREATE TABLE outbox_events (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  topic         VARCHAR(64)  NOT NULL,
  aggregate_id  VARCHAR(64)  NOT NULL,
  payload       JSON         NOT NULL,
  status        TINYINT      NOT NULL DEFAULT 0, -- 0待处理 1成功 2失败
  retry_count   INT          NOT NULL DEFAULT 0,
  next_retry_at DATETIME(3)  NOT NULL,
  created_at    DATETIME(3)  NOT NULL,
  updated_at    DATETIME(3)  NOT NULL,
  KEY idx_status_retry (status, next_retry_at)
);
```

worker 计划用 `SELECT ... FOR UPDATE SKIP LOCKED` 批量取任务，指数退避（1m/5m/30m/2h/6h，上限 8 次），超过阈值进死信状态并告警。worker 与 API 同二进制，通过配置开关启用。当前 `cmd/api` 没有 worker。

升级路径：当出现跨团队消费、峰值 >1000 TPS 或需要多订阅方时，再把 outbox 投递到 Kafka；业务代码只需替换 dispatcher。

### 4.15 对象存储选型（⏳ 规划中，未实现）

**结论：复用现有 RustFS（S3 兼容），业务资产独立 bucket；AWS SDK v2 直接预签名，不经过 goim。**

- 聊天附件继续走 goim `/third/*`（SDK 已封装）。
- 业务资产（首版头像；后续反馈截图、后台素材）走 `goim-business` 自己的 bucket `goim-business`，前缀 `avatar/`、`feedback/`、`admin/`。
- 流程：`POST /files/presign` 返回 PUT URL（10 分钟）→ 客户端直传 → `POST /files/complete` 后服务端 `HeadObject` 校验大小/类型 → 落 `file_objects` → 展示时 `GET /files/{id}/url` 现签 GET URL。
- 安全：私有桶；AccessKey/SecretKey 只在环境变量；预签名锁 Content-Type 与大小；对象 key 用 `purpose/ownerID/uuid`，禁止客户端指定任意 key；文件名不参与路径。
- 清理：定时任务删除 `status=uploading` 超 24h 的孤儿对象；软删文件延迟 7 天物理删除。
- CDN 后置：先由 Nginx 反代 RustFS；量起来后再上 CDN + 签名回源。

当前无 `storage` 配置段、无 AWS SDK 依赖、无 `client/storage` 包（⏳）。

### 4.16 推送通道选型（⏳ 规划中，不实现）

**结论：聊天离线推送沿用 `im-push`；业务侧首版不接入任何厂商推送。**

- `im-push` 已实现 GeTui / FCM / JPush，配置 `OfflinePush.Enable` 切换，无需新代码。
- 首版启用 **JPush**：一套 SDK 覆盖 iOS/Android/鸿蒙，接入最快；海外或需要 FCM 原生能力时再切/并存。
- 设备 token 注册：业务 `devices` 表存 token，同时调用 goim `/user/set_user_client_config` 写 `push_token_<PlatformName>`。
- 业务通知（账号安全、公告、审批提醒）首版**只给契约（B 档）**，不做投递；转正时写 `notifications` 表 + 通过 `/msg/send_business_notification` 投递到 IM 会话。
- 后置条件：当“离线用户必须收到业务通知”成为硬需求时，再在 worker 中接 JPush/FCM，并做去重、折叠、免打扰时段。
- 桌面端在线靠 WebSocket，离线通知后置（Windows 通知中心/系统托盘由客户端实现）。

---

## 5. 代码结构

### 5.1 当前代码结构（✅ 已实现，2~3 层）

```
goim-business/
├── cmd/
│   ├── api/main.go                 # API 进程：配置→logger→App→api/probe/metrics 三 Server
│   └── migrate/main.go             # 迁移入口：up/down/version
├── etc/
│   ├── base.yaml                   # 全环境默认配置
│   ├── dev.yaml                    # 本地差异（DSN/JWT secret/CORS）
│   └── prod.yaml                   # 生产差异（密钥留空，由环境变量注入）
├── migrations/
│   ├── 000001_init.up.sql          # account_users / account_credentials / account_sessions
│   ├── 000001_init.down.sql
│   └── embed.go                    # embed.FS
├── api/
│   ├── openapi.yaml                # 账号域最小 OpenAPI 3.0.3 契约
│   └── openapi_test.go             # 契约守护测试
├── internal/
│   ├── app/app.go                  # 手写组合根 NewApp / Close
│   ├── cache/session_cache.go      # Redis 会话活跃标记 + 登录失败计数
│   ├── config/
│   │   ├── config.go               # 配置结构体、加载、强校验
│   │   └── config_test.go
│   ├── domain/account/             # 实体、值对象、策略、错误、仓储端口
│   │   ├── user.go
│   │   ├── credential.go
│   │   ├── session.go
│   │   ├── repository.go
│   │   ├── policy.go
│   │   ├── value_object.go
│   │   └── errors.go
│   ├── handler/
│   │   ├── routes.go               # gin.New + 全局中间件 + /api/v1
│   │   └── account/
│   │       ├── routes.go           # 账号路由注册
│   │       └── account.go          # 7 个 handler
│   ├── middlewares/
│   │   ├── auth.go                 # Bearer access JWT + Redis 活跃标记
│   │   ├── context.go              # gin.Context 键与读取函数
│   │   ├── cors.go
│   │   ├── logger.go               # AccessLog
│   │   ├── metrics.go              # Prometheus + MetricsHandler
│   │   ├── ratelimit.go            # 登录/注册 IP 限流
│   │   ├── recovery.go             # panic 捕获，堆栈截断 15 行
│   │   └── requestid.go            # X-Request-ID / operationID / UUIDv7
│   ├── model/account.go            # gorm 持久化记录（account_* 表）
│   ├── pkg/                        # 无业务语义基础设施
│   │   ├── convert/                # 类型转换
│   │   ├── errcode/                # 错误码注册表
│   │   ├── hashx/                  # SHA-256 等
│   │   ├── httpx/                  # BindJSON、X-Client-Platform 解析
│   │   ├── idgen/                  # UUIDv7
│   │   ├── logger/                 # zap 构造与 Logger 接口
│   │   ├── password/               # argon2id
│   │   ├── randx/                  # 安全随机数
│   │   ├── ratelimit/              # Redis 固定窗口
│   │   ├── response/               # {code,msg,data}
│   │   ├── run/                    # actor runner（优雅关闭）
│   │   ├── text/                   # 截断
│   │   ├── timex/                  # UTC 时间
│   │   └── token/                  # access JWT + opaque refresh
│   ├── repository/
│   │   ├── db.go                   # gorm 连接/连接池/Ping
│   │   ├── tx.go                   # TxManager + DBFromContext
│   │   └── account/                # user/credential/session 仓储实现
│   ├── service/
│   │   ├── account/                # 注册/登录/刷新/会话/改密 + 单测
│   │   └── ports/ports.go          # SessionStateCache / LoginThrottle / TxManager
│   └── types/account.go            # 请求/响应契约
├── deploy/compose.dev.yaml         # MySQL 8.4 + Redis 7
├── docs/DEVELOPMENT.md             # 本文
├── .github/workflows/ci.yml
├── Dockerfile
├── Makefile
├── go.mod / go.sum
└── .env.example
```

> 注意：仓库中不存在 `configs/`、`internal/usecase`、`internal/middleware`（实际目录名是 `internal/middlewares`）、`internal/client`、`internal/job`、`test/integration`。这些属于后续蓝图（见 §5.3）。

### 5.2 一次完整请求的中间件处理链（✅）

以 `POST /api/v1/account/login` 为例：

1. `gin.New()`（不启用 gin 默认 Logger/Recovery），`SetTrustedProxies(server.trusted_proxies)`。
2. `Recovery`：注册 defer，捕获 panic 并记录截断堆栈。
3. `RequestID`：读取 `X-Request-ID`/`operationID` 或生成 UUIDv7，写入 context 与响应头。
4. `AccessLog`：开始计时，请求结束后输出一条访问日志。
5. `Metrics`：开始计时，结束后累加 requests counter 与 duration histogram。
6. `CORS`：按白名单回显 Origin；OPTIONS 直接 204。
7. `MaxBytesReader`：按 `server.max_body_bytes` 限制请求体。
8. 进入 `/api/v1/account` 组，命中 `RateLimit.Login()`：以 `c.ClientIP()` 为键做 Redis 固定窗口计数；失败关闭。
9. `httpx.BindJSON` 绑定 `types.LoginRequest`（`loginName/password/deviceId`），`httpx.PlatformFrom` 解析 `X-Client-Platform`（缺省 Web），补充 `UserAgent/IP/Platform`。
10. `AccountService.Login`：读取登录失败计数 → 查用户/凭证 → argon2id 校验 → 清零失败计数 → 事务内更新 `last_login_at` 并创建会话 → 会话上限踢出 → 签发 access/refresh → 写 Redis 活跃标记。
11. `response.OK` 输出 HTTP 200 + `{code:0,msg:"success",data:{user,token,refreshToken,refreshExpiresIn,sessionId,platform}}`；请求返回时由 Metrics 与 AccessLog 收尾。

认证请求（如 `POST /api/v1/account/logout`）在路由组内额外经过 `Authenticator`：验签 → 校验 Redis `sess:active:{sid}` → 注入用户/会话上下文。

### 5.3 目标目录结构（蓝图，⏳ 未落地部分）

```
goim-business/
├── cmd/{api,migrate}/
├── etc/{base,dev,staging,prod}.yaml
├── api/openapi.yaml                # ✅ 账号域契约事实源
├── migrations/
├── internal/
│   ├── app/
│   ├── config/
│   ├── domain/{account,profile,device,file,im,clientconfig}/   # ⏳ 其他域
│   ├── handler/{account,profile,im,device,file,clientconfig,notification,admin,internal}/
│   ├── middlewares/                # 现有目录；未来增加 adminauth.go、idempotency.go
│   ├── service/{account,profile,imtoken,device,file,clientconfig,notification,admin,outbox}/
│   ├── repository/{account,profile,device,file,config,notification,audit,outbox}/
│   ├── client/{goim,storage}/      # ⏳ goim HTTP / RustFS 预签名
│   ├── job/{outbox_worker,file_cleanup,scheduler}/  # ⏳
│   ├── model/、types/、pkg/
├── deploy/{compose.dev.yaml,k8s/}   # ⏳ k8s
├── test/{integration,e2e}/          # ⏳
├── Dockerfile、Makefile、go.mod
└── README.md
```

约定：

- 每个业务域在 handler/service/domain/repository 下同名子包，禁止跨域直接 import service。
- `client/goim` 是唯一允许出现 `operationID`/`token` 头的地方（未来）。
- `pkg/` 只放无业务语义的基础设施；出现业务名词即下沉到 service/domain。

---

## 6. 核心依赖清单（以 `go.mod` 为准）

| 依赖 | 版本 | 用途 | 状态 |
|---|---|---|---|
| `github.com/gin-gonic/gin` | v1.10.0 | HTTP 框架 | ✅ |
| `gorm.io/gorm` | v1.25.12 | ORM | ✅ |
| `gorm.io/driver/mysql` | v1.5.7 | MySQL 驱动 | ✅ |
| `github.com/redis/go-redis/v9` | v9.7.0 | Redis | ✅ |
| `github.com/spf13/viper` | v1.19.0 | 配置 | ✅ |
| `github.com/go-playground/validator/v10` | v10.23.0 | 校验 | ✅ |
| `go.uber.org/zap` | v1.27.0 | 日志 | ✅ |
| `github.com/prometheus/client_golang` | v1.20.5 | 指标 | ✅ |
| `github.com/golang-migrate/migrate/v4` | v4.18.1 | 迁移 | ✅ |
| `github.com/golang-jwt/jwt/v5` | v5.2.1 | access token | ✅ |
| `golang.org/x/crypto` | v0.31.0 | argon2id | ✅ |
| `github.com/google/uuid` | v1.6.0 | UUIDv7 | ✅ |
| `github.com/stretchr/testify` | v1.10.0 | 断言 | ✅ |
| `github.com/alicebob/miniredis/v2` | v2.40.0 | Redis 单测 | ✅ |
| `github.com/oapi-codegen/oapi-codegen/v2` | — | OpenAPI 代码生成 | ⏳ 未引入 |
| `github.com/getkin/kin-openapi` | — | 契约校验 | ⏳ 未引入 |
| `github.com/aws/aws-sdk-go-v2/service/s3` | — | RustFS 预签名 | ⏳ 未引入 |
| `github.com/testcontainers/testcontainers-go` | — | 集成测试 | ⏳ 未引入 |
| `go.uber.org/mock` | — | mock 生成 | ⏳ 未引入 |
| `github.com/golangci/golangci-lint` | — | 静态检查 | ⏳ 未接入 CI |

不引入：`uber/fx`、`wire`、`ent`、`sqlx`、消息队列客户端（首版）、服务网格 SDK。时间/转换工具已内置为 `internal/pkg/timex`、`internal/pkg/convert`，不依赖外部 goim 仓库的 `pkg/utils`。

---

## 7. 数据模型（M1 已建三张账号表，其余为设计预留 ⏳）

> 所有表：`id BIGINT UNSIGNED AUTO_INCREMENT` 主键；对前端与 IM 下发时由应用层转为字符串。
> 时间列：`DATETIME(3)`，应用层统一写入 UTC。
> 外键：不使用数据库外键，全部为逻辑外键，由应用层保证引用一致性。
> **M1 只落地 `account_users`、`account_credentials`、`account_sessions`**；其余表是后续域的设计预留，转正时再新增迁移。

### 7.1 `account_users`（✅ 已实现）

```sql
CREATE TABLE account_users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '业务 userID；对 IM 下发时转为字符串',
  login_name    VARCHAR(64)  NOT NULL COMMENT '登录名：去空格并转小写后存储，3-64 位，仅字母/数字/下划线',
  nickname      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '昵称；注册时为空则回退为 login_name，最多 64 个字符',
  avatar_key    VARCHAR(255) NOT NULL DEFAULT '' COMMENT '头像对象存储 key，不存完整 URL',
  bio           VARCHAR(255) NOT NULL DEFAULT '' COMMENT '个人简介',
  status        TINYINT      NOT NULL DEFAULT 1 COMMENT '1正常 2冻结 3注销中 4已注销',
  im_status     TINYINT      NOT NULL DEFAULT 0 COMMENT 'IM 开通状态：0待开通 1已开通 2失败',
  im_synced_at  DATETIME(3)  NULL COMMENT '最近一次与 IM 同步成功的时间',
  last_login_at DATETIME(3)  NULL COMMENT '最近一次登录成功时间',
  created_at    DATETIME(3)  NOT NULL,
  updated_at    DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_login_name (login_name),
  KEY idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

> `id` 即业务 userID，未来注册 IM 时原样转字符串传给 goim `/user/user_register`，不设 `im_user_id`、不做映射。`avatar_key/bio/im_status/im_synced_at` 当前只建表，无资料/同步接口。

### 7.2 `account_credentials`（✅ 已实现）

```sql
CREATE TABLE account_credentials (
  user_id       BIGINT UNSIGNED NOT NULL PRIMARY KEY COMMENT '逻辑关联 account_users.id',
  password_hash VARCHAR(255) NOT NULL COMMENT 'argon2id 编码串',
  updated_at    DATETIME(3)  NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 7.3 `account_sessions`（✅ 已实现）

```sql
CREATE TABLE account_sessions (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT 'sid；写入 JWT 的 sid 声明后以字符串输出',
  user_id      BIGINT UNSIGNED NOT NULL,
  family_id    CHAR(36)     NOT NULL COMMENT '刷新轮换家族 ID（UUID）；同一登录链路共享，重放时整族撤销',
  refresh_hash CHAR(64)     NOT NULL COMMENT 'refresh token 的 SHA-256 十六进制摘要；只存摘要',
  rotated_from BIGINT UNSIGNED NULL COMMENT '由哪个会话轮换而来；NULL 为首登',
  platform     TINYINT      NOT NULL COMMENT '1 iOS 2 Android 3 Windows 4 Mac 5 Web 11 HarmonyOS',
  device_id    VARCHAR(64)  NOT NULL,
  user_agent   VARCHAR(255) NOT NULL DEFAULT '',
  ip           VARCHAR(45)  NOT NULL DEFAULT '',
  expires_at   DATETIME(3)  NOT NULL,
  revoked_at   DATETIME(3)  NULL,
  rotated_at   DATETIME(3)  NULL,
  last_seen_at DATETIME(3)  NULL,
  created_at   DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_refresh_hash (refresh_hash),
  KEY idx_user_platform (user_id, platform),
  KEY idx_family (family_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

生命周期与轮换家族语义：

- 创建：`revoked_at`/`rotated_at` 均为 NULL，`family_id` 为 UUID。
- 轮换：旧行 `rotated_at` 写入时间，新行 `family_id` 继承旧行、`rotated_from=旧 sid`；同一家族的所有会话共享“一次登录链”。
- 撤销：登出/踢出/改密写 `revoked_at`；重放检测按 `family_id` 整族撤销。
- 活跃判定：`revoked_at IS NULL AND rotated_at IS NULL AND expires_at > now`；Redis 活跃标记 TTL 与 `expires_at` 对齐。
- 索引：`uk_refresh_hash` 支撑精确定位；`idx_user_platform` 支撑每平台会话上限；`idx_family` 支撑整族撤销。

### 7.4 `devices`（⏳ 规划中，未建表）

```sql
CREATE TABLE devices (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id        BIGINT UNSIGNED NOT NULL,
  device_id      VARCHAR(64)  NOT NULL,
  platform       TINYINT      NOT NULL,
  model          VARCHAR(64)  NOT NULL DEFAULT '',
  os_version     VARCHAR(32)  NOT NULL DEFAULT '',
  app_version    VARCHAR(32)  NOT NULL DEFAULT '',
  push_token     VARCHAR(255) NOT NULL DEFAULT '',
  push_provider  VARCHAR(16)  NOT NULL DEFAULT '',
  last_active_at DATETIME(3)  NULL,
  status         TINYINT      NOT NULL DEFAULT 1,
  UNIQUE KEY uk_user_device (user_id, device_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 7.5 `file_objects`（⏳ 规划中，未建表）

```sql
CREATE TABLE file_objects (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  owner_user_id BIGINT UNSIGNED NOT NULL,
  purpose       VARCHAR(32)  NOT NULL,      -- avatar/feedback/admin
  bucket        VARCHAR(64)  NOT NULL,
  object_key    VARCHAR(255) NOT NULL,
  mime          VARCHAR(128) NOT NULL,
  size_bytes    BIGINT       NOT NULL DEFAULT 0,
  sha256        CHAR(64)     NOT NULL DEFAULT '',
  status        TINYINT      NOT NULL DEFAULT 0, -- 0待上传 1就绪 2已删除
  created_at    DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_object_key (bucket, object_key),
  KEY idx_owner (owner_user_id, purpose),
  KEY idx_status_created (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 7.6 `app_configs`（⏳ 规划中，未建表）

```sql
CREATE TABLE app_configs (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  config_key VARCHAR(64)  NOT NULL,
  value      JSON         NOT NULL,
  version    INT          NOT NULL DEFAULT 1,
  status     TINYINT      NOT NULL DEFAULT 1,  -- 1生效 0下线
  updated_by BIGINT UNSIGNED NOT NULL DEFAULT 0,
  updated_at DATETIME(3)  NOT NULL,
  UNIQUE KEY uk_key (config_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 7.7 `notifications`（⏳ 规划中，未建表）

```sql
CREATE TABLE notifications (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT UNSIGNED NOT NULL,
  type       VARCHAR(32)  NOT NULL,
  title      VARCHAR(128) NOT NULL,
  body       VARCHAR(1024) NOT NULL,
  payload    JSON         NULL,
  read_at    DATETIME(3)  NULL,
  created_at DATETIME(3)  NOT NULL,
  KEY idx_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

### 7.8 `audit_logs`（⏳ 规划中，未建表）

```sql
CREATE TABLE audit_logs (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY,
  actor_id    BIGINT UNSIGNED NOT NULL,
  action      VARCHAR(64)  NOT NULL,
  resource    VARCHAR(64)  NOT NULL,
  resource_id VARCHAR(64)  NOT NULL DEFAULT '',
  detail      JSON         NULL,
  ip          VARCHAR(45)  NOT NULL DEFAULT '',
  request_id  VARCHAR(64)  NOT NULL DEFAULT '',
  created_at  DATETIME(3)  NOT NULL,
  KEY idx_actor_created (actor_id, created_at),
  KEY idx_resource (resource, resource_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci;
```

`outbox_events` 见 §4.14（⏳）。管理员角色未来用 `admin_users(user_id, role, status)` 单表，权限点后续再拆（⏳）。

---

## 8. 关键接口分组

统一前缀 `/api/v1`；业务响应体 `{code,msg,data}`，`X-Request-ID` 回写。

鉴权标记：🔓 公开、🔐 用户 access token、🛡 管理员 token、🖥 内部/探针。

实现标记：**✅ 已注册路由**、**⏳ 规划中（未注册路由）**、**❌ 不做（委托 goim 或不实现）**。

### 8.1 账号 `/account`（✅ 已实现）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| POST | `/account/register` | 🔓 | 注册（登录名+密码+deviceId），成功后直接返回令牌与会话信息 | ✅ |
| POST | `/account/login` | 🔓 | 登录，按 `X-Client-Platform` 建会话 | ✅ |
| POST | `/account/refresh` | 🔓(refresh) | 轮换 refresh，返回新 access + 新 refresh | ✅ |
| POST | `/account/logout` | 🔐 | 撤销当前会话 | ✅ |
| GET | `/account/sessions` | 🔐 | 当前用户活跃会话列表 | ✅ |
| POST | `/account/sessions/revoke` | 🔐 | 按 body `sessionId` 踢指定会话 | ✅ |
| PUT | `/account/password` | 🔐 | 改密，撤销其他会话 | ✅ |
| DELETE | `/account` | 🔐 | 注销（冷却期 + 异步清理） | ⏳ 未注册路由 |

`POST /account/login` 实际请求/响应（`types.LoginRequest` / `types.AuthResp`）：

```json
// request（Header: X-Client-Platform: web 可省略，默认 5）
{ "loginName": "alice", "password": "••••••", "deviceId": "web-abc" }
// response（HTTP 200）
{
  "code": 0,
  "msg": "success",
  "data": {
    "user": { "userId": "123", "loginName": "alice", "nickname": "Alice" },
    "token": { "accessToken": "eyJ...", "expiresIn": 900 },
    "refreshToken": "opaque-32-bytes...",
    "refreshExpiresIn": 2592000,
    "sessionId": "456",
    "platform": 5
  }
}
```

注册请求额外支持可选 `nickname`；refresh 请求为 `{ "refreshToken": "..." }`；踢会话请求为 `{ "sessionId": "456" }`；改密请求为 `{ "oldPassword": "...", "newPassword": "..." }`。当前响应**不包含 IM 信息**；`im` 字段属于 M2 蓝图（⏳）。

### 8.2 IM 凭证 `/im`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| POST | `/im/token` | 🔐 | 获取/刷新当前设备平台的 IM token | ⏳ |
| GET | `/im/status` | 🔐 | IM 账号开通状态与重试 | ⏳ |

请求：`{ "platformId": 5, "deviceId": "web-abc" }`（platform 由服务端从会话推导，客户端传值仅校验）。
响应：`{ userId, token, expireTimeSeconds, apiBaseUrl, wsUrl, platformId, status }`。`userId` 同时就是 IM userID。

### 8.3 资料 `/profile`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| GET | `/profile/me` | 🔐 | 当前用户资料（头像返回预签名 GET URL） | ⏳ |
| PUT | `/profile/me` | 🔐 | 更新昵称/头像 fileId/bio，同步 goim | ⏳ |
| POST | `/profile/batch` | 🔐 | 批量取公开资料（昵称/头像） | ⏳ |
| GET | `/profile/users/{userId}` | 🔐 | 单个公开资料 | ⏳ |
| PUT | `/profile/me/extended` | 🔐 | 扩展字段（地区/性别/签名/自定义 JSON） | ⏳ |

### 8.4 设备 `/devices`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| POST | `/devices/register` | 🔐 | 注册/更新设备、推送 token | ⏳ |
| GET | `/devices` | 🔐 | 我的设备列表 | ⏳ |
| DELETE | `/devices/{deviceId}` | 🔐 | 移除设备并撤销其会话 | ⏳ |

### 8.5 客户端配置 `/client-config`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| GET | `/client-config?platform=5&appVersion=1.0.0` | 🔓 | 返回配置 + `ETag`/`configVersion`；304 支持 | ⏳ |
| GET | `/client-config/schema` | 🔓 | 配置项元数据（类型/默认值/适用范围） | ⏳ |

响应示例：

```json
{
  "configVersion": 12,
  "features": { "voiceMessage": false, "fileUpload": true },
  "im": { "apiBaseUrl": "...", "wsUrl": "...", "platformId": 5 },
  "limits": { "maxUploadBytes": 104857600, "maxAvatarBytes": 5242880 },
  "minAppVersion": { "web": "1.0.0", "desktop": "1.0.0" }
}
```

### 8.6 文件 `/files`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| POST | `/files/presign` | 🔐 | 申请上传：校验 purpose/mime/size，返回 fileId + PUT URL | ⏳ |
| POST | `/files/{fileId}/complete` | 🔐 | 客户端上传完成后校验对象并置为 ready | ⏳ |
| GET | `/files/{fileId}/url` | 🔐 | 现签 GET URL | ⏳ |
| DELETE | `/files/{fileId}` | 🔐 | 软删（仅本人/管理端） | ⏳ |
| POST | `/files/chat-attachment/presign` | 🔐 | 通用聊天附件签名 | ❌ 不做（直接走 goim `/third/*`） |

`POST /files/presign` 目标请求/响应：

```json
// request
{ "purpose": "avatar", "fileName": "me.png", "mime": "image/png", "size": 120000, "sha256": "..." }
// response
{ "fileId": "18", "objectKey": "avatar/123/uuid.png", "uploadUrl": "https://...X-Amz-Signature=...",
  "method": "PUT", "headers": { "Content-Type": "image/png" }, "expiresAt": 1763000600 }
```

### 8.7 通知 `/notifications`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| GET | `/notifications` | 🔐 | 分页列表（站内信） | ⏳ |
| POST | `/notifications/read` | 🔐 | 批量已读 | ⏳ |
| GET | `/notifications/unread-count` | 🔐 | 未读数 | ⏳ |

首版行为：**不注册路由**；契约仅保留在文档中。当前未建 `notifications` 表。

### 8.8 管理端 `/admin`（⏳ 规划中，未注册路由）

| 方法 | 路径 | 鉴权 | 说明 | 状态 |
|---|---|---|---|---|
| POST | `/admin/auth/login` | 🔓 | 管理员登录（独立会话、严格限流） | ⏳ |
| GET | `/admin/users` | 🛡 | 分页/搜索 | ⏳ |
| GET | `/admin/users/{id}` | 🛡 | 详情（会话、设备、文件） | ⏳ |
| PATCH | `/admin/users/{id}/status` | 🛡 | 冻结/解冻 | ⏳ |
| GET | `/admin/sessions` | 🛡 | 在线/历史会话 | ⏳ |
| POST | `/admin/sessions/{sid}/revoke` | 🛡 | 强制下线 | ⏳ |
| GET | `/admin/files` | 🛡 | 文件列表/审计 | ⏳ |
| DELETE | `/admin/files/{id}` | 🛡 | 删除对象与元数据 | ⏳ |
| GET | `/admin/configs` | 🛡 | 配置列表 | ⏳ |
| PUT | `/admin/configs/{key}` | 🛡 | 写入并递增版本 | ⏳ |
| POST | `/admin/announcements` | 🛡 | 发公告（写 notifications + 调 goim 业务通知） | ⏳ |
| GET | `/admin/audit-logs` | 🛡 | 审计检索 | ⏳ |
| GET | `/admin/stats/overview` | 🛡 | 用户/会话/文件/登录趋势 | ⏳ |

首版行为：**不注册任何管理端路由**（含登录）。运营如需紧急操作，先用 goim 现有接口与数据库脚本。

### 8.9 会话 / 消息 / 关系 / 推送（❌ 委托 goim / ⏳ 业务扩展仅文档）

**C 档：委托 goim，业务后端不注册路由（❌ 不做）**

| 域 | 权威路径 | 说明 | 状态 |
|---|---|---|---|
| 会话列表/详情/增量 | `/api/v1/conversation/*` | SDK 直接调用，服务端已实现 | ❌ 不做 |
| 消息历史/拉取/已读 | `/api/v1/msg/pull_msg_by_seq`、`/msg/mark_*` | 客户端走 SDK，服务端按 seq 拉取 | ❌ 不做 |
| 消息搜索 | `/api/v1/msg/search_msg` | 管理端/客服检索 | ❌ 不做 |
| 好友/黑名单 | `/api/v1/relation/*` | 服务端已实现 | ❌ 不做 |
| 群组/群成员 | `/api/v1/group/*` | 服务端已实现 | ❌ 不做 |
| 聊天附件上传 | `/api/v1/third/*` | SDK FileUploader + RustFS | ❌ 不做 |

**B 档：业务扩展契约（⏳ 规划中，未注册路由）**

| 方法 | 路径 | 职责 | 扩展点 |
|---|---|---|---|
| GET | `/conversation-ext/{conversationId}` | 业务扩展属性（标签/备注/分组） | 转正时读 `conversation_ext` 表，主体仍以 goim 为准 |
| PUT | `/conversation-ext/{conversationId}` | 写扩展属性 | 与 SDK 会话设置做合并策略 |
| POST | `/conversation-ext/batch` | 批量读扩展属性 | 会话列表 BFF 聚合 |
| GET | `/message-archive/search` | 长期归档检索 | 转正时落归档索引（ES/对象存储），不从 Mongo 直读 |
| POST | `/message-archive/export` | 合规导出 | 异步任务 + 下载 URL |
| POST | `/push/preferences` | 推送偏好/免打扰 | 与 im-push 策略合并 |
| POST | `/push/test` | 测试推送 | 仅管理端可用 |
| GET | `/report/reasons` | 举报原因字典 | 内容治理 |
| POST | `/report` | 举报提交 | 联动 goim 拉黑/审核 |

这些 B 档路径统一使用**不与 goim 重叠的命名**（`conversation-ext`、`message-archive`、`push`、`report`），确保未来转正时客户端无需改路径。当前这些路由**一律未注册**。

### 8.10 内部（✅ 独立端口，不在 `/api/v1`）

| 方法 | 路径 | 端口 | 说明 | 状态 |
|---|---|---|---|---|
| GET | `/healthz` | 8081 | 存活 | ✅ |
| GET | `/readyz` | 8081 | 就绪（MySQL + Redis） | ✅ |
| GET | `/metrics` | 9091 | Prometheus | ✅ |

---

## 9. 关键流程

### 9.1 注册（✅ 已实现）与 IM 开通（⏳ 规划中）

当前实际流程：

```
client → POST /api/v1/account/register（X-Client-Platform 可选）
  handler:
    1. BindJSON + PlatformFrom + 注入 UserAgent/IP
  service:
    2. 校验登录名/密码/昵称/deviceId
    3. argon2id 计算密码哈希
    4. 事务：INSERT account_users → INSERT account_credentials → 创建会话（签发 access/refresh）
    5. 会话上限检查（踢出旧会话）
    6. 写 Redis sess:active:{sid}
  response: {code:0,data:{user,token,refreshToken,...,sessionId,platform}}
```

IM 开通属于目标蓝图（⏳，未实现）：

```
  事务内写 outbox：topic=im.user.provision
  outbox worker:
    → /user/account_check（幂等判断）
    → /user/user_register（admin token）
    → /user/update_user_info（昵称/头像）
    → 更新 account_users.im_status=ready
  client → POST /im/token → LoginWithToken
```

选择 outbox 而非同步调用：goim 抖动不应导致注册失败；但登录时若 `im_status != ready`，`/im/token` 会先同步重试一次，仍失败则返回 `9002`。

### 9.2 令牌刷新（✅ 已实现）

```
client → POST /api/v1/account/refresh {refreshToken}
  service:
    1. sha256(refreshToken) → 查 account_sessions.refresh_hash
    2. rotated_at 非空 → 判定重放 → 撤销整个 family（2005）
    3. 会话过期/已撤销 → 1002；账号冻结 → 2003
    4. 事务：MarkRotated(旧 sid) → 若已被并发轮换则按重放处理
              创建新会话（family_id 继承、rotated_from=旧 sid）
    5. 删除旧 sid 的 Redis 活跃标记；新 sid 写活跃标记，TTL=refresh_ttl
    6. 签发新 access（新 sid）
```

### 9.3 IM token 刷新（⏳ 规划中，SDK TokenProvider）

```
SDK token 即将过期 → 回调宿主 → POST /im/token（业务 access）
  → Redis 命中且剩余 >10min 直接返回
  → 否则 singleflight 向 goim 换新 → 返回
宿主 → SDK UpdateToken / 重新 LoginWithToken
```

### 9.4 头像上传（⏳ 规划中）

```
PUT /files/presign {purpose:avatar} → fileId + PUT URL
client ──直传──▶ RustFS
POST /files/{id}/complete → HeadObject 校验 size/mime/sha256 → status=ready
PUT /profile/me {avatarFileId} → 更新 account_users.avatar_key → outbox 同步 goim
GET /profile/me → 现签 GET URL 返回 avatarUrl
```

### 9.5 业务公告（M6，⏳ 规划中）

```
POST /admin/announcements
  → 写 notifications（站内）
  → outbox: topic=im.business.notification
  → worker: /msg/send_business_notification（admin token）
```

---

## 10. 非功能与安全基线

| 项 | 基线 | 当前状态 |
|---|---|---|
| 可用性 | 单实例可启动；多实例无状态；依赖失败快速失败并降级 | ✅ 单实例/无状态；降级仅有部分依赖错误码 |
| 时延 | 业务读 P99 < 100ms（不含外部依赖）；登录 P99 < 300ms | ⏳ 未压测 |
| 容量 | 首版按 1 万 DAU、1000 并发连接设计 | ⏳ 未验证 |
| 数据库 | 连接池 50/10；慢查询 200ms；每张表有主键与必要索引 | ✅ 配置与索引已落地 |
| 备份 | MySQL 每日全备 + binlog；RPO ≤ 5min，RTO ≤ 1h | ⏳ 部署侧 |
| 密钥 | 全部环境变量/K8s Secret；禁止进仓库、镜像、日志 | ✅ 生产 YAML 留空；dev 使用开发密钥 |
| 传输 | 全站 HTTPS/WSS；内部服务如跨网段走 TLS | ⏳ 部署侧 |
| 密码 | argon2id（m=64MB, t=3, p=4），禁止可逆存储 | ✅ |
| 越权 | 用户态接口按 `userID` 过滤；管理端独立 audience + 角色校验 | ✅ 账号接口按 userID 校验；管理端 ⏳ |
| 审计 | 管理端写操作 100% 落审计；保留 180 天 | ⏳ |
| 合规 | 注销冷却期 15 天；删除后对象与资料异步清理 | ⏳ |

---

## 11. 里程碑拆分

> 每个里程碑以“可演示 + 可回归”为完成标准；不按人天估算。

### M0 M1 必需的工程骨架（✅ 已完成）

> 决策：跳过“契约全量铺开”和 B 档占位路由，只保留 M1 运行所必需的基础设施。

- 目录、`go.mod`、分层骨架、配置加载与校验、zap、统一响应/错误码、request_id、recovery、CORS、metrics、health、graceful shutdown、Makefile、Dockerfile、compose、最小 CI。✅
- 迁移框架与 `000001_init`（只建 `account_users`、`account_credentials`、`account_sessions`）。✅
- OpenAPI 只写 M1 实际存在的 operation；B/C 档只在设计文档中保留，不生成路由。⏳ 当前未引入 OpenAPI 工具链。
- 验收：`make run` 起服务；`/readyz` 200；`/metrics` 可抓；`go test ./...` 绿；空库迁移成功；镜像可构建。✅（`make dev` 不存在，使用 `make run` 或 `make build`）

### M1 账号与鉴权闭环（✅ 已完成，首版核心）

- 注册/登录/刷新/登出/会话列表/踢会话/改密；argon2id；JWT + 会话表 + Redis 活跃标记；登录/注册限流；refresh 轮换与重放检测。✅
- **只做 A 档账号域**：不实现资料/设备/配置/文件，不注册任何 B/C 档路由，不做管理端。✅
- 验收：多端登录（Web + 任一桌面形态）互不影响；refresh 轮换与重放检测有单测；连续失败触发锁定；错误码与响应格式统一；账号接口有最小 OpenAPI 契约。✅

### M2 IM 对接闭环（⏳ 未开始）

- goim 客户端（admin token 缓存、singleflight、错误归一）；user provisioning（outbox）；`/im/token`；`/im/status`；平台 ID 约定。
- 同时改造 `goim-web-chat`：移除 `adminSecret`，改 `loginWithToken` + `TokenProvider`，`VITE_GOIM_PLATFORM_ID=5`。
- 验收：用业务账号在 Web 登录，能收发消息、拉历史；IM token 过期后自动刷新；前端产物中搜不到 admin secret。

### M3 资料 / 设备 / 客户端配置（⏳ 未开始）

- `/profile`（最小）、`/devices`、`/client-config`（ETag/版本）、资料同步 goim。
- 验收：改昵称/头像后另一客户端可见；配置版本递增、304 生效；设备列表可踢出。

### M4 文件（仅头像）+ 首版发布门禁（⏳ 未开始）

- RustFS 预签名、complete 校验、访问 URL、头像配额、孤儿清理。
- 验收：头像全链路；超大/非法类型被拒；未 complete 的对象 24h 清理。
- **首版门禁**：M0-M4 全部通过、Web 端完成 token 改造、压测与回滚演练通过后即可发布；B 档未实现不影响发布。当前 M2-M4 均未开始，尚不满足发布门禁。

### M5 上线加固（⏳ 未开始，与 M4 并行收尾）

- 压测（登录/会话/IM token）、Grafana 看板、告警、备份演练、发布/回滚手册、安全扫描。
- 验收：P99 达标；故障注入（goim/Redis 不可用）降级符合预期；回滚演练通过。

### M6 后续补齐（⏳ 按需，不动契约路径，不预设优先级）

B 档域按实际业务需要选择转正，每次只转正一个域；**当前不排优先级**：

- 通知/公告/站内信 → 厂商推送（JPush/FCM）。
- 管理端登录 → 用户/会话/文件/配置/审计/统计。
- 会话扩展属性（`conversation-ext`）。
- 消息归档/检索（`message-archive`）。
- 推送偏好、举报与内容治理。
- 账号注销、OAuth/短信登录、多租户。

转正方式：补 service/repository 与 handler 实现，再补对应 OpenAPI operation；路径与 DTO 不变。

依赖关系：M0 → M1 → M2 为硬串行；M3/M4 可并行；M5 与 M4 收尾并行；M6 在首版发布后按实际需求安排，不预设优先级。

---

## 12. 风险与待决策

| # | 风险/决策 | 影响 | 建议 |
|---|---|---|---|
| 1 | 前端 adminSecret 已泄露（立项取证，待核实） | 任意 userID 可被签发 IM token | 立即轮换；上线前删除所有 `VITE_*SECRET`；CI 加 secret 扫描 |
| 2 | goim 用户投影与业务账号双存储（同一 userID） | 资料不一致 | 业务为唯一写入口；goim 只读投影；同步失败可观测、可重放；不做 ID 映射 |
| 3 | goim 无 refresh token | 长连接掉线后需重新登录 | 业务侧提前 10 分钟换新；SDK TokenProvider 兜底 |
| 4 | IM token 频繁换发触发多端踢出 | 用户被意外下线 | 业务按 `(userID, platformID)` 缓存；禁每次请求都换 |
| 5 | 平台 ID 传错 | 会话/推送串平台 | 服务端按 `X-Client-Platform` 固化；Web=5（已实现默认值） |
| 6 | 业务 API 与 SDK 响应语义不同 | 前端拦截器误判 | 业务统一 HTTP 200 + body `code`；SDK 通道同样 200；封装独立 fetch 客户端 |
| 7 | 多租户 | goim 侧仍是设计稿 | 首版单租户；配置表带 `app_id`，用户表不引入 tenant |
| 8 | 桌面端形态未定 | 不影响业务后端方案 | 方案保持形态无关：只依赖 HTTP/WS 与平台 ID；安全存储、系统通知、安装包由客户端决定 |
| 9 | 管理后台前端范围 | 影响后续排期 | 首版不交付；后续按需单独排期 |
| 10 | 消息检索需求边界 | 可能导致复制消息数据 | 首版用 goim `/msg/search_msg`；确有全文检索需求再建归档索引 |

### 12.1 已确认决策

1. **业务 userID 直接复用为 IM userID**，不建映射表、不设 `im_user_id` 字段、不生成第二套 ID。
2. **桌面端形态待定**，本方案不绑定 cgo 原生壳或 Electron/Tauri；只约束 HTTP/WS 协议、platformID 与安全存储能力由客户端自证。
3. **首版只做 A 档**；B 档只在文档保留契约，不实现、不注册路由、不排转正优先级。
4. **账号主键使用数据库自增 BIGINT**（对前端与 IM 下发时转字符串），与蓝图早期设想的 UUIDv7 主键不同；新表沿用此约定，避免同库混用两套 ID 规则。

### 12.2 开放项（不阻塞当前开发）

- 桌面端具体技术形态：进入 M2/M3 联调前由客户端团队决定，业务后端无需改动。
- B 档转正顺序：首版发布后按实际需求决定，本文不预设。
- 管理后台 UI 范围：接口与页面解耦，后续单独评估。

---

## 13. 开发落地清单（Definition of Done）

- [x] ✅ `goim-business` 骨架可启动，配置缺失 fail-fast。
- [x] ✅ 迁移可在空库一键建表，生产不使用 AutoMigrate。
- [x] ✅ M1：注册/登录/刷新/登出/会话列表/踢会话/改密有单测；refresh 轮换与重放检测通过。
- [x] ✅ M1：多端登录互不影响，超限踢最旧，登出/踢出即时生效。
- [x] ✅ M1：`userID` 即 IM userID，注册流程原样写入，无映射层（IM 下发待 M2）。
- [x] ✅ M1：统一错误码与响应格式覆盖所有账号接口；敏感字段不落日志。
- [x] ✅ M1：账号接口在最小 OpenAPI/代码中可查；`api/openapi.yaml` 已覆盖 7 个账号接口。
- [x] ✅ `/healthz`、`/readyz`、`/metrics` 已独立端口暴露。
- [x] ✅ 日志字段规范落地，request_id 贯通；`X-Request-ID` 回写。
- [x] ✅ 登录/注册限流可配置，限流实现已落地（防刷用例已补：`internal/middlewares/ratelimit_test.go`）。
- [ ] ⏳ M2+：`/im/token` 有缓存、singleflight、限频、依赖失败降级。
- [ ] ⏳ M2+：Web 聊天室移除 adminSecret，平台 ID 改为 5，登录走业务 token。
- [ ] ⏳ M2+：outbox worker 可重试、可观测、积压有告警。
- [x] ✅ 镜像可构建；迁移入口已提供（`bin/migrate up|down|version`）。
- [ ] ⏳ 迁移 Job 与发布顺序写入 runbook。
- [ ] ⏳ 后续域（通知/管理端/会话扩展/归档/推送偏好/举报）只保留契约，不注册路由。
- [ ] ⏳ B 档转正的每个域补齐越权测试与审计（管理端写操作 100% 落审计）。

---

## 14. 实际执行顺序与当前状态

已确认跳过 M0 的“契约全量铺开”和 B 档占位路由，只落地 M1 运行必需的骨架。实际执行记录：

1. **骨架（✅ 已完成）**：模块与分层、配置加载/校验、zap、统一响应与错误码、request_id、recovery、CORS、metrics、health、优雅启停、迁移框架、compose、最小 CI。
2. **账号域纵切（✅ 已完成）**：`account_users` / `account_credentials` / `account_sessions` 迁移 → 仓储 → 注册 → 登录 → 签发 access/refresh → 刷新轮换 → 登出/踢出 → 改密。
3. **安全与多端（✅ 已完成）**：argon2id、refresh 哈希与重放检测、Redis 活跃会话、每用户/每平台会话上限、登录/注册限流、登录失败锁定。
4. **验收（✅ 已完成）**：`go test ./...` 全绿；空库迁移成功；curl 跑通注册→登录→刷新→登出；多端会话互不影响；日志无敏感字段。

未包含（⏳ 后续 M2+）：资料/设备/配置/文件域、IM token 兑换与账号开通、B/C 档路由、管理端、OpenAPI 全量契约（当前仅账号域最小契约）、outbox、RustFS/推送。

代码不自动提交，提交时机与粒度由用户决定。
