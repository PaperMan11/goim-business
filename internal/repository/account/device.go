package account

import (
	"context"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

func deviceRecord(device *domain.Device) *model.Device {
	return &model.Device{
		ID:           device.ID,
		UserID:       device.UserID,
		DeviceID:     device.DeviceID,
		Platform:     device.Platform,
		AppVersion:   device.AppVersion,
		PushToken:    device.PushToken,
		LastActiveAt: device.LastActiveAt,
		CreatedAt:    device.CreatedAt,
		UpdatedAt:    device.UpdatedAt,
	}
}

func deviceFromRecord(record *model.Device) *domain.Device {
	return &domain.Device{
		ID:           record.ID,
		UserID:       record.UserID,
		DeviceID:     record.DeviceID,
		Platform:     record.Platform,
		AppVersion:   record.AppVersion,
		PushToken:    record.PushToken,
		LastActiveAt: record.LastActiveAt,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}
}

// DeviceRepo 是 account_devices 表的仓储实现。
type DeviceRepo struct {
	db *gorm.DB
}

// NewDeviceRepo 构造设备仓储。
func NewDeviceRepo(db *gorm.DB) *DeviceRepo {
	return &DeviceRepo{db: db}
}

func (r *DeviceRepo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Upsert 按 (user_id, device_id) 覆盖写入设备；重复注册只更新可变字段，保留首次注册时间。
func (r *DeviceRepo) Upsert(ctx context.Context, device *domain.Device) error {
	record := deviceRecord(device)
	return mapErr(r.dbFrom(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "device_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"platform":       record.Platform,
			"app_version":    record.AppVersion,
			"push_token":     record.PushToken,
			"last_active_at": record.LastActiveAt,
			"updated_at":     record.UpdatedAt,
		}),
	}).Create(record).Error)
}

// ListByUser 列出用户已登记设备，按最近活跃时间倒序。
func (r *DeviceRepo) ListByUser(ctx context.Context, userID int64) ([]*domain.Device, error) {
	var records []model.Device
	if err := r.dbFrom(ctx).
		Where("user_id = ?", userID).
		Order("last_active_at DESC, id DESC").
		Find(&records).Error; err != nil {
		return nil, mapErr(err)
	}
	devices := make([]*domain.Device, 0, len(records))
	for i := range records {
		devices = append(devices, deviceFromRecord(&records[i]))
	}
	return devices, nil
}

// Delete 删除设备记录；返回删除前记录是否存在。
func (r *DeviceRepo) Delete(ctx context.Context, userID int64, deviceID string) (bool, error) {
	res := r.dbFrom(ctx).
		Where("user_id = ? AND device_id = ?", userID, deviceID).
		Delete(&model.Device{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// DeleteByUser 物理删除用户全部设备记录，返回删除条数。
func (r *DeviceRepo) DeleteByUser(ctx context.Context, userID int64) (int64, error) {
	result := r.dbFrom(ctx).
		Where("user_id = ?", userID).
		Delete(&model.Device{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
