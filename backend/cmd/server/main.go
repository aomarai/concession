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
	"github.com/sethvargo/go-envconfig"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func initDB(cfg *config.Config) (*gorm.DB, error) {
	var dialector gorm.Dialector

	dbDriver := cfg.DBDriver // "postgres" or default "sqlite"
	if dbDriver == "postgres" {
		slog.Info("Initializing database", "database", "PostgreSQL")
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			cfg.DBHost,
			cfg.DBUser,
			cfg.DBPassword,
			cfg.DBName,
			cfg.DBPort,
		)
		dialector = postgres.Open(dsn)
	} else {
		// Default to SQLite for dev
		slog.Info("Initializing database", "database", "SQLite")
		dbURI := cfg.DBPath
		if dbURI == "" {
			dbURI = "concession.db"
		}
		dialector = postgres.Open(dbURI)
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
	cfg := config.GetInstance()
	if err := envconfig.Process(ctx, &cfg); err != nil {
		slog.Error("Failed to load configuration", "error", err)
	}

	if cfg.Environment == "prod" {
		slogHandler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		slogHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	logger := slog.New(slogHandler)
	slog.SetDefault(logger)

	db, err := initDB(cfg)
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
	}

	jwtSecret := cfg.JWTSecret
	if jwtSecret == "" {
		slog.Warn("JWT_SECRET environment variable not set")
	}

	// Set up auth handler with GORM connection
	authHandler := handlers.NewAuthHandler(db, jwtSecret)

	// Set up HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/auth/google/login", authHandler.HandleGoogleLogin)
	//mux.HandleFunc("/api/v1/auth/google/callback", authHandler.HandleGoogleCallback) # TODO: implement HandleGoogleCallback

	// Start HTTP server
	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	slog.Info("HTTP server starting", "port", port)
}
