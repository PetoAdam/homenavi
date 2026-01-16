FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/weather-service ./cmd/weather-service

FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/weather-service /app/weather-service
EXPOSE 8095
ENTRYPOINT ["/app/weather-service"]
