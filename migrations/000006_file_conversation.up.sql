-- 聊天资源绑定会话：为空表示非聊天资源或历史数据。
ALTER TABLE file_objects
    ADD COLUMN conversation_id VARCHAR(64) NOT NULL DEFAULT '' AFTER purpose,
    ADD KEY idx_conversation (conversation_id, status);
