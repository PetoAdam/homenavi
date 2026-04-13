# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY email-service ./email-service
WORKDIR /src/email-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/email-service ./cmd/email-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/email-service /app/email-service
EXPOSE 8002
ENTRYPOINT ["/app/email-service"]