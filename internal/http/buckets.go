package http

import (
	"net/http"
	"time"

	"github.com/spf13/cast"

	"github.com/maolonglong/kvdb/internal/core"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

func createBucket(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	if err := r.ParseForm(); err != nil {
		return http.StatusBadRequest, err
	}

	opts := &core.BucketOptions{
		SecretKey:  r.PostForm.Get("secret_key"),
		ReadKey:    r.PostForm.Get("read_key"),
		WriteKey:   r.PostForm.Get("write_key"),
		SigningKey: r.PostForm.Get("signing_key"),
		DefaultTTL: time.Duration(cast.ToInt64(r.PostForm.Get("default_ttl"))) * time.Second,
	}
	b, err := core.NewBucket(r.Context(), d.store, opts)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	w.Write(bytesconv.StringToBytes(b.Name()))
	return 0, nil
}
