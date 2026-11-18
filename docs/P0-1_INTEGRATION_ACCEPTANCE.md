# P0-1 联调验收清单（M2 / M3 / M4）

> 对应 [DEVELOPMENT.md](DEVELOPMENT.md) 的 M2/M3/M4 与「首版门禁」。本清单只在联调环境执行，全部勾选后才视为 P0-1 完成。
> 约定：所有业务接口 HTTP 状态恒为 200，结果以响应体 `{code,msg,data}` 的 `code` 为准（`0` 成功）。

## 0. 环境与启动（前置，全部通过才能开始业务用例）

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| A1 | `make compose-up` | MySQL 3306、Redis 6379 健康 | ☐ |
| A2 | `make migrate-up` | 迁移到 `000005_file_objects`，`file_objects` 存在且含 `idx_status_expires` | ☐ |
| A3 | 配置 `im.base_url` / `im.admin_user_id` / `im.admin_secret` | `/im/token` 可成功兑换；admin token 无 1002 报错 | ☐ |
| A4 | RustFS 可达；bucket 已建；`use_path_style: true`；桶策略允许 `public/*` 匿名 GetObject，其余前缀拒绝 | 匿名 GET `public/` 对象 200，`attachment/` 对象 403 `AccessDenied` | ☐ |
| A5 | `storage.public_endpoint` 配成客户端可达地址（公网域名/Nginx） | `avatarUrl` 可直接被浏览器打开 | ☐ |
| A6 | 启动 API（`make run`）与 cron（`make run-cron`）；联调期把 `storage.cleanup_interval` 调成 `1m` | cron 日志出现 `file_object_cleanup` 注册；API `/healthz`、`/readyz` 均 200 | ☐ |
| A7 | 访问 `:19191/metrics` | 可抓到 `goim_client_requests_total`、`request_duration` 等指标 | ☐ |
| A8 | 敏感配置只走环境变量 | `etc/base.yaml` 中 `jwt_secret`、`admin_secret`、`storage.secret_key` 均为空 | ☐ |

通用请求头（下文的登录态接口都带）：

```text
Authorization: Bearer <access_token>
X-Device-Id: web-device-001        # 注册/登录必填，最长 64
X-Client-Platform: 5               # 缺省按 Web；非法值应被拒
X-Client-Version: 1.0.0            # 可选，最长 32
```

## 1. 账号 / 会话 / 令牌

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| B1 | `POST /api/v1/account/register`（含 `X-Device-Id`） | `code=0`，返回 `token`/`refreshToken`/`user` | ☐ |
| B2 | 用同一 `loginName` 再注册 | `code=2001` 账号已存在 | ☐ |
| B3 | 注册/登录不带 `X-Device-Id` | `code=1001` | ☐ |
| B4 | 错误密码登录 | `code=2002` | ☐ |
| B5 | `POST /account/refresh` 成功后再用旧 refresh 重放 | 第一次 `code=0` 且 refresh 轮换；重放 `code=2005` | ☐ |
| B6 | 把 `auth.access_ttl` 临时调成 `1m`，等待过期后访问受保护接口 | `code=2004` | ☐ |
| B7 | `POST /account/logout` 后再用原 access 访问 `/profile/me` | `code=1002` | ☐ |
| B8 | 同一平台连续登录到第 4 个设备（`max_sessions_per_platform=3`） | `code=2006`，最旧设备被踢 | ☐ |
| B9 | `GET /account/sessions` → `POST /account/sessions/revoke` | 被撤销会话的 access 立即 `code=1002` | ☐ |
| B10 | `PUT /account/password` 后旧会话刷新 | 旧 access `code=1002`、旧 refresh `code=2005` | ☐ |

