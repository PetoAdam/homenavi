# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS build
WORKDIR /src
COPY shared ./shared
COPY integration-proxy ./integration-proxy
WORKDIR /src/integration-proxy
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/integration-proxy ./cmd/integration-proxy

FROM alpine:3.22
RUN apk add --no-cache docker-cli docker-cli-compose helm kubectl
WORKDIR /app
COPY --from=build /out/integration-proxy /app/integration-proxy
EXPOSE 8099
ENTRYPOINT ["/app/integration-proxy"]
