# 前端对接：设备标识、请求头与文件上传

本文说明前端（Web / 桌面 / 移动端）如何生成与传递设备标识，以及调用账号接口时的请求头约定；§8 为分片上传与签名刷新，§9 为头像、聊天资源上传与访问控制。接口字段以 `api/openapi.yaml` 为准。

## 1. 请求头约定

| 请求头 | 必填 | 适用接口 | 说明 |
|---|---|---|---|
| `X-Device-Id` | 是 | `POST /account/register`、`POST /account/login` | 客户端设备标识，1-64 个字符；缺失或超长返回 `1001` |
| `X-Client-Platform` | 否 | 注册、登录、`GET /client-config` | 平台标识，缺省 `web`；支持 `ios/android/windows/macos/web/harmony` 或数字 `1/2/3/4/5/11`，非法返回 `1001` |
| `X-Client-Version` | 否 | 注册、登录 | 客户端版本号，最长 32 个字符；超长返回 `1001`，缺失按空值处理 |
| `Authorization` | 是 | 所有 🔐 接口 | `Bearer <access token>`；过期返回 `2004`，其余鉴权失败返回 `1002` |

`User-Agent` 由浏览器或 HTTP 客户端自动携带，不需要额外设置，也不要用它替代 `X-Device-Id`。

已认证接口（`/account/logout`、`/devices`、`/profile` 等）**不需要**再带 `X-Device-Id` 或 `X-Client-Platform`：服务端以 access token 中签发的会话属性为准，重复传递不会被采纳。

注册与登录请求体不再包含 `deviceId` / `appVersion`，这两个值只从请求头读取。

## 2. X-Device-Id 是什么

- 是**客户端稳定设备标识**，不是用户 ID、不是会话 ID，也不是浏览器指纹（不要用 UA/IP/Canvas 等推导）。
- 服务端按 `(userId, deviceId)` 维护 `account_devices`：注册/登录成功会自动登记或刷新该设备，前端不需要再调用额外的注册接口。
- 用途：设备列表展示、按设备踢出（`DELETE /devices/{deviceId}` 会同时撤销该设备的全部会话）。
- 它**不参与鉴权签名**，仅用于会话归属与设备管理。

## 3. 生成与持久化

1. 首次启动时生成一个随机标识（优先 UUID v7 / v4），之后从本地存储读取，不要每次刷新重新生成。
2. 建议字符集 `[A-Za-z0-9_-]`，长度不超过 64（标准 UUID 为 36 位，直接可用）。
3. Web 端存 `localStorage`，同一浏览器的多个标签页应共享同一个值。
4. 不要把 token、手机号、邮箱等敏感信息拼进 `deviceId`。
5. 用户清除站点数据、换浏览器或换设备后会自然产生新的 `deviceId`，属于预期行为；旧设备记录可由用户在设备列表中移除。

示例（Web）：

```js
const DEVICE_ID_KEY = 'goim_device_id';

export function getDeviceId() {
  const existing = localStorage.getItem(DEVICE_ID_KEY);
  if (existing) return existing;

  const id = (crypto.randomUUID?.() ??
    `dev-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`);
  localStorage.setItem(DEVICE_ID_KEY, id);
  return id;
}
```

React Native / 桌面端可将同一个值存入各自的持久化存储（AsyncStorage / 本地配置文件），规则一致。

## 4. 注册 / 登录示例

```js
// 业务接口前缀为 /biz-api/v1；goim im-api 的 /api/v1 由 IM SDK 使用，二者不要混用。
const API_BASE = `${BIZ_ORIGIN}/biz-api/v1`;

const headers = {
  'Content-Type': 'application/json',
  'X-Device-Id': getDeviceId(),
  'X-Client-Platform': 'web',
  'X-Client-Version': APP_VERSION, // 可选
};

// 登录：请求体不再携带 deviceId / appVersion
await fetch(`${API_BASE}/account/login`, {
  method: 'POST',
  headers,
  body: JSON.stringify({ loginName, password }),
});

// 注册：额外带 nickname
await fetch(`${API_BASE}/account/register`, {
  method: 'POST',
  headers,
  body: JSON.stringify({ loginName, password, nickname }),
});
```

建议把公共头封装到统一请求函数或 axios 拦截器，保证注册与登录都带上 `X-Device-Id`。

## 5. 设备列表与移除