## 2. IM 开通与凭证

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| C1 | B1 注册成功后 `GET /api/v1/im/status` | `code=0`，`status=1`（Ready），`syncedAt` 非空 | ☐ |
| C2 | `POST /api/v1/im/token` 连续调用两次 | 均 `code=0`，返回 `token`/`expireTimeSeconds`/`apiBaseUrl`/`wsUrl`；第二次命中 Redis 缓存 | ☐ |
| C3 | 停 goim 后注册新用户，再恢复 goim，等待 cron 一轮 | 注册仍成功且 `im_status=2`；恢复后自动补偿为 `1`，无需人工 `/im/retry` | ☐ |
| C4 | 停 goim 期间执行 `POST /api/v1/im/retry` | 返回 `code=9002`；goim 恢复后手动 retry 成功置 `1` | ☐ |
| C5 | 清空 `im.base_url` 后调用 `/im/token` | `code=9002`，注册流程正常（不阻塞） | ☐ |
| C6 | **1102 场景**：在 goim 侧预建与业务 `userID` 相同的账号（昵称为空、faceURL 为空），业务侧已有头像后触发 `/im/retry` | 开通视为成功，IM 侧昵称/faceURL 被补齐为业务资料（公开 `avatarUrl`） | ☐ |
| C7 | 缓存 token 剩余有效期不足 10 分钟时再次 `/im/token` | 返回新 token（不返回即将过期的缓存值） | ☐ |
| C8 | Web 客户端：业务账号登录 → `/im/token` → `loginWithToken` → 收发消息 → 拉历史 | 全链路可用；9002 时自动 `/im/retry` 后重试成功 | ☐ |

## 3. 用户资料与头像

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| D1 | `PUT /api/v1/profile/me` 改昵称（带首尾空格）与 bio | `code=0`，返回规范化后的值；`GET /profile/me` 一致 | ☐ |
| D2 | 完成头像上传后 `PUT /profile/me {avatarKey: objectKey}` | 返回 `avatarKey` 与 `avatarUrl`；`avatarUrl = public_endpoint/bucket/key` | ☐ |
| D3 | IM 侧查询该用户 | faceURL 与 `avatarUrl` 相同（D2 之后且 IM Ready） | ☐ |
| D4 | 传入非 `public/` 前缀的 key（如 `attachment/...`） | `avatarKey` 保存成功，但 `avatarUrl` 返回空串 | ☐ |
| D5 | 另一账号 `GET /profile/users/{userId}` 与 `POST /profile/batch` | 能看到新昵称与头像；batch 超过 100 个 ID 返回 `code=1001` | ☐ |
| D6 | 未设置头像的用户 | `avatarUrl` 为空字符串，前端展示默认头像 | ☐ |
| D7 | 停 goim 后改昵称/头像，恢复后等 cron | `profile_sync` 重试至 `done`，IM 侧资料与业务一致 | ☐ |

## 4. 设备

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| E1 | 注册/登录 | `account_devices` 自动新增/更新该 `deviceId` | ☐ |
| E2 | `GET /api/v1/devices` | 返回设备列表（平台、版本、最后活跃），不含推送 token | ☐ |
| E3 | `DELETE /api/v1/devices/{deviceId}` | 该设备全部会话撤销，原 access 访问受保护接口 `code=1002` | ☐ |
| E4 | 删除不存在的 `deviceId` | `code=1004` | ☐ |
| E5 | Web(5) 与 Windows(3) 同时在线 | 两设备并存，互不踢出 | ☐ |

## 5. 客户端配置

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| F1 | `GET /api/v1/client-config` | `code=0`，含 `im.apiBaseUrl`/`im.wsUrl`、上传限制、功能开关 | ☐ |
| F2 | 记录 `ETag` 后带 `If-None-Match` 再次请求 | HTTP 304，无 body | ☐ |
| F3 | `GET /api/v1/client-config/schema` | 返回字段元数据（类型、默认值、作用域） | ☐ |
| F4 | 清空 `client_configs` 表后请求 | 回退内置默认值，`code=0` 不报错 | ☐ |

