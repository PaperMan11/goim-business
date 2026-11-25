// Package goim 封装 goim im-api 的 HTTP 调用：admin token 缓存、用户开通与 user token 兑换。
package goim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"goim-business/internal/pkg/idgen"
)

const (
	// apiPrefix 是 goim im-api 的统一路由前缀。
	apiPrefix = "/api/v1"
	// headerOperationID 是 im-api 强制的请求追踪头。
	headerOperationID = "operationID"
	// headerToken 是 im-api 的认证头。
	headerToken = "token"

	defaultTimeout       = 3 * time.Second
	defaultRefreshAhead  = 60 * time.Second
	defaultTokenDuration = 300 * time.Second

	// codeNoPermission 是 admin token 失效时 im-api 返回的业务码。
	codeNoPermission = 1002

	methodAdminToken         = "admin_token"
	methodRegisterUser       = "register_user"
	methodUpdateUserInfo     = "update_user_info"
	methodUserToken          = "user_token"
	methodGroupMemberUserIDs = "group_member_user_ids"
)

// ErrDisabled 表示未配置 IM（base_url 为空）。
var ErrDisabled = errors.New("goim IM 客户端未启用")

var (
	metricsOnce sync.Once

	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goim_client_requests_total",
			Help: "goim im-api 客户端请求总数",
		},
		[]string{"method", "result"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goim_client_request_duration_seconds",
			Help:    "goim im-api 客户端请求耗时",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)
)

// Config 是 goim 客户端配置。
type Config struct {
	BaseURL           string
	AdminUserID       string
	AdminSecret       string
	Timeout           time.Duration
	TokenRefreshAhead time.Duration
}

// APIError 是 im-api 返回的业务错误；上层可用 errors.As 识别并映射。
type APIError struct {
	Code       int
	Message    string
	HTTPStatus int
}

// Error 实现 error 接口；不包含 secret/token 等敏感信息。
func (e *APIError) Error() string {
	return fmt.Sprintf("goim API 错误: code=%d msg=%s", e.Code, e.Message)
}

// Client 是 goim im-api 客户端，可安全并发使用。
type Client struct {
	baseURL     string
	adminUserID string
	adminSecret string
	httpClient  *http.Client

	// now 可替换，便于测试 token 过期逻辑。
	now               func() time.Time
	tokenRefreshAhead time.Duration

	mu                 sync.Mutex
	adminToken         string
	adminTokenExpireAt time.Time
}

// New 创建客户端；BaseURL 为空时返回可用但 disabled 的实例。
func New(cfg Config) *Client {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	ahead := cfg.TokenRefreshAhead
	if ahead <= 0 {
		ahead = defaultRefreshAhead
	}
	return &Client{
		baseURL:           strings.TrimRight(cfg.BaseURL, "/"),
		adminUserID:       cfg.AdminUserID,
		adminSecret:       cfg.AdminSecret,
		httpClient:        &http.Client{Timeout: timeout},
		now:               time.Now,
		tokenRefreshAhead: ahead,
	}
}

// Enabled 表示是否已配置 IM。
func (c *Client) Enabled() bool {
	return c.baseURL != ""
}

// apiResponse 是 im-api 的统一响应体。
type apiResponse struct {
	Code int             `json:"code"`
	Msg  string          `json:"msg"`
	Data json.RawMessage `json:"data"`
}

// tokenResponse 是 /auth/get_admin_token 与 /auth/get_user_token 的 data。
type tokenResponse struct {
	Token             string `json:"token"`
	ExpireTimeSeconds int64  `json:"expireTimeSeconds"`
}

func registerMetrics() {
	metricsOnce.Do(func() {
		prometheus.MustRegister(requestsTotal, requestDuration)
	})
}

// post 向 im-api 发起 POST 请求并校验统一响应包体。
// token 为空时不带认证头（用于白名单接口 get_admin_token）。
func (c *Client) post(ctx context.Context, method, path, token string, body any) (data json.RawMessage, err error) {
	registerMetrics()
	start := time.Now()
	defer func() {
		result := "ok"
		if err != nil {
			result = "failed"
		}
		requestsTotal.WithLabelValues(method, result).Inc()
		requestDuration.WithLabelValues(method).Observe(time.Since(start).Seconds())
	}()

	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("goim 请求编码失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPrefix+path, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("goim 请求构造失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set(headerOperationID, idgen.MustNewWithPrefix(idgen.PrefixOperation))
	if token != "" {
		req.Header.Set(headerToken, token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("goim 请求失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("goim 响应读取失败: %w", err)
	}

	var envelope apiResponse
	parseErr := json.Unmarshal(raw, &envelope)
	if resp.StatusCode != http.StatusOK {
		if parseErr == nil && envelope.Code != 0 {
			return nil, &APIError{Code: envelope.Code, Message: envelope.Msg, HTTPStatus: resp.StatusCode}
		}
		return nil, fmt.Errorf("goim 响应状态异常: HTTP %d", resp.StatusCode)
	}
	if parseErr != nil {
		return nil, fmt.Errorf("goim 响应解析失败: %w", parseErr)
	}
	if envelope.Code != 0 {
		return nil, &APIError{Code: envelope.Code, Message: envelope.Msg, HTTPStatus: resp.StatusCode}
	}
	return envelope.Data, nil
}

// durationFromSeconds 将响应中的秒数转为时长，非正数时按 300s 兜底。
func durationFromSeconds(seconds int64) time.Duration {
	if seconds <= 0 {
		return defaultTokenDuration
	}
	return time.Duration(seconds) * time.Second
}
