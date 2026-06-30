# Moderation Rule Production Rollout Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to execute this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This is a manual production runbook; do not dispatch subagents or automate production mutations.

**Goal:** Deploy the completed moderation-rule foundation, management API, and admin frontend to the existing single-instance production environment with one planned outage and one irreversible schema migration.

**Architecture:** Use a manual maintenance window. Stop public access and the old backend, take a full database backup, run the migration exactly once, then deploy the backend before the admin frontend through the existing GitHub Actions workflows. No compatibility columns, rolling deployment, automatic database migration, or multi-instance coordination are added.

**Tech Stack:** Go 1.25, MySQL, Docker Compose, GitHub Actions, Garage object storage, React/Vite admin static files, BaoTa-managed production services.

## Global Constraints

- Execute only after all tasks in the foundation, management API, and admin frontend plans are complete and verified.
- Production is one backend instance; temporary page unavailability is accepted.
- Do not retain `moderation_rule.enabled`, `moderation_rule.ruleset_version`, or the obsolete snapshot index for rollback compatibility.
- The production migration is `migrations/20260630_moderation_rule_management.sql`; never run it with MySQL `--force`.
- If any migration statement fails, restore the full pre-migration database backup before retrying. MySQL DDL is not treated as an all-or-nothing transaction.
- The database backup is the only supported rollback across the schema change. An old image alone is not a valid rollback after migration.
- Keep the public and admin sites in maintenance until the new backend and admin smoke tests pass.
- Deploy the backend first. Do not deploy the new admin frontend against the old backend API.
- Use exact commit-SHA images/artifacts produced by the existing workflows; do not deploy a floating `latest` tag manually.
- Do not import the full production corpus until a small import, candidate test, and publish cycle succeeds.
- A failed or canceled import must leave the current published ruleset serving traffic.

## Release Inputs

Record these values in the release ticket or private operator notes before starting. Do not commit secrets or backup locations to this repository.

```text
BACKEND_RELEASE_SHA=
FRONTEND_RELEASE_SHA=
PREVIOUS_BACKEND_IMAGE=
PREVIOUS_ADMIN_BACKUP=
DATABASE_BACKUP=
DATABASE_BACKUP_DIR=
REMOTE_USER=
REMOTE_HOST=
DEPLOY_DIR=/opt/blog-backend
DEPLOY_ADMIN_ROOT=
SERVER_PORT=8080
COMPOSE_CONTAINER_NAME=blog-server
DB_HOST=
DB_PORT=3306
DB_NAME=
DB_USER=
```

Blank values are mandatory operator inputs. Read deployment directories and remote connection values from the GitHub production Environment; read database values from the private production `.env`; obtain the previous image with `docker inspect`; and obtain backup paths only after the backup operations succeed. Enter the database password only at the interactive MySQL prompt.

---

### Task 1: Freeze and Verify the Release Candidates

**Files:**
- Verify: `docs/superpowers/plans/2026-06-30-moderation-rule-index-foundation.md`
- Verify: `docs/superpowers/plans/2026-06-30-moderation-rule-management-api.md`
- Verify: `docs/superpowers/plans/2026-06-30-moderation-rule-admin-frontend.md`
- Verify: `migrations/20260630_moderation_rule_management.sql`
- Verify: `config/config.prod.yaml`
- Verify: `docs/moderation-rollout.md`

**Interfaces:**
- Consumes: the completed backend and frontend release candidates.
- Produces: two immutable release SHAs and a reviewed production configuration.

- [ ] **Step 1: Verify the backend candidate from the backend repository**

```bash
go test ./... -count=1
go test -race ./internal/service/moderation/... ./internal/service/moderationrule ./internal/repository/moderationrule -count=1
go build ./cmd/server
git status --short
git rev-parse HEAD
```

Expected: all tests and the build pass; `git status --short` is empty; record `git rev-parse HEAD` as `BACKEND_RELEASE_SHA`.

- [ ] **Step 2: Verify the frontend candidate from `../blog-frontend`**

```bash
pnpm --filter @repo/api test
pnpm --filter admin test
pnpm --filter @repo/api check-types
pnpm --filter admin check-types
pnpm --filter @repo/api lint
pnpm --filter admin lint
pnpm --filter admin build
git status --short
git rev-parse HEAD
```

Expected: all commands pass. The previously known unrelated `apps/web` working-tree changes must be resolved or explicitly excluded before selecting the release commit. Record the committed release as `FRONTEND_RELEASE_SHA`.

- [ ] **Step 3: Review production moderation configuration**

