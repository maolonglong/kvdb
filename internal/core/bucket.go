package core

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/jaevor/go-nanoid"
	"github.com/samber/lo"
	lua "github.com/yuin/gopher-lua"

	"github.com/maolonglong/kvdb/internal/kv"
	"github.com/maolonglong/kvdb/internal/pool"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

const _bucketNameLen = 21

const (
	_markBucketOpts = ":opts"
	_markKeyValue   = ":kv:"
	_markScripts    = ":scripts:"
)

var idgen = lo.Must(nanoid.Standard(_bucketNameLen))

type OpType uint8

const (
	_ OpType = iota
	OpTypeSet
	OpTypeDelete
)

type Operation struct {
	Data any
	Type OpType
}

type OpSet struct {
	Key   []byte
	Value []byte
	TTL   time.Duration
}

type OpDelete struct {
	Key []byte
}

type BucketOptions struct {
	// TODO: To implement authorization, I am not sure how to do authorization
	// properly when there is a script, because the script can access all keys by default.

	// Manage bucket policy and other keys.
	SecretKey string

	// Prevent public reads from the bucket.
	ReadKey string

	// Prevent public writes to the bucket.
	WriteKey string

	// Enable access token generation.
	SigningKey string

	// Keys not updated expire after this duration.
	DefaultTTL time.Duration
}

type Bucket struct {
	store kv.Store
	opts  *BucketOptions
	name  string
}

func NewBucket(ctx context.Context, store kv.Store, opts *BucketOptions) (*Bucket, error) {
	b := &Bucket{
		store: store,
		opts:  opts,
		name:  idgen(),
	}

	if err := b.storeOpts(ctx); err != nil {
		return nil, err
	}

	// TODO: save email info?

	return b, nil
}

