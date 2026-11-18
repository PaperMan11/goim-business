package account

import (
	"context"
	"time"

	"gorm.io/gorm"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/model"
	"goim-business/internal/repository"
)

func sessionRecord(session *domain.Session) *model.Session {
	return &model.Session{
		ID:          session.ID,
		UserID:      session.UserID,
		FamilyID:    session.FamilyID,
		RefreshHash: session.RefreshHash,
		RotatedFrom: session.RotatedFrom,
		Platform:    session.Platform,
		DeviceID:    session.DeviceID,
		UserAgent:   session.UserAgent,
		IP:          session.IP,
		ExpiresAt:   session.ExpiresAt,
		RevokedAt:   session.RevokedAt,
		RotatedAt:   session.RotatedAt,
		LastSeenAt:  session.LastSeenAt,
		CreatedAt:   session.CreatedAt,
	}
}

func sessionFromRecord(record *model.Session) *domain.Session {
	return &domain.Session{
		ID:          record.ID,
		UserID:      record.UserID,
		FamilyID:    record.FamilyID,
		RefreshHash: record.RefreshHash,
		RotatedFrom: record.RotatedFrom,
		Platform:    record.Platform,
		DeviceID:    record.DeviceID,
		UserAgent:   record.UserAgent,
		IP:          record.IP,
		ExpiresAt:   record.ExpiresAt,
		RevokedAt:   record.RevokedAt,
		RotatedAt:   record.RotatedAt,
		LastSeenAt:  record.LastSeenAt,
		CreatedAt:   record.CreatedAt,
	}
}

// SessionRepo 是 account_sessions 表的仓储实现。
type SessionRepo struct {
	db *gorm.DB
}

// NewSessionRepo 构造会话仓储。
func NewSessionRepo(db *gorm.DB) *SessionRepo {
	return &SessionRepo{db: db}
}

func (r *SessionRepo) dbFrom(ctx context.Context) *gorm.DB {
	return repository.DBFromContext(ctx, r.db)
}

// Create 插入会话并回填自增 ID。
func (r *SessionRepo) Create(ctx context.Context, session *domain.Session) error {
	record := sessionRecord(session)
	if err := mapErr(r.dbFrom(ctx).Create(record).Error); err != nil {
		return err
	}
	session.ID = record.ID
	return nil
}

// GetByRefreshHash 按 refresh 哈希查询会话。
func (r *SessionRepo) GetByRefreshHash(ctx context.Context, hash string) (*domain.Session, error) {
	var record model.Session
	err := r.dbFrom(ctx).Where("refresh_hash = ?", hash).First(&record).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return sessionFromRecord(&record), nil
}

// GetByID 按 sid 查询会话。
func (r *SessionRepo) GetByID(ctx context.Context, id int64) (*domain.Session, error) {
	var record model.Session
	err := r.dbFrom(ctx).Where("id = ?", id).First(&record).Error
	if err != nil {
		return nil, mapErr(err)
	}
	return sessionFromRecord(&record), nil
}

// MarkRotated 原子地把会话标记为已轮换；并发刷新时只有一个请求能把 false→true。
func (r *SessionRepo) MarkRotated(ctx context.Context, id int64, at time.Time) (bool, error) {
	res := r.dbFrom(ctx).Model(&model.Session{}).
		Where("id = ? AND rotated_at IS NULL AND revoked_at IS NULL", id).
		Update("rotated_at", at)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// Revoke 撤销指定会话（幂等）。
func (r *SessionRepo) Revoke(ctx context.Context, id int64, at time.Time) error {
	return mapErr(r.dbFrom(ctx).Model(&model.Session{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", at).Error)
}

// RevokeAll 撤销用户全部活跃会话，返回被撤销的 sid（用于清理 Redis 活跃标记）。
func (r *SessionRepo) RevokeAll(ctx context.Context, userID int64, at time.Time, exceptSID int64) ([]int64, error) {
	db := r.dbFrom(ctx)
	var sids []int64
	q := db.Model(&model.Session{}).
		Where("user_id = ? AND revoked_at IS NULL AND rotated_at IS NULL", userID)
	if exceptSID != 0 {
		q = q.Where("id <> ?", exceptSID)
	}
	if err := q.Pluck("id", &sids).Error; err != nil {
		return nil, err
	}
	if len(sids) == 0 {
		return nil, nil
	}

	q2 := db.Model(&model.Session{}).
		Where("user_id = ? AND revoked_at IS NULL", userID)
	if exceptSID != 0 {
		q2 = q2.Where("id <> ?", exceptSID)
	}
	if err := q2.Update("revoked_at", at).Error; err != nil {
		return nil, err
	}
	return sids, nil
}

// RevokeFamily 撤销整个刷新家族（重放检测触发），返回本次被撤销的 sid。
func (r *SessionRepo) RevokeFamily(ctx context.Context, familyID string, at time.Time) ([]int64, error) {
	db := r.dbFrom(ctx)
	var sids []int64
	if err := db.Model(&model.Session{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Pluck("id", &sids).Error; err != nil {
		return nil, err
	}
	if len(sids) == 0 {
		return nil, nil
	}
	if err := db.Model(&model.Session{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", at).Error; err != nil {
		return nil, err
	}
	return sids, nil
}

// RevokeByDevice 撤销用户指定设备的全部活跃会话，返回被撤销的 sid。
func (r *SessionRepo) RevokeByDevice(ctx context.Context, userID int64, deviceID string, at time.Time) ([]int64, error) {
	db := r.dbFrom(ctx)
	var sids []int64
	if err := db.Model(&model.Session{}).
		Where("user_id = ? AND device_id = ? AND revoked_at IS NULL AND rotated_at IS NULL", userID, deviceID).
		Pluck("id", &sids).Error; err != nil {
		return nil, err
	}
	if len(sids) == 0 {
		return nil, nil
	}
	if err := db.Model(&model.Session{}).
		Where("user_id = ? AND device_id = ? AND revoked_at IS NULL", userID, deviceID).
		Update("revoked_at", at).Error; err != nil {
		return nil, err
	}
	return sids, nil
}

// DeleteByUser 物理删除用户全部会话，返回删除条数。
func (r *SessionRepo) DeleteByUser(ctx context.Context, userID int64) (int64, error) {
	result := r.dbFrom(ctx).
		Where("user_id = ?", userID).
		Delete(&model.Session{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ListActive 列出用户当前活跃会话（未撤销、未轮换、未过期），按创建时间升序（最旧在前）。
func (r *SessionRepo) ListActive(ctx context.Context, userID int64, at time.Time) ([]*domain.Session, error) {
	var records []*model.Session
	err := r.dbFrom(ctx).
		Where("user_id = ? AND revoked_at IS NULL AND rotated_at IS NULL AND expires_at > ?", userID, at).
		Order("created_at ASC").
		Find(&records).Error
	if err != nil {
		return nil, err
	}
	sessions := make([]*domain.Session, 0, len(records))
	for _, record := range records {
		sessions = append(sessions, sessionFromRecord(record))
	}
	return sessions, nil
}