Confirm `config/config.prod.yaml` enables moderation and contains positive rule limits. Confirm `max_build_peak_memory_mb` is greater than `max_index_memory_mb`; neither threshold may exceed available production memory after reserving memory for the Go process, MySQL, Redis, Docker, and the operating system.

For the initial approximately 100,000-keyword rollout, use the measured benchmark report in `docs/moderation-rule-index-benchmark.md`. Do not raise limits merely to make an oversized import pass.

- [ ] **Step 4: Verify production prerequisites without changing production**

Confirm all of the following:

- The Garage endpoint and bucket in production `.env` are reachable with read, write, and delete permissions.
- The production deployment directory contains `.env`, `docker-compose.yml`, and `config/`.
- GitHub production Secrets and Variables required by both repositories are present.
- The backend and frontend release SHAs have passed their staging workflows.
- Enough disk space exists for the database backup, index artifacts, import files, and Docker image.

Expected: every prerequisite is confirmed before a maintenance window is announced.

### Task 2: Rehearse the One-Way Migration

**Files:**
- Execute against a non-production copy: `migrations/20260630_moderation_rule_management.sql`
- Verify with: `docs/moderation-rollout.md`

**Interfaces:**
- Consumes: a recent schema-compatible copy of production data.
- Produces: evidence that the migration and new backend startup succeed before production downtime.

- [ ] **Step 1: Restore a recent production backup into the staging database**

Use the existing BaoTa/MySQL restore operation. Ensure the staging backend is stopped while restoring so it cannot write to the copied database.

Expected: the staging copy contains the same moderation schema shape as production.

- [ ] **Step 2: Execute the migration once against staging**

```bash
mysql --show-warnings \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --user="$DB_USER" \
  --password \
  "$DB_NAME" < migrations/20260630_moderation_rule_management.sql
```

Expected: exit code 0 and no failed statement. Do not use `--force`.

- [ ] **Step 3: Verify the migrated staging schema**

```sql
SELECT id, status, rule_count, keyword_count, regexp_count, composite_count
FROM moderation_ruleset
ORDER BY id;

SELECT id, name
FROM moderation_rule_source
ORDER BY id;

SHOW COLUMNS FROM moderation_rule LIKE 'enabled';
SHOW COLUMNS FROM moderation_rule LIKE 'ruleset_version';
SHOW COLUMNS FROM moderation_rule LIKE 'activated_ruleset_id';
SHOW COLUMNS FROM moderation_revision LIKE 'rule_matches_truncated';
SHOW COLUMNS FROM moderation_attempt LIKE 'rule_matches_truncated';
```

Expected: ruleset ID 1 is published; source ID 1 exists; the two removed columns return no rows; all three new columns exist.

- [ ] **Step 4: Start the new backend against staging and run smoke tests**

```bash
curl --fail --silent --show-error "http://127.0.0.1:${SERVER_PORT:-8080}/health"
```

Then sign in to staging admin and verify rule status, template download, text testing, a small import, candidate publishing, and restart recovery.

Expected: the backend starts by loading or rebuilding ruleset 1; health and all management smoke tests pass.

### Task 3: Enter Maintenance and Create Recovery Assets

**Files:**
- Back up privately: production MySQL database, production `.env`, production config, current admin static directory.
- Record privately: current backend image reference and both release SHAs.

**Interfaces:**
- Consumes: the verified release candidates and rehearsed migration.
- Produces: a stopped production write path and complete recovery assets.

- [ ] **Step 1: Put the public and admin sites into maintenance**

Use the existing BaoTa site controls to suspend both sites or serve the existing maintenance response. Confirm anonymous users and administrators can no longer submit writes.

Expected: public and admin writes are unavailable before the backend is stopped.

- [ ] **Step 2: Stop the production backend**

Run on the production host:

```bash
cd "${DEPLOY_DIR:-/opt/blog-backend}"
docker compose stop blog-server
docker compose ps blog-server
```

Expected: `blog-server` is stopped.

- [ ] **Step 3: Create and verify a full production database backup**

Use BaoTa's full MySQL backup operation after the backend stops. Record the backup identifier and download or copy it to storage outside the database host.

Expected: BaoTa reports a successful backup and the backup artifact has a non-zero size.

- [ ] **Step 4: Back up runtime configuration and admin files**

Run on the production host, storing the archives in the operator's private backup directory:

