FROM golang:1.26.1-alpine3.22 AS build
WORKDIR /src
COPY shared ./shared
COPY dashboard-service ./dashboard-service
WORKDIR /src/dashboard-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOWORK=off go build -mod=mod -o /out/dashboard-service ./cmd/dashboard-service

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/dashboard-service /app/dashboard-service
EXPOSE 8097
ENTRYPOINT ["/app/dashboard-service"]
