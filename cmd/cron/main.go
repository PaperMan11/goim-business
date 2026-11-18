// Package main 是 goim-business 定时任务进程入口，与 API 进程分开部署。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"goim-business/internal/app"
	"goim-business/internal/config"
	appcron "goim-business/internal/cron"
	filejob "goim-business/internal/job/file"
	imjob "goim-business/internal/job/im"
	"goim-business/internal/pkg/logger"
)

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "启动失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

	if cfg.IM.BaseURL == "" && !cfg.Storage.Enabled() {
		log.Info("IM 与对象存储均未配置，cron 无需运行")
		return nil
	}

	a, err := app.NewCronApp(context.Background(), cfg, log)
	if err != nil {
		return err
	}
	defer a.Close()

	scheduler := appcron.NewScheduler(a.Locker, a.LockPrefix, log)
	// 后续新增定时任务时在此注册；每个任务通过 Name 自动获得独立的分布式锁。
	if cfg.IM.BaseURL != "" {
		if err := scheduler.Register(imjob.NewOutboxJob(a.Worker, cfg.IM.OutboxScanInterval)); err != nil {
			return err
		}
	}
	if cfg.Storage.Enabled() {
		if err := scheduler.Register(filejob.NewCleanupJob(a.FileCleaner, cfg.Storage.CleanupInterval)); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("cron 启动", zap.String("lock_prefix", a.LockPrefix))
	scheduler.Run(ctx)
	return nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
