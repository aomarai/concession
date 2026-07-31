package main

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/handlers"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func initDB() (*gorm.DB, error) {
	var dialector gorm.Dialector

	dbDriver := os.Getenv("DB_DRIVER") // "postgres" or default "sqlite"
	if dbDriver == "postgres" {
		slog.Info("Initializing database", "database", "PostgreSQL")
		dsn := fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
			os.Getenv("DB_HOST"),
			os.Getenv("DB_USER"),
			os.Getenv("DB_PASSWORD"),
			os.Getenv("DB_NAME"),
			os.Getenv("DB_PORT"),
		)
		dialector = postgres.Open(dsn)
	} else {
		// Default to SQLite for dev
		slog.Info("Initializing database", "database", "SQLite")
		dbURI := os.Getenv("DB_PATH")
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
		&domain.RefreshToken{},
		&domain.Notification{},
	)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func main() {
	db, err := initDB()
	if err != nil {
		slog.Error("Failed to initialize database", "error", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
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
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	slog.Info("HTTP server starting", "port", port)
}
