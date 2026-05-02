# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY matter-adapter ./matter-adapter
WORKDIR /src/matter-adapter
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/matter-adapter ./cmd/matter-adapter

FROM alpine:3.22
RUN apk add --no-cache ca-certificates curl
WORKDIR /app
COPY --from=builder /out/matter-adapter /app/matter-adapter
COPY matter-adapter/scripts/matter-commissioner-wrapper.sh /app/bin/matter-commissioner-wrapper
RUN chmod +x /app/bin/matter-commissioner-wrapper
EXPOSE 8096
ENTRYPOINT ["/app/matter-adapter"]
