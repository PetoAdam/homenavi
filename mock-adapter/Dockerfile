# syntax=docker/dockerfile:1

FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY mock-adapter ./mock-adapter
WORKDIR /src/mock-adapter
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/mock-adapter ./cmd/mock-adapter

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/mock-adapter /app/mock-adapter
EXPOSE 8092
ENTRYPOINT ["/app/mock-adapter"]
