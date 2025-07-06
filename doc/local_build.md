# Local Build Guide

This guide explains how to build Homenavi services locally, without Docker Compose.

---

## Prerequisites
- Go (latest recommended)
- Node.js & npm (for frontend)
- Docker (optional, for manual image builds)

---

## Backend Services

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

## Frontend

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

For most users, we recommend using Docker Compose for a seamless experience!