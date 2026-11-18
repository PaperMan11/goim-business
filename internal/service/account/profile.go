package account

import (
	"context"
	"errors"
	"strings"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

// Profile 返回当前用户的完整资料。
func (s *Service) Profile(ctx context.Context, userID int64) (*types.ProfileView, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("查询用户资料", err)
	}
	return s.profileView(user), nil
}

// UpdateProfile 更新用户资料，并在同一事务内写入 IM 同步任务；提交后即时同步一次。
func (s *Service) UpdateProfile(ctx context.Context, userID int64, in types.UpdateProfileRequest) (*types.ProfileView, error) {
	if in.Nickname == nil && in.AvatarKey == nil && in.Bio == nil {
		return nil, errcode.ErrInvalidParam.WithMessage("至少需要更新一个字段")
	}

	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("查询用户资料", err)
	}

	nickname, avatarKey, bio := user.Nickname, user.AvatarKey, user.Bio
	if in.Nickname != nil {
		value, err := domain.NewNickname(*in.Nickname, "")
		if err != nil {
			return nil, errcode.InvalidParam(err)
		}
		if value == "" {
			return nil, errcode.ErrInvalidParam.WithMessage("昵称不能为空")
		}
		nickname = value
	}
	if in.AvatarKey != nil {
		avatarKey = strings.TrimSpace(*in.AvatarKey)
	}
	if in.Bio != nil {
		bio = strings.TrimSpace(*in.Bio)
	}

	now := s.now()
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		updated, err := s.users.UpdateProfile(ctx, user.ID, nickname, avatarKey, bio, now)
		if err != nil {
			return s.dbErr("更新用户资料", err)
		}
		if !updated {
			return errcode.ErrConflict.WithMessage("账号状态已变化，无法更新资料")
		}
		// 与资料变更同事务写入：只要事务提交，IM 同步任务就不会丢。
		if err := s.enqueueIMOutbox(ctx, user.ID, ports.OutboxActionProfileSync); err != nil {
			return s.dbErr("更新资料写入 IM outbox", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	user.Nickname = nickname
	user.AvatarKey = avatarKey
	user.Bio = bio
	user.UpdatedAt = now

	// IM 账号未开通时不再即时调用，避免必然失败的请求；任务由后台补偿在开通完成后执行。
	if s.im != nil && s.im.Enabled() && user.IMStatus == domain.IMStatusReady {
		syncCtx := context.WithoutCancel(ctx)
		syncErr := s.imSync.SyncProfile(syncCtx, user)
		s.imSync.Settle(syncCtx, user.ID, ports.OutboxActionProfileSync, syncErr)
	}
	return s.profileView(user), nil
}

// Profiles 批量返回公开资料；不存在的用户直接跳过。
func (s *Service) Profiles(ctx context.Context, ids []int64) ([]types.PublicProfileView, error) {
	if len(ids) == 0 {
		return []types.PublicProfileView{}, nil
	}
	users, err := s.users.GetByIDs(ctx, ids)
	if err != nil {
		return nil, s.dbErr("批量查询用户资料", err)
	}
	views := make([]types.PublicProfileView, 0, len(users))
	for _, user := range users {
		// 注销中/已注销账号不再对外可见，从批量结果中静默跳过。
		if user.Status == domain.StatusDeleting || user.Status == domain.StatusDeleted {
			continue
		}
		views = append(views, s.publicProfileView(user))
	}
	return views, nil
}

func (s *Service) profileView(user *domain.User) *types.ProfileView {
	return &types.ProfileView{
		UserID:    convert.ToString(user.ID),
		LoginName: user.LoginName,
		Nickname:  user.Nickname,
		AvatarKey: user.AvatarKey,
		AvatarURL: s.resolveAvatarURL(user.AvatarKey),
		Bio:       user.Bio,
	}
}

func (s *Service) publicProfileView(user *domain.User) types.PublicProfileView {
	return types.PublicProfileView{
		UserID:    convert.ToString(user.ID),
		Nickname:  user.Nickname,
		AvatarKey: user.AvatarKey,
		AvatarURL: s.resolveAvatarURL(user.AvatarKey),
	}
}

// resolveAvatarURL 把头像 key 解析为公开访问 URL；已是 http(s) 地址时原样返回。
func (s *Service) resolveAvatarURL(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	if strings.HasPrefix(key, "http://") || strings.HasPrefix(key, "https://") {
		return key
	}
	if s.cfg.AvatarURL == nil {
		return ""
	}
	return s.cfg.AvatarURL(key)
}
