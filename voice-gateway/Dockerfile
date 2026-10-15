# syntax=docker/dockerfile:1
FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY voice-gateway/go.mod ./
COPY voice-gateway/cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/voice-gateway ./cmd/voice-gateway

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/voice-gateway /usr/local/bin/voice-gateway
EXPOSE 8090
ENTRYPOINT ["voice-gateway"]