- `GET /devices`：返回 `deviceId / platform / appVersion / lastActiveAt`，按最近活跃时间倒序，不含推送 token。
- `DELETE /devices/{deviceId}`：删除该设备记录并撤销该设备的全部活跃会话；服务端同时调用 goim 踢出该设备的 WS 连接（收到 `2002`）并按平台失效 IM token。

```js
await fetch(`${API_BASE}/devices/${encodeURIComponent(deviceId)}`, {
  method: 'DELETE',
  headers: { Authorization: `Bearer ${accessToken}` },
});
```

移除当前设备后，本设备的 access token 立即失效（后续请求返回 `1002`），refresh token 也无法再次续期（返回 `1002`，即会话已失效）；IM 侧会收到 `2002` 踢下线通知，旧的 IM token 同时失效。前端应清除本地令牌、断开 SDK 并引导重新登录，不能继续复用内存中的 IM token 发消息。

## 5.1 注销账号

- `DELETE /account`（Bearer 鉴权）：立即下线并进入注销中，冷却期（默认 15 天）结束后服务端物理清除资料、凭证、会话与设备，并回收文件对象。
- 响应 `data.purgeAt` 是物理清除时间；重复调用幂等，返回同一时间。
- 注销后无法登录、无法调用业务接口，公开资料也不再对外可见；前端应清除本地令牌并退出。

```js
const res = await fetch(`${API_BASE}/account`, {
  method: 'DELETE',
  headers: { Authorization: `Bearer ${accessToken}` },
});
const { data } = await res.json();
// data.purgeAt: 冷却期结束后的物理清除时间
```

## 6. 错误码与排障

| 场景 | code | 排查 |
|---|---|---|
| 缺少或非法 `X-Device-Id` | `1001` | 确认请求头存在、已 trim、非空且不超过 64 字符 |
| `X-Device-Id` 超过 64 字符 | `1001` | 截断或改用 UUID；不要放整段 JSON |
| `X-Client-Version` 超过 32 字符 | `1001` | 只传版本号（如 `1.0.0`），不要带构建信息长串 |
| 平台非法 | `1001` | 使用约定枚举或数字编码 |
| 未登录 / 会话已被撤销 | `1002` | 清理本地令牌，重新登录 |
| access token 过期 | `2004` | 用 refresh token 续期，或重新登录 |
| refresh 令牌被重放 | `2005` | 重新登录（服务端会撤销整个令牌家族） |

跨域部署时，网关或服务端的 CORS 允许头需要包含：`Content-Type, Authorization, X-Client-Platform, X-Client-Version, X-Device-Id, X-Request-ID, Idempotency-Key`；同时必须暴露 `ETag`（`Access-Control-Expose-Headers: ETag`），否则 `/client-config` 的 304 缓存逻辑在浏览器中读不到 ETag。

## 7. 版本迁移注意

- 旧客户端如果把 `deviceId` 放在请求体，新服务端会忽略该字段并因缺少 `X-Device-Id` 返回 `1001`；升级前端时必须同时改为请求头传递。
- 注册/登录的响应结构未变化，仍为 `{ code, msg, data: { user, token, refreshToken, refreshExpiresIn, sessionId, platform } }`。
- 服务端只在注册/登录时读取设备头；其他接口即使携带也不会覆盖 token 中的 `deviceId/platform`。

## 8. 大文件分片上传与重试

头像、聊天图片、聊天文件与不超过 8MB 的附件统一走 `POST /files/presign` 单文件 PUT（`purpose` 分别为 `avatar`、`chat_image`、`chat_file`、`attachment`；`chat_image`/`chat_file` 必须传 `conversationId`）；`chat_file` 与 `attachment` 超过 8MB 时使用分片上传，最大 200MB，固定 8MB/片、最多 100 片。

### 8.1 整体流程

1. `POST /files/multipart/presign` 申请上传，响应包含 `fileId / objectKey / uploadId / partSize / partCount / parts[]`。
2. 按 `parts[].url` 逐片 `PUT`（建议 3–4 并发），每片的 `Content-Type` 必须与申请一致；PUT 成功后保存响应头 `ETag` 原值（通常带双引号）。
3. 全部分片成功后 `POST /files/{fileId}/multipart/complete`，请求体上报 `parts: [{ partNumber, etag }]`，顺序必须从 1 连续递增。
4. 用户取消时调用 `POST /files/{fileId}/multipart/abort`；`DELETE /files/{fileId}` 也会终止未完成的分片上传。

