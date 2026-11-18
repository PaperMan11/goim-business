-- 000005_file_objects: 文件对象元数据表
-- 表归属：文件域（file），服务端持有 object_key，客户端只拿 file_id。
-- 生命周期：申请上传时写入 pending（带 expires_at）→ complete 校验通过后置 ready → 删除置 deleted。
-- 对象本体存放在 RustFS/S3 bucket 中，本表只保存元数据与归属，不存文件内容。

CREATE TABLE file_objects (
  id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '文件 ID；对外以字符串下发',
  user_id     BIGINT UNSIGNED NOT NULL COMMENT '上传者用户 ID，逻辑关联 account_users.id（无数据库外键）',
  purpose     VARCHAR(32)  NOT NULL COMMENT '用途：avatar 等；用于配额、校验规则与访问策略分流',
  object_key  VARCHAR(512) NOT NULL COMMENT 'bucket 内对象路径，由服务端生成，客户端不可指定',
  file_name   VARCHAR(255) NOT NULL DEFAULT '' COMMENT '客户端原始文件名，仅用于展示与审计',
  mime        VARCHAR(128) NOT NULL DEFAULT '' COMMENT '申请上传时声明的 Content-Type，complete 时与对象实际类型比对',
  size        BIGINT       NOT NULL DEFAULT 0 COMMENT '对象实际字节数；pending 阶段为 0，complete 时由 HeadObject 回填',
  sha256      CHAR(64)     NOT NULL DEFAULT '' COMMENT '客户端可选上报的 SHA-256 十六进制摘要',
  upload_id   VARCHAR(128) NOT NULL DEFAULT '' COMMENT 'S3 分片上传 ID；单文件 PUT 上传为空，complete/abort 时按此 ID 操作',
  status      TINYINT      NOT NULL DEFAULT 0 COMMENT '状态：0待上传 1可用 2已删除',
  expires_at  DATETIME(3)  NULL COMMENT 'pending 超时时间；过期仍未 complete 时由清理任务删除对象与记录',
  created_at  DATETIME(3)  NOT NULL COMMENT '创建时间（UTC）',
  updated_at  DATETIME(3)  NOT NULL COMMENT '更新时间（UTC）',
  UNIQUE KEY uk_object_key (object_key) COMMENT '对象路径唯一，防止覆盖他人文件',
  KEY idx_user_status (user_id, status, created_at) COMMENT '按用户与状态查询文件列表、统计配额',
  KEY idx_status_expires (status, expires_at) COMMENT '扫描超时 pending 记录，支撑清理任务'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='文件对象元数据表';
