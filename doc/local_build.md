# Local Build Guide

This guide explains how to build and exercise Homenavi services on a developer
workstation. It covers both the recommended Docker Compose flow and manual
binary/image builds for advanced tinkering.

---

## Prerequisites
- Go (latest recommended)
- Node.js & npm (for frontend)
- Docker + Docker Compose

---

## Quickstart (Docker Compose)

1. Copy the sample environment file:
	```sh
	cp .env.example .env
	```
	Adjust ports, credentials, and feature toggles as needed.
2. Start the full stack:
	```sh
	docker compose up --build
	```
3. Once the stack is healthy, run integration tests from another terminal:
	```sh
	cd test
	export INTERACTIVE_CODES=0
	python3 e2e/test_auth_full.py
	```
	Replace the script with any other helper in `test/` as needed.

> **Tip:** If you maintain a separate override (for example to map local source
> code or use mocked adapters), launch with
> `docker compose -f docker-compose.yml -f docker-compose.override.yml up`.

---

## Manual Backend Builds

Each Go service can be built individually:

```sh
cd api-gateway
go build .

cd ../auth-service
go build .

cd ../user-service
go build .
```

---

## Manual Frontend Build

```sh
cd frontend
npm install
npm run dev
```

---

## Docker Images (Manual)

You can build Docker images for each service:

```sh
docker build -t homenavi-api-gateway:latest ./api-gateway
docker build -t homenavi-auth-service:latest ./auth-service
docker build -t homenavi-user-service:latest ./user-service
docker build -t homenavi-frontend:latest ./frontend
```

---

For day-to-day development, prefer Docker Compose so that shared services (DB,
MQTT, mail) are provisioned automatically.

---

## Integration Tests

Two helper scripts live under `test/` and can be executed against a running
stack (Docker Compose is fine):

- `test_devices_list.py` — enumerates all devices via the API gateway and
	collects retained MQTT state for sanity checks.
- `test_pairing_mock.py` — exercises the guided Zigbee pairing flow end to end
	by logging in as the admin user, starting a pairing session, creating a
	synthetic Zigbee device, and publishing mock pairing progress frames over
	MQTT/WebSockets.

Both scripts accept the `GATEWAY_ORIGIN`, `WS_STATE_URL`, `ADMIN_EMAIL`, and
`ADMIN_PASSWORD` environment variables. For the mock pairing test you can keep
the generated device by setting `KEEP_MOCK_DEVICE=1`.

When the Docker Compose stack is already running you can chain tests, e.g.:

```sh
cd test
export INTERACTIVE_CODES=0
python3 e2e/test_auth_full.py && python3 test_pairing_mock.py
```