package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"rentalatlas/internal/atlas"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	directory := env("DATA_DIR", "data")
	maps, err := atlas.NewMaps(directory)
	if err != nil {
		log.Fatal(err)
	}
	store, err := atlas.OpenStore(ctx, directory, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer store.DB.Close()
	if err = atlas.Seed(ctx, store, maps); err != nil {
		log.Fatal(err)
	}
	addr := env("ADDR", "127.0.0.1:8080")
	host, _, _ := net.SplitHostPort(addr)
	publish := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if v, ok := os.LookupEnv("ALLOW_PUBLISH"); ok {
		publish = v == "true"
	}
	app := &atlas.Server{Store: store, Maps: maps, StaticDir: env("STATIC_DIR", "dist"), AllowPublish: publish}
	server := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		<-ctx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 8*time.Second)
		defer stop()
		_ = server.Shutdown(shutdownCtx)
	}()
	log.Printf("租房版图 http://%s · publishing=%t", addr, publish)
	if err = server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
