-- 000005_file_objects: 回滚文件对象元数据表
-- 注意：回滚只删除元数据表，bucket 内已上传的对象需由运维按 object_key 另行清理。

DROP TABLE IF EXISTS file_objects;
