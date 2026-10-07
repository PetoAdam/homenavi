FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY shared ./shared
COPY mcp-service ./mcp-service
RUN cd mcp-service && GOWORK=off go mod download
RUN cd mcp-service && CGO_ENABLED=0 GOOS=linux GOWORK=off go build -o /out/mcp-service ./cmd/mcp-service

FROM alpine:3.20
RUN addgroup -S homenavi && adduser -S homenavi -G homenavi
USER homenavi
COPY --from=build /out/mcp-service /usr/local/bin/mcp-service
EXPOSE 8096
ENTRYPOINT ["/usr/local/bin/mcp-service"]