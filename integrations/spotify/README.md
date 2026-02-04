# Spotify Integration (Homenavi)

A full Spotify player integration with a dedicated tab and a dashboard widget. Supports:

- Play / pause
- Seek
- Volume
- Skip forward/backward
- Shuffle + loop
- Device selection
- Queue rendering
- Search (tab) with Play Now + Add to Queue

## Environment variables

Copy the example env file and fill in your secrets:

```bash
cp .env.example .env
```

Set the following values in the `.env` file:

- `SPOTIFY_CLIENT_ID`
- `SPOTIFY_CLIENT_SECRET`
- `SPOTIFY_REFRESH_TOKEN`

If you prefer central management, use the Admin → Integrations page to set these secrets (declared in the manifest). Values are write-only and cannot be read back. The integration exposes a write-only admin endpoint at `GET/PUT /api/admin/secrets` (admin-only; values are never returned). For admin access, mount the Homenavi JWT public key and set `JWT_PUBLIC_KEY_PATH` in the container.

The integration reads secrets from `INTEGRATION_SECRETS_PATH` (or `INTEGRATIONS_SECRETS_PATH` for compatibility) if environment variables are not set. By default it uses `config/integration.secrets.json` in the repo/container.

## How to get the Spotify credentials

1) Create a Spotify developer app at https://developer.spotify.com/dashboard
2) Copy the **Client ID** and **Client Secret** from the app settings.
3) Add a Redirect URI (e.g. `http://localhost:8888/callback`) in the app settings.
4) Run an OAuth authorization flow (with `user-read-playback-state`, `user-modify-playback-state`, and `user-read-currently-playing` scopes) to obtain a **refresh token**.
5) Set the environment variables above.

## Local dev (frontend)

```bash
cd src/frontend
npm install
npm run dev:tab
# in another terminal
npm run dev:widget
```

## Build + run

```bash
cd src/frontend
npm install
npm run build

cd ../..
go run ./src/backend/cmd/integration
```

## Docker

From the repo root:

```bash
docker build -t homenavi-spotify:local integrations/spotify
```

Run the container on the Homenavi network (using the repo file path):

```bash
docker run --rm \
  --name spotify \
  --network homenavi_homenavi-network \
  -v $(pwd)/integrations/spotify/config/integration.secrets.json:/app/config/integration.secrets.json \
  -e INTEGRATION_SECRETS_PATH=/app/config/integration.secrets.json \
  -v $(pwd)/keys/jwt_public.pem:/app/keys/jwt_public.pem:ro \
  -e JWT_PUBLIC_KEY_PATH=/app/keys/jwt_public.pem \
  homenavi-spotify:local
```

If you don’t need the admin secrets endpoint, omit the JWT mount/env lines.

After updating the installed integrations list, use the Admin → Integrations page and click “Refresh integrations” to reload the proxy registry.

Run the container on the Homenavi network and register it in the integrations config:

```yaml
integrations:
  - id: spotify
    upstream: http://spotify:8099
```
