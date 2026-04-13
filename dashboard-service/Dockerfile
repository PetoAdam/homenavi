# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY dashboard-service ./dashboard-service
WORKDIR /src/dashboard-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/dashboard-service ./cmd/dashboard-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/dashboard-service /app/dashboard-service
EXPOSE 8097
ENTRYPOINT ["/app/dashboard-service"]
