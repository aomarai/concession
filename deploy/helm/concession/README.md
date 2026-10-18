# Concession Helm chart

Deploys the API and the frontend (nginx, which serves the SPA and proxies `/api`). Postgres is **not** included: point `database.*` at an existing or managed database.

> This chart was written without a Helm binary available, so it has not been linted or rendered. Run `helm lint deploy/helm/concession` and `helm template` with your values before the first install, and report anything that fails.

```sh
helm install concession deploy/helm/concession \
  --set host=concession.example.com \
  --set backend.image.repository=REGISTRY/concession-backend --set backend.image.tag=TAG \
  --set frontend.image.repository=REGISTRY/concession-frontend --set frontend.image.tag=TAG \
  --set database.host=my-postgres.example.com \
  --set secrets.dbPassword=... \
  --set secrets.googleClientId=... --set secrets.googleClientSecret=... \
  --set secrets.tmdbReadAccessToken=... \
  --set ingress.enabled=true --set ingress.tlsSecretName=concession-tls
```

- Add `https://<host>/api/v1/auth/google/callback` as an authorized redirect URI in the Google console.
- Prefer `existingSecret` (keys `DB_PASSWORD`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `TMDB_READ_ACCESS_TOKEN`) over `secrets.*`, which land in Helm's release history.
- Per-IP rate limits need the real client address. `trustedProxies` must list every hop that sets `X-Forwarded-For`: the frontend pods' network **and** your ingress controller or load balancer. Prefer the narrowest ranges you can; any workload inside a trusted range can claim any client IP.
- Keep `backend.replicas` at 1 unless you accept that live updates and rate limits are per process (a change handled by one replica is not streamed from another) and that every replica migrates the schema at start (scale up after the first is running).
- Images need an explicit tag (`backend.image.tag`, `frontend.image.tag`); use an immutable tag or digest per build so an upgrade rolls out. After rotating an `existingSecret`, restart the backend (`kubectl rollout restart`) or bump `backend.podAnnotations`.
- Database TLS defaults to `require` (encrypted, certificate not verified). Use `verify-full` where the server certificate chains to a root the image trusts.
- Without TLS in front, set `tls=false` (cookies are then not `Secure`).
