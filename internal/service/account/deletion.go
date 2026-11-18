package account

import (
	"context"
	"errors"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/convert"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/service/ports"
	"goim-business/internal/types"
)

// imTokenPlatforms 是注销时需要清理 IM token 缓存的平台编码；逐个 DEL，不使用 SCAN。
// 需覆盖 httpx 支持的全部平台，包含 HarmonyOS(11)。
var imTokenPlatforms = []int8{1, 2, 3, 4, 5, 11}

// DeleteAccount 注销当前账号：立即下线、清理缓存并同步 IM，冷却期结束后由定时任务物理清除。
func (s *Service) DeleteAccount(ctx context.Context, userID int64) (*types.DeleteAccountResp, error) {
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, errcode.ErrNotFound
		}
		return nil, s.dbErr("查询注销用户", err)
	}

	now := s.now()
	alreadyDeleting := user.Status == domain.StatusDeleting && user.PurgeAt != nil
	switch user.Status {
	case domain.StatusNormal:
	case domain.StatusFrozen:
		return nil, errcode.ErrAccountFrozen
	case domain.StatusDeleting:
		// 注销中：幂等返回已有的清除时间。
		if user.PurgeAt == nil {
			return nil, errcode.ErrConflict.WithMessage("账号状态已变化，请刷新后重试")
		}
	case domain.StatusDeleted:
		return nil, errcode.ErrUnauthenticated
	default:
		return nil, errcode.ErrConflict.WithMessage("账号状态已变化，请刷新后重试")
	}

	purgeAt := now.Add(s.cfg.DeletionCooldown)
	if user.PurgeAt != nil {
		purgeAt = *user.PurgeAt
	}

	if !alreadyDeleting {
		if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
			ok, err := s.users.MarkDeleting(ctx, user.ID, purgeAt, now)
			if err != nil {
				return s.dbErr("标记账号注销", err)
			}
			if !ok {
				return errcode.ErrConflict.WithMessage("账号状态已变化，请刷新后重试")
			}
			// 与注销状态同事务写入：只要事务提交，IM 注销同步就不会丢。
			if err := s.enqueueIMOutboxPayload(ctx, user.ID, ports.OutboxActionPurgeIM, "{}"); err != nil {
				return s.dbErr("注销写入 IM outbox", err)
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	// 立即下线：撤销全部活跃会话并清除 Redis 活跃标记。
	sids, err := s.sessions.RevokeAll(ctx, user.ID, now, 0)
	if err != nil {
		return nil, s.dbErr("撤销注销账号会话", err)
	}
	stateFailed := false
	for _, sid := range sids {
		if err := s.state.Deactivate(ctx, convert.ToString(sid)); err != nil {
			s.log.Errorf("清除会话活跃标记失败: sid=%d, err=%v", sid, err)
			stateFailed = true
		}
	}

	// 清除 IM token 缓存；失败只记日志，缓存有 TTL 且业务 token 已失效无法再换取。
	if s.imTokens != nil {
		key := convert.ToString(user.ID)
		for _, platform := range imTokenPlatforms {
			if err := s.imTokens.Delete(ctx, key, platform); err != nil {
				s.log.Warnf("清除 IM token 缓存失败: user_id=%d, platform=%d, err=%v", user.ID, platform, err)
			}
		}
	}
	if stateFailed {
		// 账号已进入注销中，重试会幂等返回 purgeAt；这里返回依赖失败提示客户端稍后重试。
		return nil, errcode.ErrDependency
	}

	// 尽力即时同步 IM；失败由 outbox 补偿，不影响注销结果。
	// 从未成功开通（pending/failed）时 IM 侧没有投影，直接落定 outbox，避免无意义调用失败重试。
	if !alreadyDeleting && s.im != nil && s.im.Enabled() {
		syncCtx := context.WithoutCancel(ctx)
		user.Status = domain.StatusDeleting
		user.PurgeAt = &purgeAt
		user.UpdatedAt = now
		if user.IMStatus == domain.IMStatusReady {
			purgeErr := s.imSync.Purge(syncCtx, user)
			s.imSync.Settle(syncCtx, user.ID, ports.OutboxActionPurgeIM, purgeErr)
		} else {
			s.imSync.Settle(syncCtx, user.ID, ports.OutboxActionPurgeIM, nil)
		}
	}

	return &types.DeleteAccountResp{PurgeAt: purgeAt}, nil
}
