-- 000002_outbox_events: 通用 outbox 任务表（事务性 outbox）
-- 表归属：跨业务通用投递机制，不属于任何业务域；user_id 为逻辑关联，不建数据库外键。
-- 时间约定：沿用 DATETIME(3)，应用层统一写入 UTC。

CREATE TABLE outbox_events (
  id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY COMMENT '任务 ID',
  user_id       BIGINT UNSIGNED NOT NULL COMMENT '逻辑关联 account_users.id（无数据库外键），由具体 action 解释',
  action        VARCHAR(32)  NOT NULL COMMENT '任务类型，本轮为 provision_im',
  payload       TEXT         NOT NULL DEFAULT ('') COMMENT '任务参数（JSON 字符串），本轮为空',
  status        TINYINT      NOT NULL DEFAULT 0 COMMENT '任务状态：0待处理 1已完成 2失败待重试',
  attempts      INT          NOT NULL DEFAULT 0 COMMENT '已失败次数；成功后保留最近一次失败次数',
  next_retry_at DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '下次可执行时间（UTC），默认当前时间',
  last_error    VARCHAR(512) NOT NULL DEFAULT '' COMMENT '最近一次失败原因，最多 512 个字符',
  created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '创建时间（UTC）',
  updated_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3) COMMENT '更新时间（UTC）',
  KEY idx_status_next_retry (status, next_retry_at) COMMENT '按状态与到期时间扫描待处理任务',
  KEY idx_user_action (user_id, action) COMMENT '按用户与任务类型定位任务'
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_ai_ci COMMENT='通用 outbox 任务表';
