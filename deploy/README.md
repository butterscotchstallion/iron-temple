# deploy/ — database provisioning

Provisions the `iron-temple` tenant on the shared PostgreSQL 17 instance:
a dedicated login role (`iron_temple`) owning a single database (`iron_temple`),
with `CONNECT` revoked from `PUBLIC` for full tenant isolation.

The provisioning runs as a **Kubernetes Job** (`job.yaml`) that executes
`bootstrap.sql` via `psql`. It is **idempotent** — safe to re-run.

## Files
- `bootstrap.sql` — the idempotent provisioning SQL (source of truth).
- `job.yaml` — the Job that runs it, as the instance admin.
- `kustomization.yaml` — builds the ConfigMap from `bootstrap.sql`.

## Prerequisites
1. **Admin Secret** for the shared instance's superuser, with keys
   `username` and `password`. Set its name in `job.yaml` (`postgres-admin`)
   and the namespace in `kustomization.yaml`.
2. **Tenant Secret** holding the app role's password. Create it once — this is
   the same Secret the app's `DATABASE_URL` will reference, so there is a single
   source of truth:
   ```sh
   kubectl -n databases create secret generic iron-temple-db \
     --from-literal=password="$(openssl rand -base64 24)"
   ```
   Do **not** commit this Secret. (Use SealedSecrets / SOPS if you want it in git.)
3. Set `PGHOST`/`PGPORT` in `job.yaml` to your instance's service.

## Run
```sh
kubectl apply -k deploy/
kubectl -n databases wait --for=condition=complete job/iron-temple-bootstrap --timeout=120s
kubectl -n databases logs job/iron-temple-bootstrap
```

To re-provision after editing `bootstrap.sql`:
```sh
kubectl -n databases delete job iron-temple-bootstrap
kubectl apply -k deploy/
```

## App connection
The app connects as the tenant role (never the superuser):
```
DATABASE_URL=postgres://iron_temple:<password>@<PGHOST>:5432/iron_temple?sslmode=require
```
Assemble it in the app Deployment from the `iron-temple-db` Secret.

## Onboarding another tenant
Copy this folder, then find-and-replace the identifier `iron_temple` and the
tenant name `iron-temple` throughout. The isolation model (dedicated role +
single owned DB + `CONNECT` revoked from `PUBLIC`) is identical per tenant.

## Notes
- Password **rotation** is intentionally not handled by re-running the Job (the
  role is only created when absent). Rotate with `ALTER ROLE iron_temple PASSWORD ...`.
- Optional instance hardening (separate from this tenant): `REVOKE CONNECT ON
  DATABASE postgres FROM PUBLIC` so tenant roles can't reach the maintenance DB.
