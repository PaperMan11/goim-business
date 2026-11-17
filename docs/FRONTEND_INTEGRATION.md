# 前端对接：设备标识与请求头

本文说明前端（Web / 桌面 / 移动端）如何生成与传递设备标识，以及调用账号接口时的请求头约定。接口字段以 `api/openapi.yaml` 为准。

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
- `DELETE /devices/{deviceId}`：删除该设备记录并撤销该设备的全部活跃会话。

```js
await fetch(`${API_BASE}/devices/${encodeURIComponent(deviceId)}`, {
  method: 'DELETE',
  headers: { Authorization: `Bearer ${accessToken}` },
});
```

移除当前设备后，本设备的 access token 立即失效（后续请求返回 `1002`），refresh token 也无法再次续期（返回 `1002`，即会话已失效）。前端应清除本地令牌并引导重新登录。

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

跨域部署时，网关或服务端的 CORS 允许头需要包含：`Content-Type, Authorization, X-Client-Platform, X-Client-Version, X-Device-Id, X-Request-ID, Idempotency-Key`。

## 7. 版本迁移注意

- 旧客户端如果把 `deviceId` 放在请求体，新服务端会忽略该字段并因缺少 `X-Device-Id` 返回 `1001`；升级前端时必须同时改为请求头传递。
- 注册/登录的响应结构未变化，仍为 `{ code, msg, data: { user, token, refreshToken, refreshExpiresIn, sessionId, platform } }`。
- 服务端只在注册/登录时读取设备头；其他接口即使携带也不会覆盖 token 中的 `deviceId/platform`。
