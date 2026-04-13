# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY thread-adapter ./thread-adapter
WORKDIR /src/thread-adapter
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/thread-adapter ./cmd/thread-adapter

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/thread-adapter /app/thread-adapter
EXPOSE 8092
ENTRYPOINT ["/app/thread-adapter"]