// TODO: LUR
func LoadBucket(ctx context.Context, store kv.Store, name string) (*Bucket, error) {
	b := &Bucket{
		store: store,
		name:  name,
		opts:  &BucketOptions{},
	}
	if err := b.loadOpts(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *Bucket) Name() string {
	return b.name
}

func (b *Bucket) Set(ctx context.Context, key, val []byte, ttl time.Duration) error {
	uKey := b.udataKey(key, _markKeyValue)
	opts := &kv.SetOptions{
		TTL: b.opts.DefaultTTL,
	}
	if ttl > 0 {
		opts.TTL = ttl
	}
	return kv.WithTxn(b.store, true, func(txn any) error {
		return b.store.Set(ctx, txn, uKey, val, opts)
	})
}

func (b *Bucket) Incr(ctx context.Context, key []byte, increment int64, ttl time.Duration) (int64, error) {
	uKey := b.udataKey(key, _markKeyValue)
	opts := &kv.SetOptions{
		TTL: b.opts.DefaultTTL,
	}
	if ttl > 0 {
		opts.TTL = ttl
	}
	var num int64
	err := kv.WithTxn(b.store, true, func(txn any) error {
		var err error
		num, err = b.store.Incr(ctx, txn, uKey, increment, opts)
		return err
	})
	return num, err
}

func (b *Bucket) Get(ctx context.Context, key []byte) ([]byte, error) {
	uKey := b.udataKey(key, _markKeyValue)
	var val []byte
	err := kv.WithTxn(b.store, false, func(txn any) error {
		var err error
		val, err = b.store.Get(ctx, txn, uKey)
		return err
	})
	return val, err
}

func (b *Bucket) Delete(ctx context.Context, key []byte) error {
	uKey := b.udataKey(key, _markKeyValue)
	return kv.WithTxn(b.store, true, func(txn any) error {
		return b.store.Delete(ctx, txn, uKey)
	})
}

func (b *Bucket) ApplyTxn(ctx context.Context, ops []*Operation) error {
	if len(ops) == 0 {
		return nil
	}
	return kv.WithTxn(b.store, true, func(txn any) error {
		for _, op := range ops {
			switch op.Type {
			case OpTypeSet:
				d := op.Data.(*OpSet)
				uKey := b.udataKey(d.Key, _markKeyValue)
				opts := &kv.SetOptions{
					TTL: b.opts.DefaultTTL,
				}
				if d.TTL > 0 {
					opts.TTL = d.TTL
				}
				if err := b.store.Set(ctx, txn, uKey, d.Value, opts); err != nil {
					return err
				}
			case OpTypeDelete:
				d := op.Data.(*OpDelete)
				uKey := b.udataKey(d.Key, _markKeyValue)
				if err := b.store.Delete(ctx, txn, uKey); err != nil {
					return err
				}
			default:
			}
		}
		return nil
	})
}

func (b *Bucket) StoreScript(ctx context.Context, name, content []byte) error {
	key := b.udataKey(name, _markScripts)
	return kv.WithTxn(b.store, true, func(txn any) error {
		return b.store.Set(ctx, txn, key, content, nil)
	})
}

func (b *Bucket) DoScript(ctx context.Context, w http.ResponseWriter, r *http.Request, name []byte) error {
	script, err := b.loadScript(ctx, name)
	if err != nil {
		return err
	}

	l := pool.GetLState()
	defer func() {
		l.Pop(l.GetTop())
		pool.PutLState(l)
	}()
	buf := pool.GetByteBuffer()
	defer pool.PutByteBuffer(buf)

	txn := b.store.NewTransaction(true)
	defer b.store.Discard(txn)

	mod := mkLua(buf, r, b, txn, l)
	l.SetGlobal("kvdb", mod)

	var exitCode int
	if err := l.DoString(bytesconv.BytesToString(script)); err != nil {
		var luaErr *lua.ApiError
		if !errors.As(err, &luaErr) {
			return err
		}
		tb, ok := luaErr.Object.(*lua.LTable)
		if !ok {
			return err
		}
		reason, ok := tb.RawGetString(_kvdbExitReason).(lua.LString)
		if !ok {
			return err
		}

		switch reason {
		case "redirect":
			tb := tb.RawGetString(_kvdbExitPayload).(*lua.LTable)
			url := tb.RawGetString("url").(lua.LString)
			code := tb.RawGetString("code").(lua.LNumber)
			http.Redirect(w, r, string(url), int(code))
			return nil
		case "exit":
			code := tb.RawGetString(_kvdbExitPayload).(lua.LNumber)
			exitCode = int(code)
		default:
			return err
		}
	} else {
		if err := b.store.Commit(txn); err != nil {
			return err
		}
	}

	if header, ok := mod.RawGetString("header").(*lua.LTable); ok {
		header.ForEach(func(l1, l2 lua.LValue) {
			k, ok := l1.(lua.LString)
			v, ok2 := l2.(lua.LString)
			if ok && ok2 {
				w.Header().Set(string(k), string(v))
			}
		})
	}
	if exitCode > 0 {
		w.WriteHeader(exitCode)
	} else if code, ok := mod.RawGetString("status").(lua.LNumber); ok {
		w.WriteHeader(int(code))
	}

	buf.WriteTo(w)
	return nil
}

func (b *Bucket) loadScript(ctx context.Context, name []byte) ([]byte, error) {
	key := b.udataKey(name, _markScripts)
	var val []byte
	err := kv.WithTxn(b.store, false, func(txn any) error {
		var err error
		val, err = b.store.Get(ctx, txn, key)
		return err
	})
	return val, err
}

func (b *Bucket) storeOpts(ctx context.Context) error {
	key := bytesconv.StringToBytes(b.name + _markBucketOpts)
	val, err := json.Marshal(b.opts)
	if err != nil {
		return err
	}
	return kv.WithTxn(b.store, true, func(txn any) error {
		return b.store.Set(ctx, txn, key, val, nil)
	})
}

func (b *Bucket) udataKey(origin []byte, mark string) []byte {
	buf := pool.GetByteBuffer()
	defer pool.PutByteBuffer(buf)

	// <bucket_name>:udata:<key>
	buf.WriteString(b.name)
	buf.WriteString(mark)
	buf.Write(origin)

	return bytes.Clone(buf.Bytes())
}

func (b *Bucket) loadOpts(ctx context.Context) error {
	key := bytesconv.StringToBytes(b.name + _markBucketOpts)
	var val []byte
	err := kv.WithTxn(b.store, false, func(txn any) error {
		var err error
		val, err = b.store.Get(ctx, txn, key)
		return err
	})
	if err != nil {
		return err
	}
	return json.Unmarshal(val, b.opts)
}
