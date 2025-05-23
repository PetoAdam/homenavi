# Homenavi

[![Build Frontend Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml)

A microservice-based smart home solution.

---

## Overview

**Homenavi** is a modern, extensible smart home platform designed for flexibility, privacy, and ease of use. It leverages a microservice architecture to integrate devices, automation, and user interfaces, making it easy to manage your smart home from anywhere.

- **Frontend:** Fast, installable PWA built with React and Vite.
- **Backend:** (Planned) Modular microservices for device integration, automation, and user management.
- **Deployment:** Docker-first, with CI/CD for rapid iteration.

---

## Features

- 🏠 **Smart Home Dashboard:** Control and monitor devices, sensors, and automations.
- ⚡️ **Fast Frontend:** Built with Vite and React for instant feedback and smooth UX.
- 📱 **PWA:** Installable as a web app on desktop and mobile.
- 🎨 **Modern UI:** Glassmorphism, responsive layouts, and dark mode.
- 🔌 **Device Integration:** (Planned) Zigbee, WiFi, and more.
- 🔄 **Microservice Architecture:** Scalable and maintainable backend (coming soon).
- 🐳 **Dockerized:** Easy to build, run, and deploy anywhere.
- 🚦 **CI/CD:** Automated Docker builds and artifact publishing.

---

## Architecture

```
homenavi/
├── Frontend/         # React PWA (Vite, Tailwind, FontAwesome)
│   ├── src/
│   ├── public/
│   ├── Dockerfile
│   └── ...
├── backend/          # (Planned) Microservices for devices, automations, etc.
├── docker-compose.yml
└── README.md
```

- **Frontend:** User interface for dashboards, device control, and settings.
- **Backend:** (Planned) REST/gRPC APIs, device adapters, automation engine.
- **DevOps:** Docker Compose for local development and deployment.

---

## Quick Start

### Prerequisites

- [Node.js](https://nodejs.org/) (v18+)
- [npm](https://www.npmjs.com/)
- [Docker](https://www.docker.com/) (for containerized deployment)

### Run Frontend Locally

```bash
cd Frontend
npm install
npm run dev
```

Visit [http://localhost:5173](http://localhost:5173).

### Build & Run with Docker Compose

```bash
docker-compose up --build
```

- Frontend served at [http://localhost:5173](http://localhost:5173)
- (Backend services coming soon)

---

## Continuous Integration

This repository uses GitHub Actions for CI/CD.

- **Frontend Docker Build:**  
  [![Build Frontend Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml)

---

## Project Status

- **Frontend:** MVP dashboard, device cards, Spotify integration, map, and more.
- **Backend:** In planning/design phase.
- **Contributions:** Welcome! See [Frontend/README.md](Frontend/README.md) for details.

---

## License

MIT License  
Copyright (c) 2025 Adam Peto

See [LICENSE](LICENSE) for details.

---
