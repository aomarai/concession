# HTTP API

Base path: `/api/v1` (except `/healthz`). All responses are JSON. Authenticated routes need the `session_token` cookie obtained by logging in.

## Errors

Every error uses the same shape:

```json
{ "error": { "code": "unauthorized", "message": "Unauthorized" } }
```

Codes in use: `bad_request` (400), `unauthorized` (401), `not_found` (404), `internal_error` (500), `unhealthy` (503).

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/healthz` | no | Liveness + database ping. `200 {"status":"ok"}` or `503` |
| GET | `/api/v1/auth/google/login` | no | Starts Google OAuth; sets `oauth_state` cookie and redirects to Google |
| GET | `/api/v1/auth/google/callback` | no | Completes login; sets `session_token` cookie and redirects to `/` |
| POST | `/api/v1/auth/logout` | no | Revokes the session and clears the cookie |
| GET | `/api/v1/me` | yes | The current user |

## CORS

Origins listed in `CORS_ALLOWED_ORIGINS` may make credentialed requests (cookies). Others get no CORS headers.

---
*Keep this file in sync with `setupRouter` in `backend/cmd/server/main.go`; every new endpoint ships with its entry here.*
