# syntax=docker/dockerfile:1
FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY auth-service ./auth-service
WORKDIR /src/auth-service
RUN GOWORK=off go mod download
RUN GOWORK=off go build -o /out/auth-service

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/auth-service ./auth-service
EXPOSE 8000
CMD ["./auth-service"]
