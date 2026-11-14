package repository

import (
	"context"
	"database/sql"
	"log"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"goim-business/internal/config"
)

// NewDB 创建 gorm 连接并完成连接池与连通性检查。
func NewDB(ctx context.Context, cfg config.MySQLConfig, logger *zap.Logger) (*gorm.DB, *sql.DB, error) {
	gormLog := gormlogger.New(
		log.New(&zapWriter{log: logger}, "", 0),
		gormlogger.Config{
			SlowThreshold:             cfg.SlowThreshold,
			LogLevel:                  gormlogger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	)

	db, err := gorm.Open(mysql.Open(cfg.DSN), &gorm.Config{
		PrepareStmt:            true,
		SkipDefaultTransaction: true,
		TranslateError:         true,
		Logger:                 gormLog,
	})
	if err != nil {
		return nil, nil, err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, err
	}
	return db, sqlDB, nil
}

// zapWriter 把 gorm 的 SQL 日志转交给 zap（仅慢查询与错误会进入 Warn 级别）。
type zapWriter struct {
	log *zap.Logger
}

func (w *zapWriter) Write(p []byte) (int, error) {
	msg := strings.TrimSpace(string(p))
	if msg != "" {
		w.log.Warn("gorm 慢查询或执行错误", zap.String("sql", msg))
	}
	return len(p), nil
}
