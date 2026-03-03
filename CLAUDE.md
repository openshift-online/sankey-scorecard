# Sankey Scorecard

Go CLI and API tool that scores engineering teams on Sankey planning framework adherence by analyzing Jira issue data.

## Project Structure

- `cmd/` -- Cobra CLI commands (thin layer, no business logic)
- `config/sankey-scorecard.yaml.example` -- Annotated template for the central config file (copy to `config/sankey-scorecard.yaml` and fill in real values)
- `frontend/` -- Embedded single-page web UI (go:embed, served at `/`)
- `pkg/config/` -- YAML parsing, validation, identifier resolution
- `pkg/handlers/` -- HTTP API handlers and router (net/http ServeMux)
- `pkg/db/` -- PostgreSQL persistent store (GORM models, PGStore implementing DataStore)
- `pkg/reconciler/` -- Jira data fetching, sprint calendar, DataStore interface, in-memory store
- `pkg/scorecard/` -- Score computation, grade assignment, output formatting
- `tests/` -- Integration tests (Ginkgo, build tag: `integration`)
- `deploy/openshift/` -- OpenShift deployment manifests (app + PostgreSQL) and deploy script
- `Containerfile` -- Multi-stage container build (golang builder, UBI minimal runtime)
- `scoring-strategy.md` -- Authoritative scoring strategy spec (update this for all scoring changes)
- `.spec/` -- Original design specifications (read-only reference; do NOT update these files during implementation)

## Build and Test

```bash
make build              # Build binary
make test               # Unit tests (pkg/...)
make test-integration   # Integration tests (tests/...)
make lint               # golangci-lint
make test-all           # All of the above
make build-image        # Build container image with podman
make deploy             # Deploy to OpenShift (requires NAMESPACE)
make deploy-teardown    # Remove OpenShift resources (requires NAMESPACE)
```

## Frontend (Web UI)

- **Single-file SPA**: `frontend/index.html` contains all HTML, CSS, and JavaScript inline. No build step, no framework -- vanilla JS with client-side rendering.
- **Embedded via go:embed**: `frontend/embed.go` embeds `index.html` into the binary. The router serves it at `GET /` using `http.FileServerFS`. Go 1.22+ ServeMux pattern specificity ensures API routes (`/api/*`, `/healthz`) take priority over the catch-all `/`.
- **RHDS web components**: Uses Red Hat Design System components (`rh-table`, `rh-accordion`, `rh-spinner`) loaded from jsDelivr CDN (`@rhds/elements@4.0.2`). Fonts loaded from Google Fonts.
- **CDN requirement**: The UI requires internet access to jsDelivr and Google Fonts. It will not render correctly in air-gapped or CDN-restricted environments. The API remains fully functional without CDN access.
- **Methodology duplication**: The scoring methodology accordion in `index.html` is a separate copy from `scoring-strategy.md`. When scoring changes are made, both must be updated manually.
- **Modify the UI**: Edit `frontend/index.html` directly. Changes are picked up on next `make build` (re-embedded).

## Key Design Decisions

- **Nil vs Zero**: Teams with 0 issues get `nil` scores and grade `"-"`, not zero. Score fields are `*float64` for correct JSON null serialization.
- **Config file loading order**: `--config` / `-c` flag → `RESOURCE_MAP_PATH` env var → `/etc/sankey-scorecard/sankey-scorecard.yaml` (container default). The config is NOT embedded in the binary. In OpenShift, `deploy.sh` creates a ConfigMap (`sankey-scorecard-config`) from your local `config/sankey-scorecard.yaml` and mounts it at the default path. `config/sankey-scorecard.yaml` is gitignored; use `config/sankey-scorecard.yaml.example` as a template.
- **DataStore interface**: `reconciler.DataStore` abstracts storage. `ReconciliationStore` (in-memory) and `db.PGStore` (PostgreSQL via GORM) both implement it. CLI always uses in-memory; `serve` uses PG when `--database-url` or `DATABASE_URL` is set.
- **Atomic store swap**: Reconciler builds all data locally, swaps entire store under write lock. Partial updates never applied (replace mode). Upsert mode merges teams instead.
- **Weighted averages**: Pillar/org scores are issue-count-weighted averages. Nil-scored teams excluded.
- **Asymmetric penalties**: Over-allocating to high-priority categories penalized less than low-priority. Weights: `over_weight(rank) = 0.5 + (rank-1)*0.25`, `under_weight(rank) = 1.5 - (rank-1)*0.25` (5 scored categories).
- **Associate Wellness excluded from distribution scoring**: AW issues count toward categorization rate but are excluded from distribution alignment. See `scoring-strategy.md` for full details.
- **CLI refresh is synchronous**, API refresh is asynchronous (returns 202).
- **Sprint calendar**: Calculated from reference date + duration, not from Jira API.
- **Activity Type mapping**: Hardcoded in `pkg/scorecard/categories.go`, not configurable.
- **Custom field access**: Activity Type read from `issue.Fields.Unknowns[fieldID]` as `map[string]interface{}` with `"value"` key. Field ID defaults from `jira.activity_type_field` in the resource map, overridable via `--activity-type-field`.

