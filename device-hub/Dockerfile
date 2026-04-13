# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY device-hub ./device-hub
WORKDIR /src/device-hub
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/device-hub ./cmd/device-hub

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/device-hub /app/device-hub
EXPOSE 8090
ENTRYPOINT ["/app/device-hub"]
