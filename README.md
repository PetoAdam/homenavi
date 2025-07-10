# Homenavi

**A smart home platform for developers, by developers. Modern, microservice-based, and built to be extended.**

Welcome to Homenavi – your open, hackable smart home solution. Built with a modern microservices architecture, Homenavi is designed for tinkerers, makers, and pros who want full control and easy extensibility.

[![Build Frontend Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml) [![Build User Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml) [![Build API Gateway Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml) [![Build Auth Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml)

---

## 🚀 Why Homenavi?
- **Microservice-first:** Each core feature is its own service – scale, swap, or extend as you like.
- **Modern stack:** Go, React, Docker, and more.
- **Dev-friendly:** Easy to run, hack, and contribute.
- **Open & Transparent:** 100% open source, MIT licensed.
- **Cloud or Home:** Run it on your Raspberry Pi, your server, or in the cloud.
- **Observability built-in:** Prometheus metrics, Jaeger tracing, and request/correlation IDs for easy debugging and monitoring.

---

## 🏗️ Project Structure

```
frontend/           # React frontend
api-gateway/        # Go API gateway
auth-service/       # Go authentication service
user-service/       # Go user service
.github/workflows/  # GitHub Actions workflows
prometheus/         # Prometheus config
nginx/              # Nginx config
```

---

## 🧩 Architecture

Homenavi is built as a set of loosely coupled microservices:
- **Frontend:** Modern React SPA, served by Nginx.
- **API Gateway:** Central entrypoint, JWT auth, request routing, rate limiting, observability (metrics/tracing).
- **Auth Service:** Handles authentication, issues JWTs.
- **User Service:** Manages user data and validation.
- **Observability Stack:** Prometheus, Grafana, and Jaeger for monitoring and tracing.

All services communicate via HTTP/REST. Add your own services and plug them into the gateway!

---

## 🐳 Quickstart (Recommended)

The fastest way to get Homenavi running is with Docker Compose. Just clone, configure, and launch:

```sh
git clone https://github.com/PetoAdam/homenavi.git
cd homenavi
cp .env.example .env  # Edit as needed for secrets and service URLs
# Then:
docker compose up --build
```

- All services (including Prometheus, Grafana, Jaeger, Redis, and Nginx) will be built and started automatically.
- Nginx is the public entrypoint and routes `/api` to the API Gateway and `/` to the frontend.
  - For detailed Nginx configuration and security best practices, see [`doc/nginx_guide.md`](doc/nginx_guide.md).
- Edit `.env` for secrets and service URLs.
- See [`/doc/local_build.md`](doc/local_build.md) for advanced/local development.

### 🔑 JWT Key Generation (RS256)

Homenavi uses asymmetric JWT signing (RS256) for secure authentication:
- The **Auth Service** signs tokens with a private key.
- The **API Gateway** and other services validate tokens using the public key only.

### Generate a key pair for development/testing:
```sh
mkdir -p ./keys
openssl genpkey -algorithm RSA -out ./keys/jwt_private.pem -pkeyopt rsa_keygen_bits:2048
openssl rsa -pubout -in ./keys/jwt_private.pem -out ./keys/jwt_public.pem
```
- Set `JWT_PRIVATE_KEY_PATH` and `JWT_PUBLIC_KEY_PATH` in your `.env` to point to these files.
- **Never commit your private key to version control!**

---

## 🛠️ Extending Homenavi

Want to add a new service? Just drop it in, add to `docker-compose.yml`, and go! The API Gateway makes integration easy.

- Use Go, Python, Node.js, or any language you like.
- Register new routes in the API Gateway.
- Share authentication via JWT.
- Add your own metrics and traces for observability.
- Check out the [Extending Guide](doc/extending.md) (coming soon!)

---

## 📦 CI/CD

- All services are built and tested via GitHub Actions on every push/PR.
- Docker images are saved as artifacts for easy deployment.
- Workflows for each service live in `.github/workflows/`.
- Add your own workflow for new services!

---

## 📊 Observability & Monitoring

- **Prometheus metrics:**
  - API Gateway exposes `/metrics` for Prometheus scraping.
  - Per-endpoint request counts (`api_gateway_requests_total`).
  - Go runtime/process metrics.
- **Distributed tracing:**
  - Jaeger integration via OpenTelemetry.
  - All requests are traced; spans include HTTP method, path, and response code.
- **Correlation/Request IDs:**
  - Every request gets a unique `X-Request-ID` and `X-Correlation-ID` for easy tracking across logs and traces.
- **Grafana dashboards:**
  - Visualize metrics from Prometheus.
- **Healthcheck:**
  - `/healthz` endpoint for liveness/readiness checks.

### How to use
- Prometheus: [http://localhost:9090](http://localhost:9090)
- Grafana: [http://localhost:3000](http://localhost:3000) (add Prometheus as a data source, URL: `http://prometheus:9090`)
- Jaeger: [http://localhost:16686](http://localhost:16686)


---

## 🤝 Contributing

We love contributions! Whether it's a bugfix, new feature, or documentation improvement:
- Fork the repo & create a branch
- Open a pull request with a clear description
- Join the discussion in Issues or Discussions

---

## 🌐 Community & Support

- [Issues](https://github.com/PetoAdam/homenavi/issues)
- [Discord](https://discord.gg/your-invite) (coming soon)

---

## 📚 Docs
- [Local Build Guide](doc/local_build.md)

---

## 🔌 API Overview

- **POST /api/login** (API Gateway):
  - Forwards to Auth Service, returns JWT on success
- **GET /api/user/{id}** (API Gateway):
  - Requires JWT, fetches user info from User Service
- **GET /metrics**: Prometheus metrics endpoint
- **GET /healthz**: Healthcheck endpoint

---

## ❓ FAQ

**Q: Can I run Homenavi on a Raspberry Pi?**
A: Yes! All services are built for multi-arch Docker images.

**Q: How do I add a new device/service?**
A: Build a new microservice, register it in the API Gateway, and add it to Docker Compose.

**Q: Is it production-ready?**
A: Homenavi is under active development. Contributions and feedback are welcome!

**Q: How do I monitor and trace requests?**
A: Use Prometheus and Grafana for metrics, and Jaeger for distributed tracing. All are included in the default Docker Compose stack.

---

## 💡 Notes
- This README reflects the current codebase and workflows.
- PRs and contributions are welcome!

## License

MIT License  
Copyright (c) 2025 Adam Peto

See [LICENSE](LICENSE) for details.

---
