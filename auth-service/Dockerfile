# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o auth-service

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/auth-service .
EXPOSE 8000
CMD ["./auth-service"]
