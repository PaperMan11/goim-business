-- 000004_client_configs: 客户端配置表
-- 表归属：账号服务（account）。platform=0 为全平台默认，具体平台行覆盖默认值。
-- version 由运维在调整配置时递增；客户端通过 ETag 缓存，版本变化后自动拉取新配置。

CREATE TABLE client_configs (
  id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '配置记录 ID',
  platform   TINYINT      NOT NULL DEFAULT 0 COMMENT '平台编码；0 表示全平台默认',
  version    INT          NOT NULL DEFAULT 1 COMMENT '配置版本，递增后客户端 ETag 失效',
  content    JSON         NOT NULL COMMENT '配置内容：features/limits/minAppVersion',
  created_at DATETIME(3)  NOT NULL COMMENT '创建时间（UTC）',
  updated_at DATETIME(3)  NOT NULL COMMENT '更新时间（UTC）',
  UNIQUE KEY uk_platform (platform) COMMENT '每个平台只保留一行配置'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='客户端配置表';

INSERT INTO client_configs (platform, version, content, created_at, updated_at) VALUES (
  0,
  1,
  '{"features":{"voiceMessage":false,"fileUpload":true},"limits":{"maxUploadBytes":104857600,"maxAvatarBytes":5242880},"minAppVersion":{"web":"1.0.0","desktop":"1.0.0"}}',
  UTC_TIMESTAMP(3),
  UTC_TIMESTAMP(3)
);
