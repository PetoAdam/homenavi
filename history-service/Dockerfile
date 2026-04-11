FROM golang:1.26.1-alpine3.22 AS builder
WORKDIR /src
COPY shared ./shared
COPY history-service ./history-service
WORKDIR /src/history-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/history-service ./cmd/history-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=builder /out/history-service /app/history-service
EXPOSE 8093
ENTRYPOINT ["/app/history-service"]
