package repository

import (
	"context"

	"gorm.io/gorm"
)

// TxManager 在 service 层提供显式事务边界；事务通过 context 透传给仓储。
type TxManager struct {
	db *gorm.DB
}

// NewTxManager 构造事务管理器。
func NewTxManager(db *gorm.DB) *TxManager {
	return &TxManager{db: db}
}

// WithTx 在一个数据库事务内执行 fn，fn 内拿到的 ctx 会让仓储自动使用该事务。
func (m *TxManager) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(withTx(ctx, tx))
	})
}

type txKey struct{}

func withTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}

// DBFromContext 优先复用事务，其次使用仓储持有的连接。
func DBFromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return fallback.WithContext(ctx)
}
