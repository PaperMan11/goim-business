// Package main 是 goim-business API 进程入口。
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"goim-business/internal/app"
	"goim-business/internal/config"
	"goim-business/internal/handler"
	"goim-business/internal/middlewares"
	"goim-business/internal/pkg/logger"
	"goim-business/internal/pkg/run"
)

var version = "dev"

func main() {
	if err := serve(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func serve() error {
	env := envOr("GOIM_BIZ_ENV", "dev")
	cfg, err := config.Load(os.Getenv("GOIM_BIZ_CONFIG_DIR"), env)
	if err != nil {
		return err
	}

	log, err := logger.New(cfg.Log, cfg.Env, version)
	if err != nil {
		return err
	}
	defer func() { _ = log.Sync() }()

	if cfg.Env == "prod" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	a, err := app.NewApp(context.Background(), cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	httpSrv := &http.Server{
		Addr:         cfg.Server.HTTPAddr,
		Handler:      handler.InitRoutes(a),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}
	probeSrv := &http.Server{
		Addr:         cfg.Server.ProbeAddr,
		Handler:      probeHandler(a),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	metricsSrv := &http.Server{
		Addr:         cfg.Server.MetricsAddr,
		Handler:      middlewares.MetricsHandler(),
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	var g run.Group
	{
		execute, interrupt := run.SignalHandler(context.Background(), os.Interrupt, syscall.SIGTERM)
		g.Add(execute, interrupt)
	}

	servers := map[string]*http.Server{
		"api":     httpSrv,
		"probe":   probeSrv,
		"metrics": metricsSrv,
	}
	for name, srv := range servers {
		g.Add(
			func() error {
				log.Info("HTTP 服务启动", zap.String("name", name), zap.String("addr", srv.Addr))
				if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					return fmt.Errorf("%s 服务异常退出: %w", name, err)
				}
				return nil
			},
			func(error) {
				shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
				defer cancel()
				if err := srv.Shutdown(shutdownCtx); err != nil {
					log.Error("关闭 HTTP 服务失败", zap.String("name", name), zap.Error(err))
				}
			},
		)
	}

	if err := g.Run(); err != nil && !errors.Is(err, run.ErrSignal) {
		return err
	}
	log.Info("服务已退出")
	return nil
}

// probeHandler 提供存活与就绪探针（MySQL / Redis 均可用才算就绪）。
func probeHandler(a *app.App) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := a.SQLDB.PingContext(ctx); err != nil {
			http.Error(w, "mysql unavailable", http.StatusServiceUnavailable)
			return
		}
		if err := a.Redis.Ping(ctx).Err(); err != nil {
			http.Error(w, "redis unavailable", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
