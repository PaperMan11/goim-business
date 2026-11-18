-- 000001_account_init: M1 账号域三张表
-- 表归属：账号服务（account），故统一使用 account_ 前缀，便于同库内识别归属。
-- 主键约定：BIGINT UNSIGNED 自增；对前端与 IM 下发时由应用层转为字符串。
-- 时间约定：所有时间列均为 DATETIME(3)，应用层统一写入 UTC，展示时再转本地时区。
-- 外键约定：不使用数据库外键，全部为逻辑外键，由应用层保证引用一致性；
--           避免高频写入时的锁竞争，也便于后续分库分表与数据迁移。

-- 用户主表：一个账号一行，承载登录标识与资料快照。
CREATE TABLE account_users (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '业务 userID；对 IM 下发时转为字符串',
  login_name    VARCHAR(64)  NOT NULL COMMENT '登录名：去空格并转小写后存储，3-64 位，仅字母/数字/下划线',
  nickname      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '昵称；注册时为空则回退为 login_name，最多 64 个字符',
  avatar_key    VARCHAR(255) NOT NULL DEFAULT '' COMMENT '头像对象存储 key，不存完整 URL',
  bio           VARCHAR(255) NOT NULL DEFAULT '' COMMENT '个人简介',
  status        TINYINT      NOT NULL DEFAULT 1 COMMENT '账号状态：1正常 2冻结 3注销中 4已注销',
  im_status     TINYINT      NOT NULL DEFAULT 0 COMMENT 'IM 账号状态：0待开通 1已开通 2失败 3已注销',
  im_synced_at  DATETIME(3)  NULL COMMENT '最近一次与 IM 同步成功的时间；NULL 表示尚未同步',
  purge_at      DATETIME(3)  NULL COMMENT '注销冷却期结束后的物理清除时间（UTC）；正常账号为 NULL',
  last_login_at DATETIME(3)  NULL COMMENT '最近一次登录成功时间；NULL 表示从未登录',
  created_at    DATETIME(3)  NOT NULL COMMENT '创建时间（UTC）',
  updated_at    DATETIME(3)  NOT NULL COMMENT '更新时间（UTC）',
  UNIQUE KEY uk_login_name (login_name) COMMENT '登录名唯一，支撑按登录名查找与注册去重',
  KEY idx_status (status) COMMENT '按状态筛选，支撑冻结/注销账号的批量运维',
  KEY idx_status_purge (status, purge_at) COMMENT '扫描到期待清除账号，支撑注销冷却期后的异步清除'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号用户主表';

-- 密码凭证表：与 account_users 一对一，独立成表便于后续扩展多种凭证类型。
CREATE TABLE account_credentials (
  user_id       BIGINT UNSIGNED NOT NULL PRIMARY KEY COMMENT '逻辑关联 account_users.id，同时作为主键保证一对一',
  password_hash VARCHAR(255) NOT NULL COMMENT '密码哈希：argon2id 编码串，禁止存明文或可逆密文',
  updated_at    DATETIME(3)  NOT NULL COMMENT '凭证最近变更时间（UTC），改密后用于审计'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号密码凭证表';

-- 会话表：一次登录一行；刷新时旧行写 rotated_at，新行继承 family_id。
-- 生命周期：创建（revoked_at/rotated_at 均为 NULL）→ 轮换（rotated_at 非空）或撤销（revoked_at 非空）。
CREATE TABLE account_sessions (
  id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '会话 ID（sid），写入 JWT 的 sid 声明后以字符串形式输出',
  user_id      BIGINT UNSIGNED NOT NULL COMMENT '逻辑关联 account_users.id（无数据库外键）',
  family_id    VARCHAR(40)  NOT NULL COMMENT '刷新轮换家族 ID（acc_ + UUIDv7）；同一登录链路的所有会话共享，检测到重放时整族撤销',
  refresh_hash CHAR(64)     NOT NULL COMMENT 'refresh token 的 SHA-256 十六进制摘要；只存摘要，不存明文令牌',
  rotated_from BIGINT UNSIGNED NULL COMMENT '由哪个会话轮换而来；NULL 表示该家族的首次登录',
  platform     TINYINT      NOT NULL COMMENT '客户端平台编码：1 iOS 2 Android 3 Windows 4 Mac 5 Web',
  device_id    VARCHAR(64)  NOT NULL COMMENT '设备标识，由客户端上报，用于会话识别与踢出',
  user_agent   VARCHAR(255) NOT NULL DEFAULT '' COMMENT '创建会话时的 User-Agent，仅用于展示与审计',
  ip           VARCHAR(45)  NOT NULL DEFAULT '' COMMENT '创建会话时的客户端 IP（兼容 IPv6）',
  expires_at   DATETIME(3)  NOT NULL COMMENT 'refresh token 过期时间（UTC），过期后会话不可再刷新',
  revoked_at   DATETIME(3)  NULL COMMENT '撤销时间：登出、被踢出、改密或重放处理时写入',
  rotated_at   DATETIME(3)  NULL COMMENT '轮换时间：refresh token 使用成功后写入，再次使用即判定为重放',
  last_seen_at DATETIME(3)  NULL COMMENT '最近活跃时间；NULL 表示尚未上报活跃',
  created_at   DATETIME(3)  NOT NULL COMMENT '会话创建时间（UTC），会话超限时按此字段决定踢出顺序',
  UNIQUE KEY uk_refresh_hash (refresh_hash) COMMENT 'refresh token 摘要唯一，支撑刷新时精确定位会话',
  KEY idx_user_platform (user_id, platform) COMMENT '按用户与平台统计活跃会话，支撑每平台会话上限',
  KEY idx_family (family_id) COMMENT '按家族批量撤销，支撑重放检测后的整族下线'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号登录会话表';
