module github.com/PetoAdam/homenavi/integration-proxy

go 1.26.0

toolchain go1.26.1

require (
	github.com/PetoAdam/homenavi/shared v0.0.0
	github.com/golang-jwt/jwt/v5 v5.2.1
	github.com/santhosh-tekuri/jsonschema/v5 v5.3.1
	golang.org/x/mod v0.23.0
	gopkg.in/yaml.v3 v3.0.1
)

replace github.com/PetoAdam/homenavi/shared => ../shared
