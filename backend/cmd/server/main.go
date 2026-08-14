package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/handlers"
	"github.com/aomarai/concession/internal/logging"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func initDB(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	if cfg.DBDriver == "postgres" {
		slog.Info("Initializing database", "database", "PostgreSQL")
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBName, cfg.DBPort,
		)
		dialector = postgres.Open(dsn)
	} else {
		slog.Info("Initializing database", "database", "SQLite")
		dbURI := cfg.DBPath
		if dbURI == "" {
			dbURI = "concession.db"
		}
		dialector = sqlite.Open(dbURI)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	err = db.AutoMigrate(
		&domain.User{},
		&domain.OAuthAccount{},
		&domain.Movie{},
		&domain.Show{},
		&domain.Season{},
		&domain.Episode{},
		&domain.Genre{},
		&domain.Review{},
		&domain.Watchlist{},
		&domain.WatchlistItem{},
		&domain.Collaborator{},
		&domain.UserWatchProgress{},
		&domain.Session{},
		&domain.Notification{},
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func setupDB(cfg *config.Config, logger *slog.Logger) *gorm.DB {
	db, err := initDB(cfg)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}
	return db
}

func setupMux(authHandler *handlers.AuthHandler, userHandler *handlers.UserHandler, logger *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/me", authHandler.AuthenticateMiddleware(http.HandlerFunc(userHandler.HandleGetMe)))
	mux.HandleFunc("/api/v1/auth/google/login", authHandler.HandleGoogleLogin)
	mux.HandleFunc("/api/v1/auth/google/callback", authHandler.HandleGoogleCallback)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.HandleLogout)

	var rootHandler http.Handler = mux
	rootHandler = logging.RequestLoggerMiddleware(logger)(rootHandler)
	return rootHandler
}

func main() {
	ctx := context.Background()
	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.NewLogger(cfg)
	db := setupDB(cfg, logger)

	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)
	rootHandler := setupMux(authHandler, userHandler, logger)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	logger.Info("HTTP server starting", "port", port)
	if err := http.ListenAndServe(":"+port, rootHandler); err != nil {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
