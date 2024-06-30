package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/cast"

	"github.com/maolonglong/kvdb/internal/core"
	"github.com/maolonglong/kvdb/internal/kv"
	"github.com/maolonglong/kvdb/internal/model/request"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

func withBucket(next handleFunc) handleFunc {
	return func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
		vars := mux.Vars(r)
		bucket, err := core.LoadBucket(r.Context(), d.store, vars["bucket"])
		if err != nil {
			if errors.Is(err, kv.ErrKeyNotFound) {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte("bucket not found"))
				return 0, nil
			}
			return http.StatusInternalServerError, err
		}
		d.bucket = bucket
		return next(w, r, d)
	}
}

var setKeyValue = withBucket(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
	vars := mux.Vars(r)

	val, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	ttl := time.Duration(cast.ToInt64(r.URL.Query().Get("ttl"))) * time.Second
	// FIXME: handle error
	_ = d.bucket.Set(r.Context(), bytesconv.StringToBytes(vars["key"]), val, ttl)
	return 0, nil
})

var deleteKeyValue = withBucket(
	func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
		vars := mux.Vars(r)
		if err := d.bucket.Delete(r.Context(), bytesconv.StringToBytes(vars["key"])); err != nil {
			return http.StatusInternalServerError, nil
		}
		return 0, nil
	},
)

var getKeyValue = withBucket(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	vars := mux.Vars(r)
	val, err := d.bucket.Get(r.Context(), bytesconv.StringToBytes(vars["key"]))
	if err != nil {
		if errors.Is(err, kv.ErrKeyNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("key not found"))
			return 0, nil
		}
		return http.StatusInternalServerError, err
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	_, _ = w.Write(val)
	return 0, nil
})

var executeTxn = withBucket(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	var req request.ExecuteTransactionRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return http.StatusBadRequest, err
	}

	var ops []*core.Operation
	for _, item := range req.Txn {
		if item.Set != nil && item.Value != nil && item.TTL != nil && item.Delete == nil {
			ops = append(ops, &core.Operation{
				Data: &core.OpSet{
					Key:   bytesconv.StringToBytes(*item.Set),
					Value: bytesconv.StringToBytes(*item.Value),
					TTL:   time.Duration(*item.TTL) * time.Second,
				},
				Type: core.OpTypeSet,
			})
		} else if item.Set == nil && item.Value == nil && item.TTL == nil && item.Delete != nil {
			ops = append(ops, &core.Operation{
				Data: &core.OpDelete{
					Key: bytesconv.StringToBytes(*item.Delete),
				},
				Type: core.OpTypeDelete,
			})
		} else {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("invalid operation"))
			return 0, nil
		}
	}

	if err := d.bucket.ApplyTxn(r.Context(), ops); err != nil {
		if errors.Is(err, kv.ErrTxnTooBig) {
			w.WriteHeader(http.StatusRequestEntityTooLarge)
			_, _ = w.Write([]byte("txn too big"))
			return 0, nil
		}
		return http.StatusInternalServerError, err
	}
	return 0, nil
})

var incrKeyValue = withBucket(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	vars := mux.Vars(r)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	if len(body) < 2 || (body[0] != '-' && body[0] != '+') {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid body"))
		return 0, nil
	}

	increment, err := strconv.ParseInt(bytesconv.BytesToString(body[1:]), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid num"))
		return 0, nil
	}

	key := bytesconv.StringToBytes(vars["key"])
	ttl := time.Duration(cast.ToInt64(r.URL.Query().Get("ttl"))) * time.Second
	val, err := d.bucket.Incr(r.Context(), key, increment, ttl)
	if err != nil {
		if errors.Is(err, kv.ErrInvalidNum) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(fmt.Sprintf("key `%s` is not a number", key)))
			return 0, nil
		}
		return http.StatusInternalServerError, err
	}
	_, _ = w.Write(bytesconv.StringToBytes(strconv.FormatInt(val, 10)))
	return 0, nil
})