```bash
cp "${DEPLOY_DIR:-/opt/blog-backend}/.env" "$DATABASE_BACKUP_DIR/backend.env"
tar -czf "$DATABASE_BACKUP_DIR/backend-config.tar.gz" -C "${DEPLOY_DIR:-/opt/blog-backend}" config docker-compose.yml
tar -czf "$DATABASE_BACKUP_DIR/admin-static.tar.gz" -C "$DEPLOY_ADMIN_ROOT" .
docker inspect --format '{{.Config.Image}}' "${COMPOSE_CONTAINER_NAME:-blog-server}"
```

Expected: all three files exist with non-zero size; record the inspected image as `PREVIOUS_BACKEND_IMAGE`.

### Task 4: Migrate Production and Deploy the Backend

**Files:**
- Execute: `migrations/20260630_moderation_rule_management.sql`
- Deploy through: `.github/workflows/deploy.yml`
- Deploy configuration: `config/config.yaml`, `config/config.prod.yaml`, `docker-compose.yml`

**Interfaces:**
- Consumes: the stopped backend, production backup, and `BACKEND_RELEASE_SHA`.
- Produces: a healthy new backend running exclusively on the migrated schema.

- [ ] **Step 1: Copy the exact reviewed migration to the production host**

From the backend release checkout:

```bash
scp migrations/20260630_moderation_rule_management.sql \
  "$REMOTE_USER@$REMOTE_HOST:/tmp/20260630_moderation_rule_management.sql"
```

Expected: the file SHA-256 on the production host matches the local release file.

- [ ] **Step 2: Execute the migration once**

Run on the production host using the production database values from the private `.env`:

```bash
mysql --show-warnings \
  --host="$DB_HOST" \
  --port="$DB_PORT" \
  --user="$DB_USER" \
  --password \
  "$DB_NAME" < /tmp/20260630_moderation_rule_management.sql
```

Expected: exit code 0 and no failed statement. If any statement fails, stop here and follow Recovery A; do not rerun against the partially migrated database.

- [ ] **Step 3: Verify the production migration before starting a server**

Run the schema queries from Task 2 Step 3 against production.

Expected: the same results as staging. The backend remains stopped until these checks pass.

- [ ] **Step 4: Merge the verified backend release into `main`**

Use the normal reviewed merge process. Confirm the resulting production workflow is building the recorded release contents and producing an exact commit-SHA image.

Expected: GitHub Actions test and image jobs pass; the deploy job synchronizes production config and starts `blog-server` with the new SHA image.

- [ ] **Step 5: Verify backend deployment on the production host**

```bash
cd "${DEPLOY_DIR:-/opt/blog-backend}"
docker compose ps blog-server
docker inspect --format '{{.Config.Image}} {{.State.Status}} {{.State.Restarting}}' "${COMPOSE_CONTAINER_NAME:-blog-server}"
curl --fail --silent --show-error "http://127.0.0.1:${SERVER_PORT:-8080}/health"
docker logs --since 10m "${COMPOSE_CONTAINER_NAME:-blog-server}"
```

Expected: the exact release image is running, health returns success, the container is not restarting, ruleset 1 loads or rebuilds successfully, and logs contain no migration, index, Garage, panic, or out-of-memory error.

- [ ] **Step 6: Verify rule-management API readiness**

Sign in with the production administrator account and open the rule-management status endpoint through the existing admin authentication flow.

Expected: the response reports a published current ruleset, no failed candidate, non-negative rule counts, and index/build memory values within configured limits.

### Task 5: Deploy and Verify the Admin Frontend

**Files:**
- Deploy through: `../blog-frontend/.github/workflows/deploy.yml`
- Verify: `../blog-frontend/apps/admin/src/modules/moderation`

**Interfaces:**
- Consumes: the healthy production backend and `FRONTEND_RELEASE_SHA`.
- Produces: the production rule-management UI backed by the new API.

- [ ] **Step 1: Merge the verified frontend release into `main`**

Use the normal reviewed merge process only after Task 4 passes.

Expected: the frontend Deploy workflow builds the Web SHA image and admin artifact, then deploys both successfully.

- [ ] **Step 2: Verify the deployed admin application while maintenance remains active**

Temporarily allow only the operator to access the admin site through the existing BaoTa access control, or verify from the host/private network.

Check all of the following:

- Administrator login succeeds.
- The moderation page contains the rule-management tab.
- Rule status and the first cursor page load.
- CSV and TXT templates download successfully.
- Text testing returns the current ruleset ID.
- Rule export returns a downloadable file.

Expected: no browser console error, API 404, DTO parsing failure, or authentication-refresh loop.

- [ ] **Step 3: Run a small reversible import smoke test**

