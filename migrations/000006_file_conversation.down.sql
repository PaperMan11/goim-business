ALTER TABLE file_objects
    DROP INDEX idx_conversation,
    DROP COLUMN conversation_id;
