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

	"github.com/joho/godotenv"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
func main() {
	// Backend credentials are separate from Vite's client configuration. Existing environment wins.
	if err := godotenv.Load(".env.server.local"); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Fatal("后端本地环境配置格式错误")
	}
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
	auth, err := atlas.NewAuth(ctx, store, directory)
	if err != nil {
		log.Fatal("账号服务初始化失败，请检查后端配置")
	}
	if err = atlas.Seed(ctx, store, maps); err != nil {
		log.Fatal(err)
	}
	addr := env("ADDR", "127.0.0.1:8080")
	host, _, _ := net.SplitHostPort(addr)
	publish := host == "127.0.0.1" || host == "localhost" || host == "::1"
	if v, ok := os.LookupEnv("ALLOW_PUBLISH"); ok {
		publish = v == "true"
	}
	metro, err := atlas.NewMetro(directory)
	if err != nil {
		log.Fatal("地铁城市索引不存在或格式错误，请检查 data/metro/cities.json")
	}
	app := &atlas.Server{Store: store, Maps: maps, StaticDir: env("STATIC_DIR", "dist"), AllowPublish: publish, Auth: auth, Metro: metro}
	server := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 90 * time.Second, WriteTimeout: 120 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
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
