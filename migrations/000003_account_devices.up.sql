-- 000003_account_devices: 设备注册表
-- 表归属：账号服务（account），用于多端识别、设备列表与按设备踢出。
-- 唯一键 (user_id, device_id)：同一用户同一设备只保留一行，重复注册按最新状态覆盖。

CREATE TABLE account_devices (
  id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '设备记录 ID',
  user_id        BIGINT UNSIGNED NOT NULL COMMENT '逻辑关联 account_users.id（无数据库外键）',
  device_id      VARCHAR(64)  NOT NULL COMMENT '客户端上报的设备标识，同一用户内唯一',
  platform       TINYINT      NOT NULL COMMENT '平台编码：1 iOS 2 Android 3 Windows 4 Mac 5 Web 11 HarmonyOS',
  app_version    VARCHAR(32)  NOT NULL DEFAULT '' COMMENT '客户端版本号',
  push_token     VARCHAR(255) NOT NULL DEFAULT '' COMMENT '推送 token，空表示未开启推送',
  last_active_at DATETIME(3)  NOT NULL COMMENT '最近活跃时间（UTC）',
  created_at     DATETIME(3)  NOT NULL COMMENT '首次注册时间（UTC）',
  updated_at     DATETIME(3)  NOT NULL COMMENT '更新时间（UTC）',
  UNIQUE KEY uk_user_device (user_id, device_id) COMMENT '同一用户同一设备只保留一行',
  KEY idx_user_active (user_id, last_active_at) COMMENT '按用户列出设备并按活跃时间排序'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='账号设备注册表';
