// Package main 是数据库迁移入口：仅做 up / down / version，不做自动迁移。
package main

import (
	"errors"
	"flag"
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
	configPath := "etc/dev.yaml"
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.StringVar(&configPath, "c", configPath, "配置文件路径（YAML），env 由文件内 env 字段声明")
	fs.StringVar(&configPath, "config", configPath, "配置文件路径，等同 -c")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "用法: migrate [-c 配置文件] up|down|version\n")
		fs.PrintDefaults()
	}
	if err := fs.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	args := fs.Args()
	if len(args) < 1 {
		fs.Usage()
		return fmt.Errorf("缺少子命令")
	}

	cfg, err := config.LoadFile(configPath)
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

	switch args[0] {
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
		return fmt.Errorf("未知命令 %q，用法: migrate [-c 配置文件] up|down|version", args[0])
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