## 6. 文件域（单文件 + 分片）

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| G1 | `POST /api/v1/files/presign`，`purpose=avatar`、`image/png`、≤5MB | `code=0`，`uploadUrl` 可直接 PUT，`headers` 含 Content-Type，`expiresAt`≈15 分钟 | ☐ |
| G2 | 按 G1 直传后再 `POST /files/{fileId}/complete` | `code=0`，`status=1`，回填真实大小与 MIME | ☐ |
| G3 | 未上传对象直接 complete | `code=1001`（对象不存在或未完成） | ☐ |
| G4 | 直传时把 Content-Type 改成 `text/plain` 再 complete | `code=1001` 类型不匹配 | ☐ |
| G5 | avatar >5MB；attachment ≤8MB 走单文件接口、>8MB 走 `POST /files/multipart/presign` | 前者 `code=1001` 大小超限；后者提示 `ErrUseMultipart`，分片入口成功 | ☐ |
| G6 | `GET /api/v1/files/{fileId}/url` | `code=0`，URL 可直接 GET，`expiresAt`≈15 分钟 | ☐ |
| G7 | `DELETE /api/v1/files/{fileId}` | `code=0`，记录置 deleted；删除后再 `GET .../url` 返回 `code=1004` | ☐ |
| G8 | attachment 40MB 走 `POST /files/multipart/presign` | `partCount=5`、`partSize=8MB`、每片独立签名 URL | ☐ |
| G9 | 逐片 PUT 后 `POST /files/{fileId}/multipart/complete`（带 ETag 列表） | `code=0`，`status=1`；分片数/序号不符返回 `code=1001` | ☐ |
| G10 | 未完成时 `POST /files/{fileId}/multipart/parts` 刷新部分分片 | 已传分片保留，新 URL 可用 | ☐ |
| G11 | `POST /files/{fileId}/multipart/abort` 后再次 abort | 第一次 `code=0`（分片释放、记录 deleted）；第二次幂等 `code=0` | ☐ |
| G12 | 用他人 `fileId` 调用 complete/url；未 ready 文件取 url | 前者 `code=1004`，后者 `code=1001`（尚未完成） | ☐ |
| G13 | 匿名 `curl` 头像对象与附件对象 | `public/avatar/...` 200；`attachment/...` 403 `AccessDenied` | ☐ |

## 7. cron 清理（`file_object_cleanup`）

> 联调期把 `storage.cleanup_interval` 设为 `1m`，或直接改库造数据；调度器启动时会先执行一轮。

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| H1 | 启动 cron（IM 与 storage 至少一项启用） | 日志含 `name=file_object_cleanup`；只配 IM 时不注册该任务 | ☐ |
| H2 | 造一条 `status=0` 且 `expires_at < now` 的单文件记录 | 一轮后 `status=2`，日志 `scanned/cleared/failed` 计数正确 | ☐ |
| H3 | H2 的对象已实际 PUT | 对象被删除（HeadObject 404） | ☐ |
| H4 | 造一条带 `upload_id` 的过期分片记录 | 分片被 abort（ListMultipartUploads 不再出现该 uploadId） | ☐ |
| H5 | 过期记录在清理前先 complete 成功 | 清理跳过，`status` 保持 1，对象不被删除 | ☐ |
| H6 | 无过期记录时观察一轮 | 无 S3 调用、无清理日志（`scanned=0`） | ☐ |

## 8. 可观测与健康检查

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| I1 | 任一请求检查响应头与日志 | 有 `request_id`，访问日志为 JSON 且含 `code`、耗时 | ☐ |
| I2 | `/metrics` | 含 IM 客户端请求指标、HTTP 指标；清理任务有日志可观测 | ☐ |
| I3 | `/healthz`、`/readyz` | 分别 200；MySQL/Redis 断开时 `readyz` 转 503 | ☐ |
| I4 | 构造 5xx/panic（临时路由或注入） | Recovery 生效，进程不退出且日志有堆栈 | ☐ |

## 9. 异常与降级

| ID | 步骤 | 期望 | 结果 |
|---|---|---|---|
| J1 | 停 Redis 后登录/鉴权 | 依赖不可用返回 `code=9002`（按设计 fail-closed），进程不崩溃 | ☐ |
| J2 | 停 goim 后注册与 `/im/token` | 注册成功、`im_status=2`，token 兑换 `code=9002`；恢复后自动补偿 | ☐ |
| J3 | 停 RustFS 后调用文件接口 | `code=9002`；`avatarUrl` 直链不可访问（预期，恢复后自动可用） | ☐ |
| J4 | 高频调用登录/`/im/token` | 触发 `code=1029` 限流 | ☐ |
| J5 | `X-Client-Platform: abc` | `code=1001` | ☐ |
| J6 | 请求体超过 `server.max_body_bytes`（8MB） | 请求被拒（`code=1001`） | ☐ |

## 10. 签收

| 字段 | 填写 |
|---|---|
| 执行人 |  |
| 执行日期 |  |
| 环境（goim / RustFS 地址） |  |
| 失败用例编号与证据（日志/截图/request_id） |  |
| 结论（全部通过 / 阻塞项） |  |
