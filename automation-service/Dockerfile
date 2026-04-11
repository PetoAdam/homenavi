FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY automation-service ./automation-service
WORKDIR /src/automation-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/automation-service ./cmd/automation-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/automation-service /app/automation-service
EXPOSE 8094
ENTRYPOINT ["/app/automation-service"]
