# CPU Voice POC

This profile runs CPU-only faster-whisper and Piper locally, with the gateway forwarding transcripts to the existing Open WebUI Orion chat endpoint. It is a manual/push-to-talk vertical slice; no credentials are embedded in the repository.

## Start locally

Set the Open WebUI OpenAI-compatible endpoint and model, then start the stack:

```sh
export OPEN_WEBUI_CHAT_URL='https://your-open-webui.example/api/chat/completions'
export OPEN_WEBUI_MODEL=orion
# Required only when the Open WebUI deployment authenticates its OpenAI-compatible API.
export OPEN_WEBUI_API_KEY='...'
# Required when the API key is not itself an Open WebUI user session JWT.
export OPEN_WEBUI_SOCKET_TOKEN='...'
# Optional: defaults to the bundled Piper service.
export TTS_URL='http://your-tts-host:8000/tts'
docker compose -f voice-poc/docker-compose.yml up --build
```

`TTS_URL` selects the HTTP TTS provider used by the gateway. It can be exported as above or set in a `voice-poc/.env` file; leave it unset to use the bundled `piper` service. In Kubernetes, set the same `TTS_URL` environment value in the voice gateway GitOps manifest.

For Orion voice control, `OPEN_WEBUI_TOOL_IDS` defaults to `server:mcp:homenavi-mcp`. This explicitly attaches the Homenavi MCP server to API chat-completion requests; Open WebUI does not attach model tools implicitly for API callers.

Managed responses use a Socket.IO WebSocket authenticated with `OPEN_WEBUI_SOCKET_TOKEN`. In Open WebUI v0.11.4 this must be a user session JWT, not an API key, and it must belong to the same user as `OPEN_WEBUI_API_KEY`. If the HTTP credential is itself a session JWT, the socket token may be omitted. Store both credentials in a secret, keep the session token renewed before it expires, and allow WebSocket upgrades through any proxy. Set the socket token in the Kubernetes voice gateway secret as well as in local Compose. No automatic token refresh is provided.

Every voice turn creates a saved Open WebUI chat, including turns without tools. Its retention follows the Open WebUI deployment's chat policy. The assistant is instructed to describe device writes as sent or queued, but model instructions alone do not verify device state or guarantee its wording.

The gateway tests include an opt-in live managed-stream check. Set `OPEN_WEBUI_TEST_BASE_URL`, `OPEN_WEBUI_TEST_API_KEY`, `OPEN_WEBUI_TEST_SOCKET_TOKEN`, and `OPEN_WEBUI_TEST_MODEL` to a model with tools disabled, then run `GOWORK=off go test ./cmd/voice-gateway -run '^TestLiveManagedChatStreamsReadOnly$'` from the `voice-gateway` directory. This sends a harmless prompt and deletes the temporary chat afterward; it may incur model charges. Without these variables the test is skipped.

The first startup downloads the Whisper and Piper models into named Docker volumes. `base.en` is CPU-friendly for English; use `WHISPER_MODEL=tiny.en` for a faster, lower-accuracy Pi experiment.

## Run STT/TTS on the development machine

Run the providers locally on the development machine:

```sh
docker compose -f voice-poc/docker-compose.local-providers.yml up --build
```

Keep this tunnel running from the development machine so the Pi can reach those providers over its loopback interface:

```sh
sh voice-poc/run-pi-provider-tunnel.sh
```

On the Pi, run the gateway with the local-provider override. Its Open WebUI endpoint and API key remain in the Pi-local `.env` file:

```sh
docker compose -f docker-compose.yml -f docker-compose.pi-local-providers.yml up -d --no-deps voice-gateway
```

The override puts only the gateway on host networking so it can use the tunnel endpoints at `127.0.0.1:18001` and `127.0.0.1:18002`. Stop the Pi-local `faster-whisper` and `piper` services after the gateway becomes healthy.

Set `VOICE_GATEWAY_LOG_CONTENT=true` only for Pi-local diagnostics. It logs STT transcripts and Orion response sentences to the gateway container logs; leave it unset for normal use.

`POST /voice/v1/turns` requires a complete mono 16 kHz WAV request and produces a streaming `multipart/mixed` response. Each part is a sentence-sized `audio/wav` response segment. Cancelling the client request cancels the in-flight STT and TTS calls and requests that Open WebUI stop the managed chat. A device command already sent may still execute; the gateway does not retry the chat or command.

This POC is deliberately unauthenticated. Keep it on the LAN, and do not expose port 8090 until satellite credentials and pairing are implemented.