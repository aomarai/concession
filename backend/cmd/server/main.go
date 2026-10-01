package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aomarai/concession/internal/catalog"
	"github.com/aomarai/concession/internal/config"
	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/handlers"
	"github.com/aomarai/concession/internal/logging"
	"github.com/aomarai/concession/internal/progress"
	"github.com/aomarai/concession/internal/reviews"
	"github.com/aomarai/concession/internal/tmdb"
	"github.com/aomarai/concession/internal/watchlist"
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

	if err := migrate(db); err != nil {
		return nil, err
	}
	return db, nil
}

// apiHandlers groups the HTTP handlers mounted by setupRouter.
type apiHandlers struct {
	Auth      *handlers.AuthHandler
	User      *handlers.UserHandler
	Catalog   *handlers.CatalogHandler
	Watchlist *handlers.WatchlistHandler
	Collab    *handlers.CollaborationHandler
	Progress  *handlers.ProgressHandler
	Reviews   *handlers.ReviewHandler
}

func setupRouter(db *gorm.DB, cfg *config.Config, h apiHandlers, logger *slog.Logger) *gin.Engine {
	// Use gin.New() instead of gin.Default() to avoid Gin's built-in logger
	// middleware producing duplicate request logs alongside GinRequestLoggerMiddleware.
	// We explicitly add only the recovery middleware and our structured logger.
	r := gin.New()
	// Recover from panics, and answer unknown routes/methods, in the same JSON
	// error shape as every other API error.
	r.Use(gin.CustomRecoveryWithWriter(gin.DefaultErrorWriter, func(c *gin.Context, recovered any) {
		logger.Error("panic recovered", "panic", recovered)
		handlers.RespondError(c, http.StatusInternalServerError, "internal_error", "Internal error")
	}))
	r.HandleMethodNotAllowed = true
	r.NoRoute(func(c *gin.Context) {
		handlers.RespondError(c, http.StatusNotFound, "not_found", "Not found")
	})
	r.NoMethod(func(c *gin.Context) {
		handlers.RespondError(c, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed")
	})
	r.Use(logging.GinRequestLoggerMiddleware(logger))
	r.Use(handlers.CORSMiddleware(cfg.CORSAllowedOrigins))

	r.GET("/healthz", handlers.HealthHandler(db))

	apiV1 := r.Group("/api/v1")

	// Public routes
	apiV1.GET("/auth/google/login", h.Auth.HandleGoogleLogin)
	apiV1.GET("/auth/google/callback", h.Auth.HandleGoogleCallback)
	apiV1.POST("/auth/logout", h.Auth.HandleLogout)

	// Authenticated routes
	auth := apiV1.Group("/")
	auth.Use(h.Auth.AuthMiddleware())
	auth.GET("/me", h.User.HandleGetMe)
	h.Catalog.RegisterRoutes(auth)
	h.Watchlist.RegisterRoutes(auth)
	h.Collab.RegisterRoutes(auth)
	h.Progress.RegisterRoutes(auth)
	h.Reviews.RegisterRoutes(auth)

	return r
}

// migrate creates or updates the schema and fixes up legacy row data.
func migrate(db *gorm.DB) error {
	err := db.AutoMigrate(
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
		return err
	}
	return normalizeReviewTypes(db)
}

// normalizeReviewTypes rewrites the plural reviewable_type values ("movies",
// "shows") that older versions of the polymorphic tags used to the singular
// ReviewableItem values ("movie", "show"). AutoMigrate changes the schema but
// never touches row data, so without this such reviews would be invisible to
// the Movie.Reviews / Show.Reviews associations. It is idempotent.
func normalizeReviewTypes(db *gorm.DB) error {
	for old, current := range map[string]domain.ReviewableItem{"movies": domain.ReviewableMovies, "shows": domain.ReviewableShows} {
		err := db.Unscoped().Model(&domain.Review{}).Where("reviewable_type = ?", old).
			UpdateColumn("reviewable_type", current).Error
		if err != nil {
			return fmt.Errorf("normalize review types: %w", err)
		}
	}
	return nil
}

// connStateHook, when set, is installed as the HTTP server's ConnState
// callback. Tests use it to wait for a connection to become active instead of
// sleeping.
var connStateHook func(net.Conn, http.ConnState)

// shutdownTimeout bounds how long in-flight requests get to finish on shutdown.
var shutdownTimeout = 30 * time.Second

func main() {
	os.Exit(execute(nil))
}

// execute runs the server until SIGINT/SIGTERM and returns the process exit code.
func execute(ready func(net.Addr)) int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, ready); err != nil {
		slog.Error("server failed", "error", err)
		return 1
	}
	return 0
}

// closeDB releases the database connection pool on shutdown.
func closeDB(db *gorm.DB) {
	if sqlDB, err := db.DB(); err == nil {
		_ = sqlDB.Close()
	}
}

// run wires up config, database, and routes, then serves until ctx is
// cancelled. If ready is non-nil it is called with the listening address once
// the server is accepting connections (used by tests with PORT=0).
func run(ctx context.Context, ready func(net.Addr)) error {
	cfg, err := config.Load(ctx)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	logger := logging.NewLogger(cfg)
	db, err := initDB(cfg)
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer closeDB(db)

	authHandler := handlers.NewAuthHandler(db, cfg)
	userHandler := handlers.NewUserHandler(db)
	if cfg.TMDBReadAccessToken == "" {
		logger.Warn("TMDB_READ_ACCESS_TOKEN is not set; catalog endpoints will fail")
	}
	catalogSvc := catalog.NewService(db, tmdb.NewClient(cfg.TMDBReadAccessToken, tmdb.WithBaseURL(cfg.TMDBBaseURL)))
	if cfg.TMDBReadAccessToken != "" {
		go func() {
			if err := catalogSvc.SyncGenres(ctx); err != nil {
				logger.Warn("genre sync failed", "error", err)
			}
		}()
	}
	watchlistSvc := watchlist.NewService(db, catalogSvc)
	engine := setupRouter(db, cfg, apiHandlers{
		Auth:      authHandler,
		User:      userHandler,
		Catalog:   handlers.NewCatalogHandler(catalogSvc),
		Watchlist: handlers.NewWatchlistHandler(watchlistSvc),
		Collab:    handlers.NewCollaborationHandler(watchlistSvc),
		Progress:  handlers.NewProgressHandler(progress.NewService(db, catalogSvc)),
		Reviews:   handlers.NewReviewHandler(reviews.NewService(db, catalogSvc)),
	}, logger)

	port := cfg.Port
	if port == "" {
		port = "8080"
	}
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("HTTP server starting", "addr", ln.Addr().String())
	if ready != nil {
		ready(ln.Addr())
	}

	server := &http.Server{Handler: engine, ReadHeaderTimeout: 10 * time.Second, ConnState: connStateHook}

	// Graceful shutdown: use Shutdown() so in-flight requests can complete
	// before the server exits, reducing client-visible errors on deployment.
	shutdownErr := make(chan error, 1)
	go func() {
		<-ctx.Done()
		logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		shutdownErr <- server.Shutdown(shutdownCtx)
	}()

	if err := server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	return <-shutdownErr
}
