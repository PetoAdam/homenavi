# Homenavi Integration Template (Standalone Repo)

This directory is a **copy-out template** for building a Homenavi integration repository.

It implements:

- `GET /.well-known/homenavi-integration.json` (manifest)
- An iframe UI surface (sidebar tab)
- An iframe widget surface
- A single React (Vite) codebase that can build both the tab UI and widget UI
- Basic security headers + CSP suitable for sandboxed iframes
- A manifest schema validator (`cmd/validate-manifest`)
- A GitHub Actions “verification” workflow (tests + gosec + manifest validation + docker build)

## Secrets declaration (admin-managed)

Integrations should declare required secrets in the manifest so the Homenavi Admin → Integrations page can render editable fields:

```json
"secrets": ["EXAMPLE_API_KEY", "EXAMPLE_API_SECRET"]
```

Values are sent to the integration via write-only fields and can be stored locally by the integration itself.

This template exposes a write-only admin endpoint at `GET/PUT /api/admin/secrets` (admin-only; values are never returned). It stores secrets in `config/integration.secrets.json` by default (configurable via `INTEGRATION_SECRETS_PATH`). For admin access, mount the Homenavi JWT public key and set `JWT_PUBLIC_KEY_PATH` in the integration container.

## Project layout (clean & minimal)

```
src/
	backend/     # integration backend logic (HTTP handlers, data access, etc.)
	frontend/    # integration UI (tab + widget) + Vite config
web/
	assets/      # static assets (icons, etc.)
	ui/          # build output for /ui
	widgets/     # build output for /widgets/<id>
```

**Rule of thumb:** edit code under `src/` and treat `web/` as build output/static assets.

- `src/backend`: full integration HTTP server (routes + API + static wiring)
- `src/backend/cmd/integration`: standalone entrypoint
- `src/frontend`: tab + widget UI code

## Quick start (local)

```bash
go test ./...
go run ./cmd/validate-manifest

# Build the tab + widget UI from the single React codebase
cd src/frontend
npm install
npm run build
cd ../..

go run ./src/backend/cmd/integration
```

Then open:

- `http://localhost:8099/ui/`
- `http://localhost:8099/widgets/hello/`

## Frontend dev (React)

This template uses one React codebase to build **two entrypoints**:

- Tab UI → `web/ui/` (served at `/ui/`)
- Widget UI → `web/widgets/hello/` (served at `/widgets/hello/`)

Commands (from `src/frontend`):

```bash
cd src/frontend
npm install

# dev server for tab
npm run dev:tab

# dev server for widget
npm run dev:widget

# produce production assets into web/ui + web/widgets/hello
npm run build
```

UI preview during dev:

- Tab dev server: http://localhost:10000/tab.html
- Widget dev server: http://localhost:10001/widget.html

If the port changed (free-port auto-pick), use the exact URL printed by the dev server.

## Local dev flow (backend + frontend)

1) Run the Go backend (serves manifest + APIs + built assets):

```bash
go run ./src/backend/cmd/integration
```

2) In another terminal, run the Vite dev server (tab or widget):

```bash
cd src/frontend
npm install
npm run dev:tab
```

The backend API runs on `http://localhost:8099` and the dev UI runs on the Vite port
(default `10000` / `10001`). The UI talks to the backend via relative `/api/...` calls.

## Docker: auto port assignment (no manual host ports)

If you run integrations with Docker Compose, **do not publish a fixed host port**.
Either omit `ports` entirely (recommended inside the Homenavi network) or use `0:8099`:

```yaml
services:
	hello-homenavi:
		image: your-org/hello-homenavi:latest
		ports:
			- "0:8099" # auto-assign host port
```

Inside the Homenavi stack, the integration proxy should use the **container port**:

```yaml
integrations:
	- id: hello-homenavi
		upstream: http://hello-homenavi:8099
```

If port 5173/5174 is in use, set `HN_PORT`:

```bash
HN_PORT=5175 npm run dev:tab
HN_PORT=5176 npm run dev:widget
```

## Docker: build + run on the Compose network

Use Docker directly (no Compose) but attach to the same network as the core stack
(replace `homenavi_default` with your Compose network name):

```bash
docker build -t homenavi-integration:local .

docker run --rm \
	--name hello-homenavi \
	--network homenavi_homenavi-network \
	homenavi-integration:local
```

Then configure the integration proxy to reach it via container DNS:

```yaml
integrations:
	- id: hello-homenavi
		upstream: http://hello-homenavi:8099
```

## Test checklist (local)

These commands validate the integration builds correctly:

- Run unit tests: `go test ./...`
- Validate manifest schema: `go run ./cmd/validate-manifest`
- Build UI assets: `cd src/frontend && npm install && npm run build`
- Build Docker image: `docker build -t homenavi-integration:local .`

## Environment

- `PORT` (default `8099`)

## Security notes

This server sets strict headers for iframe content:

- `Content-Security-Policy` with `connect-src 'self'` and `default-src 'none'`
- `X-Frame-Options: SAMEORIGIN` / `frame-ancestors 'self'`
- `Referrer-Policy: no-referrer`
- `X-Content-Type-Options: nosniff`

The intended host (Homenavi) should still render third-party UI inside an `<iframe sandbox>` without `allow-same-origin`.

## Icons (Font Awesome, etc.)

Homenavi **does not allow remote icon URLs** for integrations.

Use one of these:

### Option A (recommended): bundled icon under `/assets/...`

Ship an SVG/PNG in `web/assets/` and reference it portably from the manifest:

```json
{
	"ui": {
		"sidebar": {
			"icon": "/assets/icon.svg"
		}
	}
}
```

At runtime, Homenavi will serve it same-origin via the integration proxy as:

- `/integrations/<id>/assets/icon.svg`

This template includes `web/assets/icon.svg` and the default manifest uses it.

The included `icon.svg` is intentionally **monochrome white** so it works well across themes/backgrounds.

### Option B: Font Awesome token

Set `ui.sidebar.icon` to a token string:

```json
{
	"ui": {
		"sidebar": {
			"icon": "fa:sparkles"
		}
	}
}
```

Notes:

- Tokens are intentionally limited/allowlisted by the Homenavi frontend (for safety + consistency).
- Remote URLs like `https://.../icon.svg` are rejected by the verifier and dropped by the runtime registry.

If you want a “no-license-surprises” default for custom icons, keep using bundled SVGs you own (or permissively-licensed sets) and serve them from `/assets/`.
