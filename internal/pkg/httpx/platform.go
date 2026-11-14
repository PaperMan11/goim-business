package httpx

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"goim-business/internal/pkg/errcode"
)

// 平台 ID 与服务端常量一致：iOS=1、Android=2、Windows=3、macOS=4、Web=5、HarmonyOS=11。
const (
	PlatformIOS       int8 = 1
	PlatformAndroid   int8 = 2
	PlatformWindows   int8 = 3
	PlatformMacOS     int8 = 4
	PlatformWeb       int8 = 5
	PlatformHarmonyOS int8 = 11
)

var validPlatforms = map[int8]struct{}{
	PlatformIOS: {}, PlatformAndroid: {}, PlatformWindows: {},
	PlatformMacOS: {}, PlatformWeb: {}, PlatformHarmonyOS: {},
}

// PlatformFrom 读取 X-Client-Platform；缺省按 Web，非法取值直接拒绝。
func PlatformFrom(c *gin.Context) (int8, error) {
	raw := strings.TrimSpace(strings.ToLower(c.GetHeader("X-Client-Platform")))
	if raw == "" {
		return PlatformWeb, nil
	}
	switch raw {
	case "ios":
		return PlatformIOS, nil
	case "android":
		return PlatformAndroid, nil
	case "windows", "win":
		return PlatformWindows, nil
	case "macos", "mac", "osx":
		return PlatformMacOS, nil
	case "web", "browser":
		return PlatformWeb, nil
	case "harmony", "harmonyos":
		return PlatformHarmonyOS, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errcode.ErrInvalidParam.WithMessage("不支持的客户端平台")
	}
	platform := int8(value)
	if _, ok := validPlatforms[platform]; !ok {
		return 0, errcode.ErrInvalidParam.WithMessage("不支持的客户端平台")
	}
	return platform, nil
}
