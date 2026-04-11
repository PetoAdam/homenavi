# syntax=docker/dockerfile:1
FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY api-gateway ./api-gateway
WORKDIR /src/api-gateway
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/api-gateway ./cmd/api-gateway

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/api-gateway ./api-gateway
COPY --from=builder /src/api-gateway/config ./config
EXPOSE 8080
CMD ["./api-gateway"]
