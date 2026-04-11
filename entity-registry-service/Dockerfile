FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY entity-registry-service ./entity-registry-service
WORKDIR /src/entity-registry-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/entity-registry-service ./cmd/entity-registry-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/entity-registry-service /app/entity-registry-service
EXPOSE 8095
ENTRYPOINT ["/app/entity-registry-service"]
