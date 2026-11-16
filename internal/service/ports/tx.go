package ports

import "context"

// TxManager 提供显式事务边界，事务通过 context 透传给仓储实现。
type TxManager interface {
	WithTx(ctx context.Context, fn func(ctx context.Context) error) error
}
