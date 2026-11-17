package account

import (
	"strings"
	"time"
)

// Device 是用户在某一台设备上的登记记录；同一用户同一 deviceID 只保留一行。
type Device struct {
	ID           int64
	UserID       int64
	DeviceID     string
	Platform     int8
	AppVersion   string
	PushToken    string
	LastActiveAt time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewDevice 归一化并校验设备注册信息。
func NewDevice(userID int64, deviceID string, platform int8, appVersion, pushToken string, at time.Time) (*Device, error) {
	id, err := NewDeviceID(strings.TrimSpace(deviceID))
	if err != nil {
		return nil, err
	}
	return &Device{
		UserID:       userID,
		DeviceID:     id,
		Platform:     platform,
		AppVersion:   strings.TrimSpace(appVersion),
		PushToken:    strings.TrimSpace(pushToken),
		LastActiveAt: at,
		CreatedAt:    at,
		UpdatedAt:    at,
	}, nil
}