## Testing

- Test framework: Ginkgo/Gomega
- All packages in `pkg/` must maintain >80% coverage
- `cmd/` is not directly tested; all logic is tested through package-level tests
- Integration tests use a mock HTTP server mimicking the Jira REST API (`/rest/api/2/search`)
- The `JiraClient` interface in `pkg/reconciler` enables mock injection
- `pkg/db/pgstore_test.go` uses testcontainers-go to spin up PostgreSQL; these tests require the `integration` build tag and a container runtime (podman/docker)
- **No separate suite files**: Do not create `*_suite_test.go` files. Place the Ginkgo bootstrap function (`func TestXxx(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "...") }`) directly in the main `_test.go` file for each package.

## Dependencies

- `github.com/spf13/cobra` -- CLI framework
- `github.com/andygrunwald/go-jira/v2` -- Jira SDK (onpremise package for PAT auth)
- `gorm.io/gorm` + `gorm.io/driver/postgres` -- ORM for PostgreSQL persistent storage (uses pgx under the hood)
- `github.com/lib/pq` -- PostgreSQL `text[]` type support (`pq.StringArray`)
- `github.com/onsi/ginkgo/v2` + `gomega` -- Test framework
- `github.com/testcontainers/testcontainers-go` -- PostgreSQL integration tests (test only)
- `gopkg.in/yaml.v3` -- YAML parsing

## Common Tasks

- **Add a new team**: Edit `config/sankey-scorecard.yaml` (your local, gitignored copy), add entry under the appropriate pillar with ownership method
- **Add a new Activity Type mapping**: Edit `pkg/scorecard/categories.go`, add to `activityTypeMapping`
- **Change target distribution**: Edit `DefaultTargetDistribution` in `pkg/scorecard/categories.go` and update `scoring-strategy.md`
- **Change scoring strategy**: Update `scoring-strategy.md` (authoritative spec), implement in `pkg/scorecard/`, and update the methodology accordion in `frontend/index.html`
- **Modify the web UI**: Edit `frontend/index.html` (single file, inline CSS/JS). Rebuild to re-embed.
- **Update RHDS version**: Change the version in the import map and CSS link in `frontend/index.html` (currently `@4.0.2`)
- **Modify API routes**: Edit `pkg/handlers/router.go` and `pkg/handlers/handlers.go`
- **Enable PostgreSQL storage**: Set `DATABASE_URL` env var or pass `--database-url` to `serve`. Schema is auto-migrated on startup. Without it, in-memory storage is used (CLI always uses in-memory).
- **Refresh mode**: `POST /api/refresh_data?mode=upsert` merges teams into existing data; `?mode=replace` (default) replaces all data.
- **Scoped refresh**: `POST /api/refresh_data?start_date=...&end_date=...&status=...` limits the refresh to a subset of data. Scope params force upsert mode (preserves out-of-scope data). `mode=replace` cannot be combined with scope params. Period-level merge keeps existing periods whose `SetType` was not refreshed (e.g., in-progress periods preserved when only closed data is refreshed). Scope is API-only; the CLI always does a full refresh.
- **Deploy to OpenShift**: Run `make deploy NAMESPACE=<ns>`. The script builds the image, pushes it to the internal registry (`${NAMESPACE}/sankey-scorecard:latest`), creates the `sankey-scorecard-config` ConfigMap, deploys PostgreSQL and the app. Prerequisite: `sankey-scorecard-jira` secret must exist in the namespace. Override via env vars: `CONFIG_FILE`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`.
- **Route configuration**: `ROUTE_SHARD` (default: `internal`) sets the `shard` label on the route for clusters with shard-based ingress routing. `ROUTE_HOST` sets an explicit hostname (required on managed platforms where the router shard serves a specific domain). Example: `ROUTE_HOST=sankey-scorecard.apps.example.com ROUTE_SHARD=internal make deploy NAMESPACE=my-ns`. On self-managed clusters without shard routing, set `ROUTE_SHARD=""`.
- **Managed platform image**: For managed platform deployments, push the image to your container registry (e.g. `podman push localhost/sankey-scorecard:<tag> <registry>/<org>/sankey-scorecard:latest`) then restart the deployment (`oc rollout restart deployment/sankey-scorecard -n <namespace>`).
- **Modify deployment manifests**: Edit files in `deploy/openshift/`, the `NAMESPACE` placeholder is substituted at deploy time. PostgreSQL manifests: `postgres-pvc.yaml`, `postgres-deployment.yaml`, `postgres-service.yaml`.
