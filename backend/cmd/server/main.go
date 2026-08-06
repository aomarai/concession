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
	"github.com/sethvargo/go-envconfig"
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

func main() {
	var slogHandler slog.Handler

	ctx := context.Background()
	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Could not load configuration")
		os.Exit(1)
	}

	if err := envconfig.Process(ctx, cfg); err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	if cfg.Environment == "prod" {
		slogHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	} else {
		slogHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	}

	logger := slog.New(slogHandler)
	slog.SetDefault(logger)

	db, err := initDB(cfg)
	if err != nil {
		logger.Error("Failed to initialize database", "error", err)
		os.Exit(1)
	}

	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)

	mux := http.NewServeMux()
	mux.Handle("/api/v1/me", authHandler.AuthenticateMiddleware(http.HandlerFunc(userHandler.HandleGetMe)))
	mux.HandleFunc("/api/v1/auth/google/login", authHandler.HandleGoogleLogin)
	mux.HandleFunc("/api/v1/auth/google/callback", authHandler.HandleGoogleCallback)
	mux.HandleFunc("/api/v1/auth/logout", authHandler.HandleLogout)

	// Example of a protected route:
	// mux.Handle("/api/v1/me", authHandler.AuthenticateMiddleware(http.HandlerFunc(someHandler)))

	var rootHandler http.Handler = mux
	rootHandler = logging.RequestLoggerMiddleware(logger)(rootHandler) // logging wraps everything

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
