module github.com/PetoAdam/homenavi/weather-service

go 1.26.0

toolchain go1.26.1

require (
	github.com/PetoAdam/homenavi/shared v0.0.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/go-chi/cors v1.2.1
)

replace github.com/PetoAdam/homenavi/shared => ../shared
