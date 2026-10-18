# Concession Helm chart

Deploys the API and the frontend (nginx, which serves the SPA and proxies `/api`). Postgres is **not** included: point `database.*` at an existing or managed database.

> This chart was written without a Helm binary available, so it has not been linted or rendered. Run `helm lint deploy/helm/concession` and `helm template` with your values before the first install, and report anything that fails.

```sh
helm install concession deploy/helm/concession \
  --set host=concession.example.com \
  --set database.host=my-postgres.example.com \
  --set secrets.dbPassword=... \
  --set secrets.googleClientId=... --set secrets.googleClientSecret=... \
  --set secrets.tmdbReadAccessToken=... \
  --set ingress.enabled=true --set ingress.tlsSecretName=concession-tls
```

- Add `https://<host>/api/v1/auth/google/callback` as an authorized redirect URI in the Google console.
- Prefer `existingSecret` (keys `DB_PASSWORD`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `TMDB_READ_ACCESS_TOKEN`) over `secrets.*`, which land in Helm's release history.
- Set `trustedProxies` to the pod network so per-IP rate limits see real client addresses; rate limits are per backend process, so more replicas multiply them.
- Without TLS in front, set `tls=false` (cookies are then not `Secure`).
