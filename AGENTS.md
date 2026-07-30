# Conception Project — Agents Guide

This document provides guidance for AI assistants working on the **Conception** codebase: a collaborative movie and TV show watchlist website built with Go (Gin framework) backend, React frontend, and PostgreSQL database. Use it to understand context, conventions, patterns, and tooling available in this project.

---

## 📌 Project Overview

A web application enabling users to:
- Track desired movies/TV shows via TMDB API integration  
- Write reviews for tracked titles  
- Share watchlists with friends/family through real-time database collaboration  
- Manage multiple lists (want-to-watch, currently-watching, completed)

---

## 🗂️ Project Layout (Monorepo Structure)

```
concession/
├── .devcontainer/         # Docker dev environment configuration
│   ├── devcontainer.json  # VS Code container settings  
│   └── Dockerfile         # Container image definition
├── backend/               # Go API service (`github.com/aomarai/conception`)
│   ├── cmd/internal/domain/      # Domain models with GORM tags (movie.go, show.go)
│   │   ├── movie.go             # Movie entity mapping to movies table  
│   │   └── show.go              # TV Show entity for shows table
│   ├── handlers/           # HTTP request handlers using Gin framework
│   ├── services/          # Business logic layer
│   ├── router.go          // Routes and middleware definitions 
│   └── main.go            // Entry point with graceful shutdown handling  
├── frontend/              # Vite + React TypeScript app (@vitejs/plugin-react)
│   ├── src/components/ui/  # Reusable UI component library (Tailwind CSS)  
│   ├── hooks/             # Custom React Query hooks for API calls
│   └── ...                
├── .gitignore            
├── AGENTS.md             
└── README.md             

```

---

## 🛠️ Tech Stack Overview

### Backend — Go (Gin Framework) with PostgreSQL (`go 1.26`)

| Component | Purpose / Usage Pattern |
|-----------|------------------------|
| **gin-gonic/gin v1.12** | HTTP framework for REST API endpoints; handlers in `/backend/cmd/internal/handlers/` |
| **gorm.io/gorm + gorm.io/driver/postgres** | PostgreSQL ORM with `db.AutoMigrate()` support using GORM struct tags (`movie.go`, `show.go`) |  
| **bytedance/gopkg/sonic v1.15.2** | Fast JSON serialization for high-throughput TMDB API responses (indirect dependency) |  

### Frontend — Vite + React 18+ + TypeScript

- Functional components with TSX types defined to mirror Go struct fields via `json` tags  
- `@tanstack/react-query` (`react-query`) for async movie/show data fetching and watchlist mutations (CRUD)
- Tailwind CSS utility classes preferred over custom stylesheets  

### Database — PostgreSQL

Postgres instance managed by Docker Compose or external CI/CD pipeline. Connection string read from environment variables in `.devcontainer/devcontainer.json`.

---

## 📝 Go Backend Development Guidelines

### 1. Project Structure Standards
```go
// backend/cmd/internal/domain/     // Entities with GORM struct tags  
type Movie struct {
    ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
    Title     string    `json:"title,omitempty"`                       
}

// HTTP routes and middleware definitions        
func initRouter() gin.IRouter {
    r := gin.Default()
    
    apiV1 := r.Group("/api/v1")
    movies, _ := handlers.RouteMovies(apiV1) // Group of handler functions
    
    return &r, apiV1, movies
}

```

### 2. Code Conventions & Guidelines

**Error Handling**: Use explicit error checks (`if err != nil`) throughout public API layers; wrap errors using `fmt.Errorf("%w", ...)` before returning to React clients via JSON responses with semantic status codes (400/401/403/404).

**RESTful Routes (Gin)**:
- GET `/api/movies` → List all movies/shows  
- GET `/api/movies/:id` → Get single item by TMDB ID or internal ID
- PUT/PATCH `/api/movies/:id` → Update title/date metadata 
- DELETE `/api/movies/:id` → Remove from watchlist database

**Gin Framework Patterns**: Define routes and middleware in `/backend/cmd/internal/router.go`; accept `context.Context` through handler/service layers to enable request cancellation/deadline enforcement across goroutines spawned during TMDB API calls; Set CORS headers appropriately (allow local dev origins: `http://localhost:5173`, etc.).

**Database Migrations**: Use GORM auto-migration with `db.AutoMigrate()` on service startup OR manual migrations via SQL files when preferred; ensure DB state matches models defined under `/backend/cmd/internal/domain/`. Pass database and query context through all handler/service layers.

### 3. Gin Framework Configuration Example
```go
// backend/cmd/internal/router.go
package internal
  
func SetupRoutes() gin.IRouter {
    r := gin.Default() // or setup custom recovery/middleware
    
    apiV1 := r.Group("/api/v1") // API version prefix 
      
    db.DB.AutoMigrate(&Movie{}, &Show{})
    
    return apiV1, movies
}

```

### 4. Graceful Shutdown Example (Kubernetes-ready)
```go
// main.go entrypoint with SIGTERM handling  
func main() {
    ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
    
    // Initialize database and handlers after signals are registered
    
    defer func(ctx context.Context) { 
        <-ctx.Done(); if err == nil { http.Server.Shutdown(ctx) }

```

### 5. Structured Logging Pattern (slog JSON)
```go
import "log/slog"

// Example log output with structured fields:
logger.Info("Movie created", "title", movie.Title, "id", movie.ID)
// Output: {"level":"INFO","msg":"Movie created","time":1719624000,"title":"Inception","id":3}

```

---

## 🐳 Dev Environment Configuration

Add these keys to `.devcontainer/devcontainer.json` for CI/CD:
```yaml
env:
  DATABASE_HOST: "${POSTGRES_DB}"     
  DATABASE_PORT: "5432"                
  POSTGRES_USER: ${USER}               
  POSTGRES_PASSWORD: ${SECRET_VALUE_FROM_ENV_FILE_LATER?}  
  API_LISTEN_ADDR: ":8080"          
  
```

Also add these to `docker-compose.yml`: database service (`image: postgres`) + Go container orchestration using GORM models in migration scripts. Ensure all containers listen on `--host 0.0.0.0` for cross-interface access during development.

---

## 🚫 Do NOT Modify Without Explicit Confirmation:

- Files in `.devcontainer/`, `.github/actions/*` unless explicitly told otherwise