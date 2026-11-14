// Package main 是数据库迁移入口：仅做 up / down / version，不做自动迁移。
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"goim-business/internal/config"
	"goim-business/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "迁移失败: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("用法: migrate up|down|version")
	}

	env := os.Getenv("GOIM_BIZ_ENV")
	if env == "" {
		env = "dev"
	}
	cfg, err := config.Load(os.Getenv("GOIM_BIZ_CONFIG_DIR"), env)
	if err != nil {
		return err
	}

	source, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return fmt.Errorf("加载迁移文件失败: %w", err)
	}
	m, err := migrate.NewWithSourceInstance("iofs", source, migrateURL(cfg.MySQL.DSN))
	if err != nil {
		return fmt.Errorf("初始化迁移器失败: %w", err)
	}
	defer func() {
		if _, dbErr := m.Close(); dbErr != nil {
			fmt.Fprintf(os.Stderr, "关闭迁移器失败: %v\n", dbErr)
		}
	}()

	switch os.Args[1] {
	case "up":
		if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		fmt.Println("迁移已执行到最新版本")
	case "down":
		if err := m.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
			return err
		}
		fmt.Println("已回滚一个版本")
	case "version":
		v, dirty, err := m.Version()
		if err != nil {
			return err
		}
		fmt.Printf("当前版本: %d, dirty: %v\n", v, dirty)
	default:
		return fmt.Errorf("未知命令 %q，用法: migrate up|down|version", os.Args[1])
	}
	return nil
}

// migrateURL 把 gorm DSN 转成 golang-migrate 的 mysql URL，并补齐多语句执行能力。
func migrateURL(dsn string) string {
	url := dsn
	if !strings.HasPrefix(url, "mysql://") {
		url = "mysql://" + url
	}
	if strings.Contains(url, "multiStatements=") {
		return url
	}
	if strings.Contains(url, "?") {
		return url + "&multiStatements=true"
	}
	return url + "?multiStatements=true"
}
