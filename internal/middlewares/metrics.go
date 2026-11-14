package middlewares

import (
	"goim-business/internal/pkg/convert"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	metricsOnce sync.Once

	requestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "goim_business_http_requests_total",
			Help: "业务 HTTP 请求总数",
		},
		[]string{"method", "route", "status"},
	)
	requestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "goim_business_http_request_duration_seconds",
			Help:    "业务 HTTP 请求耗时",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route"},
	)
)

// Metrics 采集请求数量与耗时。
func Metrics() gin.HandlerFunc {
	metricsOnce.Do(func() {
		prometheus.MustRegister(requestsTotal, requestDuration)
	})
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		requestsTotal.WithLabelValues(c.Request.Method, route, convert.ToString(c.Writer.Status())).Inc()
		requestDuration.WithLabelValues(c.Request.Method, route).Observe(time.Since(start).Seconds())
	}
}

// MetricsHandler 返回 /metrics 的 Prometheus 处理器。
func MetricsHandler() http.Handler {
	return promhttp.Handler()
}
