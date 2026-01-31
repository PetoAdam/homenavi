# syntax=docker/dockerfile:1

FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . ./
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/integration-proxy ./cmd/integration-proxy

FROM alpine:3.20
RUN adduser -D -H -u 10001 app
USER app
WORKDIR /app
COPY --from=build /out/integration-proxy /app/integration-proxy
EXPOSE 8099
ENTRYPOINT ["/app/integration-proxy"]
