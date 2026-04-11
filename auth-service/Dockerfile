# syntax=docker/dockerfile:1
FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY auth-service ./auth-service
WORKDIR /src/auth-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/auth-service ./cmd/auth-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/auth-service ./auth-service
EXPOSE 8000
CMD ["./auth-service"]
