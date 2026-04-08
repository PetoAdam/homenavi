# syntax=docker/dockerfile:1
FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY api-gateway ./api-gateway
WORKDIR /src/api-gateway
RUN GOWORK=off go mod download
RUN GOWORK=off go build -o /out/api-gateway

FROM alpine:3.22
WORKDIR /app
COPY --from=builder /out/api-gateway ./api-gateway
COPY --from=builder /src/api-gateway/config ./config
EXPOSE 8080
CMD ["./api-gateway"]
