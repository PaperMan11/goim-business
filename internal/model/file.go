package model

import "time"

// 文件对象状态，与 file_objects.status 保持一致。
const (
	FileStatusPending int8 = 0
	FileStatusReady   int8 = 1
	FileStatusDeleted int8 = 2
)

// FileObject 是文件元数据记录；对象本体存放在 RustFS/S3 bucket 中。
type FileObject struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int64      `gorm:"column:user_id"`
	Purpose   string     `gorm:"column:purpose;size:32"`
	ObjectKey string     `gorm:"column:object_key;size:512"`
	FileName  string     `gorm:"column:file_name;size:255"`
	MIME      string     `gorm:"column:mime;size:128"`
	Size      int64      `gorm:"column:size"`
	SHA256    string     `gorm:"column:sha256;size:64"`
	UploadID  string     `gorm:"column:upload_id;size:128"`
	Status    int8       `gorm:"column:status"`
	ExpiresAt *time.Time `gorm:"column:expires_at"`
	CreatedAt time.Time  `gorm:"column:created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at"`
}

func (FileObject) TableName() string { return "file_objects" }
