# Development

## Prerequisites

Go (version in `backend/go.mod`), Docker (for local Postgres), and Node 22 once the frontend lands. The `.devcontainer/` provides all of these.

## Running the backend

```sh
docker compose up -d db
cp backend/.env.example backend/.env
cd backend && go run ./cmd/server
```

With no `DB_DRIVER` set the server uses SQLite (`DB_PATH`, default `concession.db`), so Docker is optional for quick experiments. Schema is created with GORM `AutoMigrate` on startup. AutoMigrate never rewrites existing rows, so any change that needs data fixed up (for example the `movies`/`shows` → `movie`/`show` review type rename) goes in `migrate()` in `cmd/server/main.go` as an idempotent step with a test.

### Configuration

All settings are environment variables (a `backend/.env` file is loaded if present; it is gitignored). See `backend/.env.example` for the full list.

| Variable | Purpose |
|---|---|
| `DB_DRIVER` | `sqlite` (default) or `postgres` |
| `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` `DB_NAME` | Postgres connection (`DB_PORT` defaults to `5432`, also when set but empty) |
| `DB_SSLMODE` | Postgres `sslmode`; defaults to `disable` (fine for the local compose database). Use `verify-full` (or at least `require`) for a database reached over a network |
| `DB_PATH` | SQLite file (default `concession.db`) |
| `PORT` | Listen port (default `8080`) |
| `COOKIE_SECURE` | Defaults to `true`; set `false` for plain-http local dev |
| `COOKIE_SAME_SITE` | `lax` (default), `strict`, `none` (requires `COOKIE_SECURE=true`) |
| `GOOGLE_CLIENT_ID` `GOOGLE_CLIENT_SECRET` `GOOGLE_REDIRECT_URL` | Google OAuth login |
| `RATE_LIMIT_AUTH_PER_MINUTE` | Sign-in routes per client IP (default `30`) |
| `RATE_LIMIT_IP_PER_MINUTE` | Authenticated routes per client IP, charged before the session lookup (default `1200`) |
| `RATE_LIMIT_API_PER_MINUTE` | All authenticated routes per user (default `600`) |
| `RATE_LIMIT_FRIEND_REQUESTS_PER_HOUR` | Friend requests sent per user (default `30`) |
| `RATE_LIMIT_INVITES_PER_HOUR` | List invitations sent per user (default `60`) |
| `MAX_EVENT_STREAMS_PER_USER` | Concurrent live-update streams per user (default `10`) |
| `TRUSTED_PROXIES` | Comma-separated IPs/CIDRs of reverse proxies whose `X-Forwarded-For` is believed when finding the client IP. Empty (default) trusts none. **Behind a proxy set this**, otherwise every visitor shares the proxy's address and the per-IP sign-in limit applies to all of them at once; trust only proxies you control or anyone can spoof their address. An explicit `0` for any limit above switches it off; a negative value is rejected at startup |
| `CORS_ALLOWED_ORIGINS` | Comma-separated browser origins (default `http://localhost:5173`) |
| `TMDB_READ_ACCESS_TOKEN` | TMDB v4 "API Read Access Token" (bearer). Without it the catalog endpoints fail with 502 and the server logs a warning |
| `TMDB_BASE_URL` | Override the TMDB API URL (tests/proxies); default `https://api.themoviedb.org/3` |

Get a TMDB token at <https://www.themoviedb.org/settings/api> (use the "API Read Access Token", not the v3 API key). Genres are synced from TMDB at startup when a token is set.

Never commit secrets; keep them in `backend/.env` or your environment.

## Project layout

