FROM golang:1.26.1-alpine3.22 AS build
WORKDIR /src
COPY shared ./shared
COPY weather-service ./weather-service
WORKDIR /src/weather-service
RUN GOWORK=off go mod download
RUN CGO_ENABLED=0 GOWORK=off go build -o /out/weather-service ./cmd/weather-service

FROM alpine:3.22
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/weather-service /app/weather-service
EXPOSE 8095
ENTRYPOINT ["/app/weather-service"]
