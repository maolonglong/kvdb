package main

import (
	"log"
	"log/slog"
	gohttp "net/http"
	_ "net/http/pprof"
	"os"

	"github.com/ory/graceful"
	"github.com/samber/lo"

	"github.com/maolonglong/kvdb/internal/http"
	badgerstore "github.com/maolonglong/kvdb/internal/kv/badger"
)

func main() {
	store := lo.Must(badgerstore.New("./data"))
	defer store.Close()

	go func() {
		log.Println(gohttp.ListenAndServe("localhost:6060", nil))
	}()

	server := graceful.WithDefaults(&gohttp.Server{
		Addr:    "localhost:8080",
		Handler: http.NewHandler(store),
	})

	slog.Info("main: Starting the server")
	if err := graceful.Graceful(server.ListenAndServe, server.Shutdown); err != nil {
		store.Close()
		slog.Error("main: Failed to gracefully shutdown")
		os.Exit(1)
	}
	slog.Info("main: Server was shutdown gracefully")
}
