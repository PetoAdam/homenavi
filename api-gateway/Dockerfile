# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o api-gateway

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/api-gateway .
COPY --from=builder /app/config ./config
EXPOSE 8080
CMD ["./api-gateway"]
