# Homenavi

**A smart home platform for developers, by developers. Modern, microservice-based, and built to be extended.**


Welcome to Homenavi – your open, hackable smart home solution. Built with a modern microservices architecture, Homenavi is designed for tinkerers, makers, and pros who want full control and easy extensibility.

[![Build Frontend Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml) [![Build User Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml) [![Build API Gateway Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml) [![Build Auth Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml) [![Build Email Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml) [![Build Profile Picture Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml) [![Build Echo Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml)

---

## 🚀 Why Homenavi?
- **Microservice-first:** Each core feature is its own service – scale, swap, or extend as you like.
- **Modern stack:** Go, React, Python, Docker, and more.
- **Dev-friendly:** Easy to run, hack, and contribute.
- **Open & Transparent:** 100% open source, MIT licensed.
- **Cloud or Home:** Run it on your Raspberry Pi, your server, or in the cloud.
- **Observability built-in:** Prometheus metrics, Jaeger tracing, and request/correlation IDs for easy debugging and monitoring.
- **WebSocket support:** Real-time communication with cookie-based JWT authentication.

---

## 🧩 Architecture

Homenavi follows a **microservices architecture** designed for scalability and modularity:

### Core Services
- **API Gateway** (Go + Chi): JWT authentication, WebSocket support, rate limiting, request routing
- **Auth Service** (Go): User authentication, JWT tokens, 2FA (TOTP/Email), OAuth integration
- **User Service** (Go): Profile management, user data storage
- **Email Service** (Go): Email notifications and verification codes
- **Profile Picture Service** (Python): Image processing and avatar management
- **Echo Service** (Python): WebSocket testing and real-time communication demo

### Frontend & Infrastructure
- **Frontend** (React + Vite): Modern SPA with PWA capabilities, cookie-based auth
- **Database**: PostgreSQL for persistent data storage
- **Cache**: Redis for sessions and temporary data
- **Proxy**: Nginx for load balancing and SSL termination
- **Monitoring**: Prometheus metrics + Grafana dashboards + Jaeger tracing

### Authentication & Security
- **JWT RS256**: Cryptographically signed tokens with public/private key pairs
- **Cookie-based WebSocket auth**: Seamless authentication for real-time features
- **2FA Support**: TOTP (Google Authenticator) and Email-based verification
- **Rate Limiting**: Protection against brute force and abuse
- **Account Lockout**: Automatic protection against repeated failed attempts

Add your own services and plug them into the gateway!

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

**Services Overview:**
- **Frontend**: React app with authentication and PWA features
- **API Gateway**: Central routing with JWT authentication (REST + WebSocket)
- **Auth Service**: User authentication, 2FA, password management
- **User Service**: User profile and data management
- **Email Service**: Email notifications and verification
- **Profile Picture Service**: Avatar and image handling
- **Echo Service**: WebSocket testing and demonstration
- **Monitoring**: Prometheus (`:9090`), Grafana (`:3000`), Jaeger (`:16686`)

---

### 🔑 JWT Key Generation (RS256)

Homenavi uses asymmetric JWT signing (RS256) for secure authentication:
- The **Auth Service** signs tokens with a private key.
- The **API Gateway** and other services validate tokens using the public key only.

### Generate a key pair:
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

## � Security Features

- **JWT authentication:** RS256 signed access tokens, public key validation in API Gateway
- **Cookie-based WebSocket auth:** Seamless real-time communication authentication
- **Rate-limited verification/2FA attempts:** Prevent brute-force attacks on codes
- **Account lockout:** Login is blocked for locked users

---

## 🧑‍💻 2FA Support

- **Email 2FA:** Request and verify codes via Auth Service
- **TOTP 2FA:** Setup and verify using standard authenticator apps (coming soon!)

---

## 🔌 WebSocket Support

Homenavi includes full WebSocket support for real-time communication:

- **Cookie-based authentication:** WebSockets authenticate using the same JWT tokens as REST APIs
- **Echo Service:** Python WebSocket service for testing and demonstration
- **Test Script:** `test-websocket.py` provides comprehensive WebSocket testing capabilities

---

## ❓ FAQ

**Q: Can I run Homenavi on a Raspberry Pi?**
A: Yes! All services are built for multi-arch Docker images.

**Q: How do I add a new device/service?**
A: Build a new microservice, register it in the API Gateway, and add it to Docker Compose.

**Q: Is it production-ready?**
A: Homenavi is under active development. The core authentication and user management features are stable, but always review the code for your specific use case.

**Q: How do I monitor and trace requests?**
A: Use Prometheus and Grafana for metrics, and Jaeger for distributed tracing. All are included in the default Docker Compose stack.

**Q: Does it support real-time communication?**
A: Yes! WebSocket support is built-in with cookie-based JWT authentication. See the echo service for examples.

---

## 💡 Notes
- This README reflects the current codebase and workflows.
- PRs and contributions are welcome!

## License

MIT License  
Copyright (c) 2025 Adam Peto

See [LICENSE](LICENSE) for details.

---
