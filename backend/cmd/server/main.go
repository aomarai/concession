package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/handlers"
	"github.com/aomarai/concession/internal/logging"
	"github.com/gin-gonic/gin"
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

func setupRouter(authHandler *handlers.AuthHandler, userHandler *handlers.UserHandler, logger *slog.Logger) *gin.Engine {
	r := gin.Default()

	// Global logging middleware
	r.Use(logging.GinRequestLoggerMiddleware(logger))

	apiV1 := r.Group("/api/v1")

	// Public routes
	apiV1.GET("/auth/google/login", authHandler.HandleGoogleLogin)
	apiV1.GET("/auth/google/callback", authHandler.HandleGoogleCallback)
	apiV1.POST("/auth/logout", authHandler.HandleLogout)

	// Authenticated routes
	auth := apiV1.Group("/")
	auth.Use(authHandler.AuthMiddleware())
	auth.GET("/me", userHandler.HandleGetMe)

	return r
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg, err := config.Load(ctx)
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	logger := logging.NewLogger(cfg)
	db := setupDB(cfg, logger)

	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)
	engine := setupRouter(authHandler, userHandler, logger)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}

	logger.Info("HTTP server starting", "port", port)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: engine,
	}

	// Graceful shutdown: use Shutdown() so in-flight requests can complete
	// before the server exits, reducing client-visible errors on deployment.
	go func() {
		<-ctx.Done()
		logger.Info("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("server shutdown error", "error", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("HTTP server stopped", "error", err)
		os.Exit(1)
	}
}
