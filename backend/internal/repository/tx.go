package repository

import (
	"context"

	"gorm.io/gorm"
)

type txContextKey struct{}

// WithTransaction runs fn inside a single database transaction. Repositories
// that resolve their handle through DBFromContext join the ambient transaction
// instead of opening their own, so a cross-aggregate workflow (proof
// acceptance driving a batch) is atomic: any failure rolls back every write,
// including audit entries.
func WithTransaction(ctx context.Context, db *gorm.DB, fn func(context.Context) error) error {
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok {
		// Already inside a transaction (SavePoint semantics are unnecessary for
		// the current call graph) — reuse it so nested calls share the commit.
		return fn(context.WithValue(ctx, txContextKey{}, tx))
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txContextKey{}, tx))
	})
}

// DBFromContext returns the ambient transaction handle when one is active,
// otherwise the repository's own database handle.
func DBFromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txContextKey{}).(*gorm.DB); ok {
		return tx.WithContext(ctx)
	}
	return db.WithContext(ctx)
}

// TxFrom returns the ambient transaction handle. It must only be called inside
// a WithTransaction callback, where the handle is guaranteed to be present.
func TxFrom(ctx context.Context) *gorm.DB {
	return ctx.Value(txContextKey{}).(*gorm.DB)
}