`uploadId` 由服务端随 `fileId` 落库，complete/abort/刷新都只需 `fileId`，客户端无需回传。

### 8.2 签名有效期

- 分片 PUT URL：默认 **2 小时**（`storage.multipart_presign_expire`）。
- 单文件 PUT 与 `GET /files/{fileId}/url`：默认 **15 分钟**（`storage.presign_expire`）。
- pending 记录保留 24 小时；超时未完成的上传会被服务端清理，客户端需要重新申请。

### 8.3 分片失败与续期

分片 PUT 返回 **403**（`Request has expired` / `AccessDenied`）通常是签名过期，不是文件内容错误。不要重新调用 `multipart/presign`（会创建新记录、丢失已传分片），改为刷新失败分片的签名：

```js
// 只刷新失败的分片；partNumbers 省略表示刷新全部
await fetch(`${API_BASE}/files/${fileId}/multipart/parts`, {
  method: 'POST',
  headers: { Authorization: `Bearer ${accessToken}`, 'Content-Type': 'application/json' },
  body: JSON.stringify({ partNumbers: [2, 5] }),
});
```

响应结构与 `multipart/presign` 相同，复用原 `uploadId / objectKey / partSize / partCount`，已上传分片保持不变。刷新后只重试对应分片。

重试建议：

- 单分片最多重试 3 次，指数退避（约 1s / 2s / 4s）；仅对网络错误、5xx 与上面提到的 403 重试。
- 4xx（除签名过期）如 400/403 内容校验失败不要盲目重试，先检查 `Content-Type` 与分片大小。
- 分片签名还有效时不要频繁刷新；可在本地记录 `parts[].expiresAt`，临近过期或收到 403 时再刷新。
- 重试仍失败时调用 `abort`，再重新 `multipart/presign`；也可直接把失败信息提示给用户。

### 8.4 complete 与状态冲突

- `complete` 是幂等的：重复调用或网络超时后重试，已 ready 的文件会直接返回成功。
- 刷新接口对已 ready 的文件返回 `1005`，表示无需刷新；前端应改为调用 `complete` 或直接按成功处理。
- 若刷新/complete 返回 `1004` 或提示上传任务不存在，说明 pending 已超时被清理，只能重新申请分片上传。
- pending 记录超过 24 小时会失效，长时间离线续传前建议先刷新；仍失败则重新申请。

### 8.5 错误码速查

| 场景 | code | 处理 |
|---|---|---|
| 分片号越界/非递增、超出 8MB–200MB、MIME 不支持 | `1001` | 修正请求参数，不要重试 |
| 未登录或登录态失效 | `1002` | 清理令牌并重新登录 |
| 文件不存在、他人文件、pending 已清理 | `1004` | 重新申请上传 |
| 已 ready 还调用刷新，或状态已变化 | `1005` | 按成功处理或先查询状态 |
| 对象存储不可用 | `9002` | 退避后重试，持续失败则提示稍后再试 |

## 9. 头像、聊天资源与访问控制

头像与聊天资源（图片、语音、视频、文件）都走**单文件/分片上传**。头像公开、可长期缓存；聊天资源绑定会话，**仅会话成员可见**，每次渲染前换取短时 URL。头像见 9.1–9.3，聊天图片与聊天文件见 9.4。

### 9.1 上传流程

1. `POST /files/presign`，请求体：
   ```json
   { "purpose": "avatar", "fileName": "me.png", "mime": "image/png", "size": 120000 }
   ```
   响应中的 `objectKey` 就是要写回资料的 key。
2. 按 `uploadUrl` + `headers` 直接 `PUT` 文件到对象存储。
3. `POST /files/{fileId}/complete`，服务端校验后置为 ready。
4. `PUT /profile/me`，请求体传 `{ "avatarKey": "<objectKey>" }`。

注意：`PUT /profile/me` 传的是 `objectKey`（形如 `public/avatar/123/uuid.png`），不是文件名，也不是临时预签名 URL。

### 9.2 其他人如何拿到 URL

资料响应同时返回两个字段：

- `avatarKey`：对象 key，仅用于更新资料时回传，不要直接渲染。
- `avatarUrl`：**稳定、不过期的公开 URL**，前端直接作为 `<img src>` 使用。

获取他人头像时，从这些接口读取 `avatarUrl`：

- `GET /profile/users/{userId}`：单个用户公开资料。
- `POST /profile/batch`：批量取公开资料（最多 100 个）。

