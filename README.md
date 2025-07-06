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

---

## 🏗️ Project Structure

```
frontend/           # React frontend
api-gateway/        # Go API gateway
auth-service/       # Go authentication service
user-service/       # Go user service
.github/workflows/  # GitHub Actions workflows
```

---

## 🧩 Architecture

Homenavi is built as a set of loosely coupled microservices:
- **Frontend:** Modern React SPA, served by Nginx.
- **API Gateway:** Central entrypoint, JWT auth, request routing.
- **Auth Service:** Handles authentication, issues JWTs.
- **User Service:** Manages user data and validation.

All services communicate via HTTP/REST. Add your own services and plug them into the gateway!

---

## 🐳 Quickstart (Recommended)

The fastest way to get Homenavi running is with Docker Compose. Just clone, configure, and launch:

```sh
git clone https://github.com/PetoAdam/homenavi.git
cd homenavi
cp .env.example .env  # Edit as needed
# Then:
docker compose up --build
```

- All services will be built and started automatically.
- Edit `.env` for secrets and service URLs.
- See [`/doc/local_build.md`](doc/local_build.md) for advanced/local development.

---

## 🛠️ Extending Homenavi

Want to add a new service? Just drop it in, add to `docker-compose.yml`, and go! The API Gateway makes integration easy.

- Use Go, Python, Node.js, or any language you like.
- Register new routes in the API Gateway.
- Share authentication via JWT.
- Check out the [Extending Guide](doc/extending.md) (coming soon!)

---

## 📦 CI/CD

- All services are built and tested via GitHub Actions on every push/PR.
- Docker images are saved as artifacts for easy deployment.
- Workflows for each service live in `.github/workflows/`.
- Add your own workflow for new services!

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

---

## ❓ FAQ

**Q: Can I run Homenavi on a Raspberry Pi?**
A: Yes! All services are built for multi-arch Docker images.

**Q: How do I add a new device/service?**
A: Build a new microservice, register it in the API Gateway, and add it to Docker Compose.

**Q: Is it production-ready?**
A: Homenavi is under active development. Contributions and feedback are welcome!

---

## 💡 Notes
- This README reflects the current codebase and workflows.
- PRs and contributions are welcome!

## License

MIT License  
Copyright (c) 2025 Adam Peto

See [LICENSE](LICENSE) for details.

---
