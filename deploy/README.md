# deploy/ — database provisioning and backups

Provisions the `iron-temple` tenant on the shared PostgreSQL 17 instance:
a dedicated login role (`iron_temple`) owning a single database (`iron_temple`),
with `CONNECT` revoked from `PUBLIC` for full tenant isolation.

The provisioning runs as a **Kubernetes Job** (`job.yaml`) that executes
`bootstrap.sql` via `psql`. It is **idempotent** — safe to re-run. A **CronJob**
(`backup.yaml`) then dumps that database nightly; see [Backups](#backups).

## Files
- `bootstrap.sql` — the idempotent provisioning SQL (source of truth).
- `job.yaml` — the Job that runs it, as the instance admin.
- `backup.yaml` — the nightly `pg_dump` CronJob and the PVC it writes to.
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

## Backups

`backup.yaml` adds a **CronJob** that runs `pg_dump` at **03:17 UTC** daily and
keeps the last **14** dumps on a 2Gi **PVC** (`iron-temple-backups`). It connects
as the tenant role, not the admin — a backup reads one database, so it is taken
with credentials that can reach exactly one.

Before applying it, set `PGHOST` in `backup.yaml` the same way you set it in
`job.yaml`, and uncomment `storageClassName` if your cluster has no usable
default. Retention is the `KEEP` env var; the schedule is `spec.schedule`.

Each run writes to `<name>.dump.partial`, moves it into place only after
`pg_dump` exits clean, and then verifies the archive is readable with
`pg_restore --list` before rotating anything out. A dump interrupted halfway
must not be left in the directory looking exactly like a good one, and rotation
must never delete a good backup on the strength of a bad one.

```sh
# Is it running?
kubectl -n databases get cronjob iron-temple-backup
kubectl -n databases logs -l app.kubernetes.io/component=backup --tail=50

# Take one right now
kubectl -n databases create job --from=cronjob/iron-temple-backup backup-manual
```

### Restoring

The dumps are in the custom format, so `pg_restore` — not `psql`. From a pod
with the PVC mounted:

```sh
# What is on the volume
kubectl -n databases exec deploy/<any-pod-with-the-pvc> -- ls -lh /backups

# Restore into a FRESH database, then repoint the app at it. Restoring over a
# live one is how a bad backup becomes a bad database.
pg_restore --no-owner --no-privileges \
  --dbname="postgres://iron_temple:<password>@<PGHOST>:5432/iron_temple_restored" \
  /backups/iron_temple-<stamp>.dump
```

`--no-owner --no-privileges` at restore, not at dump: the archive keeps what it
found, and whether to reapply ownership is a decision for the database you are
restoring *into*. Scale the app to zero first if you are going to swap
databases under it.

**This is an in-cluster copy.** It survives a deleted database, a bad migration
and a fat-fingered `DELETE`. It does not survive losing the cluster or the
storage behind it — for that, copy the PVC somewhere else on a schedule of your
own. The app also offers each lifter `GET /me/export`, which is their account as
JSON and readable without Postgres at all; the two are different tools and
neither replaces the other.

## Onboarding another tenant
Copy this folder, then find-and-replace the identifier `iron_temple` and the
tenant name `iron-temple` throughout. The isolation model (dedicated role +
single owned DB + `CONNECT` revoked from `PUBLIC`) is identical per tenant.

## Notes
- Password **rotation** is intentionally not handled by re-running the Job (the
  role is only created when absent). Rotate with `ALTER ROLE iron_temple PASSWORD ...`.
- Optional instance hardening (separate from this tenant): `REVOKE CONNECT ON
  DATABASE postgres FROM PUBLIC` so tenant roles can't reach the maintenance DB.