```json
// POST /profile/batch response data 示例
[
  { "userId": "123", "nickname": "Alice",
    "avatarKey": "public/avatar/123/0198c2f0....png",
    "avatarUrl": "https://files.example.com/goim-business/public/avatar/123/0198c2f0....png" }
]
```

`avatarUrl` 为空字符串表示未设置头像，前端展示默认头像即可。

### 9.3 注意事项

- `avatarUrl` 由服务端用 `storage.public_endpoint + bucket + avatarKey` 生成，任何客户端可直接 GET；**不要**再对它调用 `/files/{fileId}/url`。
- 该方案要求部署侧对 `public/` 命名空间开放匿名 GET（bucket policy 或 Nginx 规则）；若直接访问返回 403，说明基础设施未配置，不是签名过期。
- 他人可见性依赖 `avatarUrl`，因此 `PUT /profile/me` 后应重新拉取资料；不要缓存旧的 `avatarKey` 自行拼接域名。
- goim 侧同步的 faceURL 也使用同一个公开 URL，聊天界面无需再换取签名。
- 更新头像时先完成新文件上传与 complete，再更新资料；旧对象由清理任务按记录状态回收。
- 附件（`purpose=attachment`）仍是私有资源，只对本人返回 15 分钟预签名 URL，二者不要混用。

### 9.4 聊天图片与聊天文件（chat_image / chat_file）

聊天图片与聊天文件是**会话私有资源**：上传时必须传 `conversationId`，只有发送双方的会话成员或群成员才能换取访问 URL。

**会话 ID（conversationId）**

- 单聊：`si_<id1>_<id2>`。必须原样使用 goim 生成的会话 ID，服务端只校验格式、不重排；上传者必须是两端之一。
- 群聊：`g_<groupID>` 或 `sg_<groupID>`，上传者必须是当前群成员。

**聊天图片（chat_image）**

1. `POST /files/presign`，请求体 `{ "purpose": "chat_image", "conversationId": "si_123_456", "fileName": "pic.jpg", "mime": "image/jpeg", "size": 300000 }`。
   - 仅支持 `image/jpeg`、`image/png`、`image/webp`、`image/gif`，单文件 ≤ 8MB，不支持分片。
2. 按 `uploadUrl` + `headers` 直接 `PUT` 文件到对象存储。
3. `POST /files/{fileId}/complete` 置为 ready；响应 `data.url` 为**空字符串**，请不要写入消息体。
4. IM 图片消息只存 `fileId`（或业务侧返回的文件 ID），渲染时调用 `GET /files/{fileId}/url` 换取 15 分钟预签名 URL。

**聊天文件（chat_file：语音、视频、文档等）**

1. ≤ 8MB：与图片相同，`POST /files/presign` 且 `purpose=chat_file`，完成后 `data.url` 为空。
2. > 8MB：走 `POST /files/multipart/presign`（`purpose=chat_file` + `conversationId`），逐片 PUT 后调用 `/files/{fileId}/multipart/complete`，最大 200MB。
3. 支持 JPEG/PNG/WebP/GIF、MP3/WAV/AMR/M4A/AAC、PDF/ZIP/TXT、MP4、DOCX/XLSX/PPTX。

**获取访问 URL**

```js
// 每次渲染或点击下载前换取；不要缓存 URL，也不要写回消息体
const res = await fetch(`${API_BASE}/files/${fileId}/url`, {
  headers: { Authorization: `Bearer ${accessToken}` },
});
const { data } = await res.json(); // data.url 为 15 分钟预签名地址
```

要点：

- 上传缺失 `conversationId` 返回 **1001**；单聊上传者不在两端、群聊上传者不在群同样返回 1001。
- 群成员校验依赖 IM；IM 不可用时上传或换 URL 返回 **9002**，前端应提示稍后重试。
- 非会话成员、他人附件或已删除文件统一返回 **1004**，不要据此推断文件是否存在。
- 聊天资源 `GET /files/{fileId}/url` 每次现签，URL 最长存活 15 分钟；不要用 `storage.public_endpoint` 自行拼接。
- 与 `attachment` 的区别：`attachment` 仅本人可见；聊天资源对会话成员可见，两者都不走 `public/` 匿名读。
- 历史（升级前）的聊天消息可能只有公开 URL、没有 `fileId`，前端按兼容逻辑继续直接渲染；新消息必须改为 `fileId` + 现签。
