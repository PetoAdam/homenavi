# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY weather-service ./weather-service
WORKDIR /src/weather-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/weather-service ./cmd/weather-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/weather-service /app/weather-service
EXPOSE 8095
ENTRYPOINT ["/app/weather-service"]
