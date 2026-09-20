// Package dbtx 提供事务与 context 的绑定能力。
//
// 业务模块的仓储默认使用自身持有的 *gorm.DB; 当需要在一个事务内跨多个
// 模块(例如区域故障统一派工要同时写故障、维修记录与路灯状态)时,
// 由上层开启事务并把事务句柄放入 context, 仓储通过 Session 取到该事务,
// 从而保证跨模块写操作的原子性。
package dbtx

import (
	"context"

	"gorm.io/gorm"
)

type contextKey struct{}

// WithTx 返回事务句柄绑定后的 context。
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, tx)
}

// Session 返回应在当前请求中使用的数据库句柄: context 内有事务时用事务, 否则用默认连接。
func Session(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(contextKey{}).(*gorm.DB); ok && tx != nil {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

// InTransaction 在事务中执行 fn, 并把事务句柄写入传入 fn 的 context。
// fn 返回错误时事务回滚, 否则提交。
func InTransaction(ctx context.Context, db *gorm.DB, fn func(ctx context.Context) error) error {
	return db.Transaction(func(tx *gorm.DB) error {
		return fn(WithTx(ctx, tx))
	})
}
