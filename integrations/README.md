# Integrations

This folder contains **third-party style integrations** that can be served alongside a Homenavi deployment.

They are structured so you can:

- Run them from this monorepo via `docker-compose.yml` (dev convenience)
- Or copy one of them out into a standalone repository

## Template

Use `integrations/integration-template-repo/` as a starting point for new integrations.

## Installing integrations (current)

Today, “installing” an integration is a deployment concern:

- The integration ships as **one Docker image** (it contains its backend + bundled static web assets).
- You run that container in the same network as `integration-proxy` (Docker Compose or Kubernetes).

Tip: do not publish fixed host ports. Either omit `ports` entirely or use `0:8099` for auto-assigned host ports.

### Where the install config lives (important)

The “installed integrations” list is **deployment configuration**, not source code.

- This repo ships a committed example at `integrations/config/installed.example.yaml`.
- Your real config should live outside git (or at least be gitignored), and be mounted into the deployment.

In Docker Compose, set `INTEGRATIONS_CONFIG_FILE` to point at your deployment-local YAML file.

### Registering an integration

You register it by adding an entry into your installed integrations YAML so `integration-proxy` knows:
	- the integration `id`
	- where to reach it on the internal network

After that, Homenavi discovers it via the integration registry and renders:

- The tab UI under `/apps/<id>` (host shell route), with content served from `/integrations/<id>/...`.
- Any widgets exposed by the manifest.

This is intentionally simple for now; a “store” later can automate the same steps.

## Icons

Integrations can choose how their sidebar icon renders:

- FontAwesome token: set `ui.sidebar.icon` to something like `fa:plug`, `fa:music`, `fa:sparkles`, `fa:spotify`.
- Image URL/path: set `ui.sidebar.icon` to a same-origin path like `/assets/icon.svg`

If you use a path starting with `/` (like `/assets/icon.svg`), the registry will automatically serve it as `/integrations/<id>/assets/icon.svg`.

## What this folder becomes

Long-term, this repo’s `integrations/` folder should mainly hold:

- `integrations/spec/` (schema + documentation)
- `integrations/config/` (local/dev installed integrations config)

The template is here as a convenience while the workflow solidifies; it’s expected to be moved into its own standalone repo.

## Secrets declaration (admin-managed)

Integrations should declare required secrets in the manifest:

```json
"secrets": ["EXAMPLE_API_KEY", "EXAMPLE_API_SECRET"]
```

Admins can then manage values in the Admin → Integrations page (write-only fields), which sends values to each integration’s admin endpoint.

Integrations should expose a write-only admin endpoint at `GET/PUT /api/admin/secrets` and store values in `config/integration.secrets.json` (configurable with `INTEGRATION_SECRETS_PATH`) to avoid cross-integration access.

## Integration CI/CD (centralized)

Integration repos should use the centralized actions hosted in the main Homenavi repo:

- Verify: `PetoAdam/homenavi/.github/actions/integration-verify@main`
- Release: `PetoAdam/homenavi/.github/actions/integration-release@main`

This keeps the release logic and marketplace publish flow inside the main repo so third-party integrations do not run custom publish scripts.

### Release inputs

The release action expects:

- `image_name` (e.g. `homenavi-example`)
- `manifest_path` (default `manifest/homenavi-integration.json`)
- `metadata_path` (default `marketplace/metadata.json`)

Marketplace publish only runs when `MARKETPLACE_API_URL` and `MARKETPLACE_PUBLISH_TOKEN` are provided.

