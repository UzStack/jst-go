package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/example/goapp/docs"
	"github.com/example/goapp/internal/server"
	"github.com/example/goapp/internal/shared/config"
	"github.com/example/goapp/internal/shared/database"
	"github.com/example/goapp/internal/shared/logger"
)

// @title           jst-go API
// @version         1.0
// @description     Go clean architecture template — gin + pgx + sqlc + JWT auth.
// @termsOfService  http://swagger.io/terms/
//
// @contact.name   JscorpTech
// @contact.url    https://github.com/JscorpTech/jst-go
// @contact.email  admin@jscorp.uz
//
// @license.name  MIT
//
// @host      localhost:8080
// @BasePath  /api/v1
//
// @securityDefinitions.apikey BearerAuth
// @in                         header
// @name                       Authorization
// @description                Type "Bearer <access_token>" in the Value field.

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic("config load: " + err.Error())
	}

	log, err := logger.New(cfg.Env, cfg.Log.Level)
	if err != nil {
		panic("logger init: " + err.Error())
	}
	defer func() { _ = log.Sync() }()

	rootCtx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := database.NewPool(rootCtx, cfg.DB)
	if err != nil {
		log.Fatal("db connect failed", logger.Err(err))
	}
	defer pool.Close()

	if err := database.MigrateUp(cfg.DB.MigrationsURL(), cfg.DB.MigrationsPath); err != nil {
		log.Fatal("migration failed", logger.Err(err))
	}

	srv := server.New(cfg, log, pool)

	httpServer := &http.Server{
		Addr:              cfg.HTTP.Addr(),
		Handler:           srv.Router(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("http server starting", logger.String("addr", httpServer.Addr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	select {
	case err := <-serverErr:
		log.Error("http server error", logger.Err(err))
	case <-rootCtx.Done():
		log.Info("shutdown signal received")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), cfg.HTTP.ShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", logger.Err(err))
		os.Exit(1)
	}

	log.Info("server stopped cleanly")
}
