# Concession

A movie and TV show watchlist app. Track what you want to watch, review what you've seen, and share lists with friends.

**Status:** under active development. The backend has Google login, a TMDB-backed catalog (search, movies, shows, seasons), watchlists (create, items, reorder) with owner/editor/viewer roles, per-user watch progress, reviews with ratings, collaboration (invites, roles, share links), friends, notifications and live updates (server-sent events); the web UI is planned. Progress is tracked in the GitHub issues (one per phase) — see [Roadmap](#roadmap).

## Stack

- **Backend:** Go, Gin, GORM (PostgreSQL in production, SQLite for local dev and tests)
- **Frontend:** React + TypeScript + Vite + Tailwind + React Query (Phase 5 in progress: sign-in, search, lists, title pages and reviews done; see `docs/DEVELOPMENT.md`)
- **Data:** movies and TV metadata from [TMDB](https://www.themoviedb.org/); movies are keyed by TMDB ID, shows by TVDB ID

## Quick start

```sh
docker compose up -d db            # local Postgres
cp backend/.env.example backend/.env   # then fill in Google OAuth + TMDB token
cd backend && go run ./cmd/server
curl localhost:8080/healthz
```

Full setup, configuration, and testing instructions: [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md).
HTTP API reference: [docs/API.md](docs/API.md).
Contributor/AI-agent conventions: [AGENTS.md](AGENTS.md).

## Roadmap

| Phase | Scope |
|---|---|
| 0 | Foundations: cleanup, CI, dev environment, coverage tracking |
| 1 | TMDB integration and title catalog |
| 2 | Watchlists and watch progress |
| 3 | Reviews and ratings |
| 4 | Sharing, collaboration, notifications, live updates |
| 5 | Frontend |
| 6 | Polish and deployment |

## License

See [LICENSE](LICENSE).
