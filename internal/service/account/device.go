package account

import (
	"context"
	"time"

	domain "goim-business/internal/domain/account"
	"goim-business/internal/pkg/errcode"
	"goim-business/internal/types"
)

// touchDevice 在注册/登录成功时登记或刷新设备。
func (s *Service) touchDevice(ctx context.Context, userID int64, platform int8, deviceID, appVersion string, now time.Time) error {
	device, err := domain.NewDevice(userID, deviceID, platform, appVersion, "", now)
	if err != nil {
		return errcode.InvalidParam(err)
	}
	if err := s.devices.Upsert(ctx, device); err != nil {
		return s.dbErr("登记设备", err)
	}
	return nil
}

// Devices 列出当前用户已登记设备，按最近活跃时间倒序。
func (s *Service) Devices(ctx context.Context, userID int64) ([]types.DeviceView, error) {
	devices, err := s.devices.ListByUser(ctx, userID)
	if err != nil {
		return nil, s.dbErr("查询设备列表", err)
	}
	views := make([]types.DeviceView, 0, len(devices))
	for _, device := range devices {
		views = append(views, *deviceView(device))
	}
	return views, nil
}

// RemoveDevice 删除设备记录并撤销该设备的全部活跃会话；两者在事务内完成。
func (s *Service) RemoveDevice(ctx context.Context, userID int64, deviceID string) error {
	id, err := domain.NewDeviceID(deviceID)
	if err != nil {
		return errcode.InvalidParam(err)
	}

	now := s.now()
	var (
		found bool
		sids  []int64
	)
	if err := s.tx.WithTx(ctx, func(ctx context.Context) error {
		var txErr error
		found, txErr = s.devices.Delete(ctx, userID, id)
		if txErr != nil {
			return s.dbErr("删除设备", txErr)
		}
		sids, txErr = s.sessions.RevokeByDevice(ctx, userID, id, now)
		if txErr != nil {
			return s.dbErr("撤销设备会话", txErr)
		}
		if !found && len(sids) == 0 {
			return errcode.ErrNotFound
		}
		return nil
	}); err != nil {
		return err
	}

	for _, sid := range sids {
		if err := s.deactivateSession(ctx, sid); err != nil {
			return err
		}
	}
	return nil
}

func deviceView(device *domain.Device) *types.DeviceView {
	return &types.DeviceView{
		DeviceID:     device.DeviceID,
		Platform:     device.Platform,
		AppVersion:   device.AppVersion,
		LastActiveAt: device.LastActiveAt,
	}
}
