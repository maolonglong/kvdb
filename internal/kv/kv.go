package kv

import (
	"context"
	"time"
)

type Store interface {
	NewTransaction(update bool) any
	Discard(txn any)
	Commit(txn any) error

	Get(ctx context.Context, txn any, key []byte) ([]byte, error)
	Has(ctx context.Context, txn any, key []byte) (bool, error)

	Set(ctx context.Context, txn any, key, val []byte, opts *SetOptions) error
	Delete(ctx context.Context, txn any, key []byte) error
	Incr(
		ctx context.Context,
		txn any,
		key []byte,
		increment int64,
		opts *SetOptions,
	) (int64, error)

	Close() error
}

type SetOptions struct {
	TTL time.Duration
}

func WithTxn(store Store, update bool, fn func(txn any) error) error {
	txn := store.NewTransaction(update)
	defer store.Discard(txn)

	if err := fn(txn); err != nil {
		return err
	}

	return store.Commit(txn)
}