See [AGENTS.md](../AGENTS.md#layout).

## Testing

```sh
cd backend
go vet ./...
go test ./...
./scripts/coverage.sh        # tests with -race + coverage, enforces the ratchet
go tool cover -html=coverage.out   # browse uncovered lines
```

Conventions:

- Tests use in-memory SQLite via `internal/testutil.NewDB`; never call real external APIs (use `httptest` servers).
- To test database failure paths, use `testutil.FailOn(t, db, "create"|"query"|"update"|"delete"|"row", "<table>")` (`"row"` covers `Scan`/`Row` queries), or `testutil.FailAfter(..., n)` to let the first n calls succeed. Table names are GORM's (e.g. `o_auth_accounts`).
- Tests that run goroutines against the database use `testutil.NewFileDB` (WAL file); shared in-memory SQLite fails concurrent cross-table access with "table is locked".
- Randomness sources are package variables (`randRead`) so entropy failures can be simulated.
- New code should land with tests; the goal is ~100% statement coverage.

### Postgres-backed tests

The unit tests run on in-memory SQLite, which creates no foreign keys. Tests that depend on Postgres enforcing them (account deletion, `TestPurgeUserOnPostgres`) run only when `TEST_POSTGRES_DSN` is set; CI sets it against a Postgres service. Locally:

```sh
docker compose up -d db
cd backend && TEST_POSTGRES_DSN="host=localhost user=concession password=concession dbname=concession port=5432 sslmode=disable" go test ./internal/domain -run Postgres
```

The test works in a private schema it drops afterwards, so it is safe to point at the dev database.

### Coverage ratchet

`backend/.coverage-threshold` holds the minimum total coverage. CI runs `scripts/coverage.sh --summary`, which fails the build below that number and prints per-function gaps in the job summary. When coverage improves, raise the threshold in the same PR; never lower it to get a PR green. Coverage status per phase is tracked in the "Test coverage tracker" GitHub issue.

Intentionally uncovered code (kept minimal): `main()` (a one-line `os.Exit(execute(nil))`) and the `Serve` failure branch in `run`, which cannot be triggered once the listener is bound.

## Running everything in containers

```sh
cp .env.example .env      # fill in GOOGLE_CLIENT_ID/SECRET and TMDB_READ_ACCESS_TOKEN
docker compose --profile app up --build
```

Open http://localhost:8080 (`APP_PORT` changes the port). Three services start: Postgres (`db`), the Go API (`backend`, built from `backend/Dockerfile`) and `frontend` (nginx, built from `frontend/Dockerfile`), which serves the UI and proxies `/api` and `/healthz` to the API, so the browser sees a single origin. Only the frontend port is published. The frontend and backend share a small `edge` network with a fixed subnet (`APP_SUBNET`, default `172.29.250.0/24`) and the frontend has a fixed address on it (`FRONTEND_IP`, default `172.29.250.10`); the backend trusts `X-Forwarded-For` from that address only (`TRUSTED_PROXIES`), so a client cannot forge its IP for the per-IP rate limits. The network is created only with `--profile app`; if the subnet overlaps another Docker network on your machine, change the two variables. Behind your own TLS proxy or load balancer, add its address to `TRUSTED_PROXIES` too.

- **Sign-in:** Google OAuth or a local password. Create an account with `POST /api/v1/auth/register` (e-mail, username, display name, password) or from the sign-in page; sign in with `POST /api/v1/auth/login` using an e-mail address or a username. Passwords are stored as bcrypt hashes (cost 12), must be at least 12 characters, and are never returned by the API. `POST /api/v1/me/password` lets a signed-in person set or change one, which is how a Google-only account gains a local credential.
- **Not implemented yet:** password reset and e-mail verification — both need a mailer, so a local account's e-mail is unverified. A Google sign-in never auto-links to an existing password account with the same address (without verification that would be an account-takeover path); such a person is told to sign in with their password.
- **Google OAuth:** create an OAuth client and add `http://localhost:8080/api/v1/auth/google/callback` (matching `GOOGLE_REDIRECT_URL`) as an authorized redirect URI. Without credentials the app starts (and password sign-in works) but Google sign-in cannot complete.
- **Cookies:** `COOKIE_SECURE=false` is for plain `http://localhost`. Behind HTTPS set it to `true` and put the public URL in `GOOGLE_REDIRECT_URL` and `CORS_ALLOWED_ORIGINS`.
- **Reverse proxies in front of the frontend container** must not buffer `/api/` (the event streams) and should allow long reads; the bundled nginx config already does (`frontend/nginx.conf.template`, `BACKEND_URL` selects the upstream).
- `docker compose up -d` with no profile still starts only Postgres, for running the API and frontend from source.
- The `docker` CI job builds both images, starts the stack and smoke-tests `/healthz`, the SPA fallback and the API through the frontend container.

## Frontend

```sh
cd frontend
npm ci
npm run dev        # http://localhost:5173, proxies /api to $BACKEND_URL (default http://localhost:8080)
npm test           # Vitest + Testing Library, enforces 100% coverage
npm run lint && npm run build
npm run tokens     # regenerate src/styles/tokens.css after editing design/tokens.json
```

The UI follows the Concession design system (published as a Design System artifact; its tokens are `design/tokens.json`). Colours, shadows and the two themes (Matinee, the default, and Late show, followed from `prefers-color-scheme`) come from `src/styles/tokens.css`, generated from that file: edit the JSON, run `npm run tokens`, never edit the CSS by hand. Component styles live in the `@layer components` block of `src/index.css` (the design system's `bundle.css` is derived from it). Fonts (Instrument Serif, Gabarito) load from Google Fonts in `index.html`.

Motion is spring physics in `src/lib/spring.ts`: `usePop({ entry, delay, hover, tap })` makes an element pop from `scale(0.9)` past `1.08` to `1` on a stiffness 350 / damping 18 / mass 0.8 spring, writing only `transform` (give the element the `pop` class). It does nothing under `prefers-reduced-motion`. Buttons and nav items pop on hover and press; ticket cards, list cards and rows pop in on entry, staggered.

Brand files are in `design/brand/` (VectorCraft source `concession-brand.vectorcraft` and its SVG exports); `public/` holds the favicon, touch icon, manifest icons and `manifest.webmanifest` made from them. `design/concession.figcraft` is the first FigCraft prototype and predates the current card style.

Routes: `/` (your lists, paged), `/lists/:id` (owners and editors reorder items with the arrow buttons, edit notes and remove items; owners also edit list settings and delete the list), `/search` (the add-to-list picker offers your first 100 lists), `/invites` (accept or decline invitations), `/profile` (your account with "Edit profile" for display name and username, and "Delete account", which asks you to type your username or e-mail and then signs you out; plus your tracked titles, paged and filterable by status, and your reviews), `/friends` (friend list and requests, each paged, add by username or e-mail), `/notifications` (paged inbox, mark read; the nav shows an unread badge refreshed every minute), `/shared/:token` (read-only list behind a share link), `/movies/:tmdbId` and `/shows/:tmdbId` (title details with your watch status — and season/episode for shows — and reviews). The title page recognizes your own review by author ID within the loaded page of reviews (20 per page); if you have reviewed a title but your review is on a later page, posting again returns the backend's `409`.

The list page has a Sharing panel: the owner invites people by username or e-mail, changes roles, removes members, cancels pending invitations and copies or rotates the share link (only offered while the list is Shared or Public); collaborators can leave. Share links point at the frontend route `/shared/:token`, so a production host must serve `index.html` for unknown paths (SPA fallback).

Notification links arrive from the API as backend-style paths (`/watchlists/:id`, `/invites`, `/friends`); `src/lib/links.ts` maps those to frontend routes and ignores anything else.

The list page listens to the list'"'"'s event stream (`useListEvents`, an `EventSource` on `/api/v1/watchlists/:id/events`) and refetches when a collaborator changes something; if the list is deleted it returns to the lists page. Events are not replayed, so a reconnect also refetches. Behind a reverse proxy the stream must not be buffered (see "Live updates" below).

Sign in with Google through the backend (`/api/v1/auth/google/login`); because the dev server proxies `/api`, the session cookie stays same-origin. Tests mock `src/api/endpoints` and never touch a network. Write the test first, then the component.

## Live updates (server-sent events)

`internal/events` is an in-process hub; services publish events and `GET /api/v1/watchlists/:id/events` streams them. Try it with `curl -N -b "session_token=..." localhost:8080/api/v1/watchlists/<id>/events`. Because streams never finish by themselves, the server closes the hub at shutdown (`server.RegisterOnShutdown`); a test in `cmd/server` guards that. Tests that read a stream use `httptest.NewServer` and a client that parses `event:`/`data:` lines (see `internal/handlers/events_test.go`).

## CI

`.github/workflows/ci.yml` runs on every PR and push to `main`: gofmt check, `go vet`, tests with race detector and the coverage gate; plus the frontend lint/build/test job once `frontend/package.json` exists.

`.github/workflows/publish.yml` builds both images and pushes them to the GitHub Container Registry as `ghcr.io/aomarai/concession/<backend|frontend>`, tagged with the full commit SHA (and `latest`). It runs on pushes to `main`, on `v*` tags, on `deploy/*` branches (so an unmerged commit can be deployed for testing) and on manual dispatch. A deployment pins the SHA tag:

```sh
docker pull ghcr.io/aomarai/concession/backend:<commit-sha>
```

Packages inherit the repository's visibility when they are first published; if a cluster pulls them anonymously the package must be public.

## Kubernetes

`deploy/helm/concession` is a Helm chart for the API and the frontend (see its README for values and an example install). Postgres is not bundled. The chart was written without a Helm binary, so it has not been linted or rendered; run `helm lint` and `helm template` before relying on it. Set `backend.image.repository` / `frontend.image.repository` to the published images above and `image.tag` to a commit SHA. Hosting it on the homelab cluster is done from the `aomarai/homelab` GitOps repo (`apps/concession/`), which keeps the namespace, Postgres and routing manifests next to the Application.
