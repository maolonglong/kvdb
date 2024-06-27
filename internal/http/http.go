package http

import (
	"net/http"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/maolonglong/kvdb/internal/kv"
)

func NewHandler(store kv.Store) http.Handler {
	monkey := func(fn handleFunc) http.Handler {
		return handle(fn, store)
	}

	r := mux.NewRouter()

	r.Handle("/", monkey(createBucket)).Methods(http.MethodPost)
	r.Handle("/{bucket}", monkey(executeTxn)).Methods(http.MethodPost)
	r.Handle("/{bucket}/{key}", monkey(getKeyValue)).Methods(http.MethodGet)
	r.Handle("/{bucket}/{key}", monkey(setKeyValue)).Methods(http.MethodPost)
	r.Handle("/{bucket}/{key}", monkey(deleteKeyValue)).Methods(http.MethodDelete)
	r.Handle("/{bucket}/{key}", monkey(incrKeyValue)).Methods(http.MethodPatch)
	r.Handle("/{bucket}/_scripts/{name}", monkey(createScript)).Methods(http.MethodPost)
	r.Handle("/{bucket}/scripts/{name}", monkey(doScript)).Methods(http.MethodGet, http.MethodPost)

	h := handlers.CORS(
		handlers.AllowedHeaders([]string{"Content-Type", "Authorization"}),
		handlers.AllowedMethods([]string{
			http.MethodGet,
			http.MethodHead,
			http.MethodPut,
			http.MethodPatch,
			http.MethodPost,
			http.MethodDelete,
		}),
		handlers.AllowedOrigins([]string{"*"}),
		handlers.MaxAge(600),
	)(r)
	h = handlers.RecoveryHandler(handlers.PrintRecoveryStack(true))(h)
	return h
}
