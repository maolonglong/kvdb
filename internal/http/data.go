package http

import (
	"log"
	"net/http"
	"strconv"

	"github.com/tomasen/realip"

	"github.com/maolonglong/kvdb/internal/core"
	"github.com/maolonglong/kvdb/internal/kv"
)

type handleFunc func(w http.ResponseWriter, r *http.Request, d *data) (int, error)

type data struct {
	store  kv.Store
	bucket *core.Bucket
}

func handle(fn handleFunc, store kv.Store) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status, err := fn(w, r, &data{
			store: store,
		})

		if status >= 400 || err != nil {
			clientIP := realip.FromRequest(r)
			log.Printf("%s: %v %s %v", r.URL.Path, status, clientIP, err)
		}

		if status != 0 {
			txt := http.StatusText(status)
			http.Error(w, strconv.Itoa(status)+" "+txt, status)
			return
		}
	})
}