Create a UTF-8 TXT file containing one harmless synthetic keyword that cannot match normal site content. Import it with source name `production-smoke`, wait for validation/build completion, test the candidate, and publish it.

Expected: counts and memory estimates are displayed; publishing succeeds; the status endpoint reports the new ruleset; a backend restart reloads that ruleset successfully.

- [ ] **Step 4: Restart once to verify persisted recovery**

```bash
cd "${DEPLOY_DIR:-/opt/blog-backend}"
docker compose restart blog-server
curl --retry 12 --retry-delay 5 --retry-connrefused \
  --fail --silent --show-error \
  "http://127.0.0.1:${SERVER_PORT:-8080}/health"
```

Expected: health recovers, the published smoke ruleset remains current, and logs show successful index loading or deterministic rebuilding.

### Task 6: Restore Access and Import the Production Corpus

**Files:**
- Follow: `docs/moderation-rollout.md`
- Record results in private operator notes; do not commit the sensitive corpus or its error report.

**Interfaces:**
- Consumes: the verified backend, admin UI, and production corpus.
- Produces: a live site and a published production ruleset within configured capacity.

- [ ] **Step 1: Restore public and admin site access**

Remove the BaoTa maintenance response or site suspension.

Expected: public pages, login, content reads, and normal writes work; admin moderation pages work.

- [ ] **Step 2: Observe the baseline release before the large import**

For at least one normal traffic interval, inspect container restarts, Go memory, CPU, MySQL errors, Redis errors, Garage errors, moderation failures, and HTTP 5xx responses.

Expected: no increasing error trend or repeated restart. If the baseline is unstable, do not start the large import.

- [ ] **Step 3: Import the production corpus**

Use the admin import wizard. Verify the source, default category, effect, risk level, and priority before upload. Keep the browser file object only for the upload; rely on the persisted task status afterward.

Expected: validation completes without invalid or duplicate rows; reported rule count, artifact size, retained index bytes, and build peak bytes stay below configured limits.

- [ ] **Step 4: Test the ready candidate before publishing**

Test representative safe text and representative fixtures for every imported category. Confirm expected matches, risk levels, effects, suppression behavior, and truncation reporting.

Expected: no safe fixture receives an unacceptable result; every required risky fixture matches the intended category and rule.

- [ ] **Step 5: Publish once and verify**

Publish with the current `expected_ruleset_version`, then verify rule status, a sample of live content writes, backend logs, memory, and Garage artifact access.

Expected: the new ruleset becomes current atomically; the old ruleset serves until success; no restart or sustained memory growth occurs.

- [ ] **Step 6: Close the maintenance release**

Keep the database, configuration, and admin backups until the chosen observation period completes. Record release SHAs, ruleset ID, rule counts, index bytes, build peak bytes, publish time, and any operational anomaly.

Expected: the release has a reproducible audit record without storing rules, secrets, tokens, or private error reports in Git.

## Recovery Procedures

### Recovery A: Migration Fails or New Backend Cannot Start

1. Keep both sites in maintenance and keep `blog-server` stopped.
2. Drop the partially migrated production database only through the existing BaoTa restore workflow.
3. Restore the full backup created in Task 3.
4. Restore the previous `.env` and config archive.
5. Set `BLOG_SERVER_IMAGE` to `PREVIOUS_BACKEND_IMAGE` and start `blog-server`.
6. Verify `/health` and the pre-release site before removing maintenance.
7. Diagnose the migration or startup failure on a fresh staging restore before scheduling another attempt.

Do not rerun the migration over a partially migrated database and do not start the old image against the new schema.

### Recovery B: Backend Works but Admin Deployment Fails

1. Leave the new backend and migrated database running.
2. Restore `PREVIOUS_ADMIN_BACKUP` to the admin static directory.
3. Verify existing admin functions and backend health.
4. Fix the frontend artifact and redeploy it; no database restore is required.

### Recovery C: Large Import or Candidate Build Fails

1. Do not restore the database or backend image.
2. Confirm the current published ruleset remains unchanged.
3. Download the error report, cancel the failed candidate if allowed, and correct the source file offline.
4. Retry as a new import only after memory, row-count, duplicate, and format errors are resolved.

### Recovery D: Published Rules Produce Incorrect Results

1. Stop further rule edits and record the current ruleset ID.
2. Use rule replacement or bounded batch deactivation to publish a corrective ruleset.
3. Verify the correction with text testing before reopening affected content writes if they were manually suspended.
4. Use a full database restore only for a system-wide failure that cannot be corrected through a new ruleset; restoring the database discards post-backup production writes.
