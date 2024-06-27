package badger

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	badger "github.com/dgraph-io/badger/v4"
	bopt "github.com/dgraph-io/badger/v4/options"
	"github.com/dgraph-io/ristretto/z"

	"github.com/maolonglong/kvdb/internal/kv"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

type Store struct {
	inner  *badger.DB
	closer *z.Closer
}

func New(path string) (kv.Store, error) {
	db, err := badger.Open(
		badger.DefaultOptions(path).
			// FIXME: If multiple connections are written at the same time,
			// it seems that there will always be conflicts.
			WithDetectConflicts(false).
			WithCompression(bopt.ZSTD).
			WithSyncWrites(false).
			WithBlockCacheSize(100 * (1 << 20)).
			WithIndexCacheSize(100 * (1 << 20)).
			WithZSTDCompressionLevel(3),
	)
	if err != nil {
		return nil, err
	}
	store := &Store{
		inner:  db,
		closer: z.NewCloser(1),
	}
	go store.gc()
	return store, nil
}

func (s *Store) Close() error {
	s.closer.SignalAndWait()
	return s.inner.Close()
}

func (s *Store) NewTransaction(update bool) any {
	return s.inner.NewTransaction(update)
}

func (s *Store) Discard(_txn any) {
	txn := _txn.(*badger.Txn)
	txn.Discard()
}

func (s *Store) Commit(_txn any) error {
	txn := _txn.(*badger.Txn)
	return txn.Commit()
}

func (s *Store) Get(ctx context.Context, _txn any, key []byte) ([]byte, error) {
	txn := _txn.(*badger.Txn)
	item, err := txn.Get(key)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return nil, kv.ErrKeyNotFound
		}
		return nil, err
	}
	return item.ValueCopy(nil)
}

func (s *Store) Set(ctx context.Context, _txn any, key, val []byte, opts *kv.SetOptions) error {
	txn := _txn.(*badger.Txn)

	var ttl time.Duration
	if opts != nil {
		ttl = opts.TTL
	}

	return s.setEntry(txn, key, val, ttl)
}

func (s *Store) Delete(ctx context.Context, _txn any, key []byte) error {
	txn := _txn.(*badger.Txn)
	return txn.Delete(key)
}

func (s *Store) Has(ctx context.Context, _txn any, key []byte) (bool, error) {
	txn := _txn.(*badger.Txn)
	item, err := txn.Get(key)
	if err != nil {
		if errors.Is(err, badger.ErrKeyNotFound) {
			return false, nil
		}
		return false, err
	}
	return item != nil, nil
}

func (s *Store) Incr(ctx context.Context, _txn any, key []byte, increment int64, opts *kv.SetOptions) (int64, error) {
	txn := _txn.(*badger.Txn)

	var (
		invalid bool
		ttl     time.Duration
		num     int64
	)
	if opts != nil {
		ttl = opts.TTL
	}

	prev, err := txn.Get(key)
	if err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
		return 0, err
	}
	if prev != nil {
		_ = prev.Value(func(val []byte) error {
			num, err = strconv.ParseInt(bytesconv.BytesToString(val), 10, 64)
			if err != nil {
				invalid = true
			}
			return nil
		})
	}

	if invalid {
		return 0, kv.ErrInvalidNum
	}

	num += increment
	val := strconv.FormatInt(num, 10)
	if err := s.setEntry(txn, key, bytesconv.StringToBytes(val), ttl); err != nil {
		return 0, err
	}

	return num, nil
}

func (s *Store) setEntry(txn *badger.Txn, key, val []byte, ttl time.Duration) error {
	ent := badger.NewEntry(key, val)
	if ttl > 0 {
		ent.ExpiresAt = uint64(time.Now().Add(ttl).Unix())
	}
	return txn.SetEntry(ent)
}

func (s *Store) gc() {
	defer func() {
		s.closer.Done()
		if err := recover(); err != nil {
			slog.Error("badger: gc goroutine panic", "err", err)
		}
	}()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-s.closer.HasBeenClosed():
			return
		case <-ticker.C:
		}
	again:
		err := s.inner.RunValueLogGC(0.7)
		if err == nil {
			goto again
		}
	}
}
