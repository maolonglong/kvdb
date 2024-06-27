package http

import (
	"errors"
	"io"
	"net/http"

	"github.com/gorilla/mux"
	lua "github.com/yuin/gopher-lua"

	"github.com/maolonglong/kvdb/internal/kv"
	"github.com/maolonglong/kvdb/pkg/bytesconv"
)

var createScript = withBucket(func(_ http.ResponseWriter, r *http.Request, d *data) (int, error) {
	vars := mux.Vars(r)
	name := vars["name"]

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return http.StatusInternalServerError, err
	}

	if err := d.bucket.StoreScript(r.Context(), bytesconv.StringToBytes(name), body); err != nil {
		return http.StatusInternalServerError, err
	}
	return 0, nil
})

var doScript = withBucket(func(w http.ResponseWriter, r *http.Request, d *data) (int, error) {
	vars := mux.Vars(r)
	name := vars["name"]

	if err := d.bucket.DoScript(r.Context(), w, r, bytesconv.StringToBytes(name)); err != nil {
		if errors.Is(err, kv.ErrKeyNotFound) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("script not found"))
			return 0, nil
		}
		var luaErr *lua.ApiError
		if errors.As(err, &luaErr) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(luaErr.Object.String()))
			return 0, nil
		}
		return http.StatusInternalServerError, err
	}
	return 0, nil
})
