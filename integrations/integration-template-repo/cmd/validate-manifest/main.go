package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func main() {
	manifestPath := os.Getenv("MANIFEST")
	if manifestPath == "" {
		manifestPath = "manifest/homenavi-integration.json"
	}
	schemaPath := os.Getenv("SCHEMA")
	if schemaPath == "" {
		schemaPath = "spec/homenavi-integration.schema.json"
	}

	schemaPath = filepath.Clean(schemaPath)
	schemaBytes, err := os.ReadFile(schemaPath) // #nosec G304 -- path comes from env/config
	if err != nil {
		log.Fatalf("read schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", bytes.NewReader(schemaBytes)); err != nil {
		log.Fatalf("add schema resource: %v", err)
	}
	schema, err := compiler.Compile("schema.json")
	if err != nil {
		log.Fatalf("compile schema: %v", err)
	}

	manifestPath = filepath.Clean(manifestPath)
	manifestBytes, err := os.ReadFile(manifestPath) // #nosec G304 -- path comes from env/config
	if err != nil {
		log.Fatalf("read manifest: %v", err)
	}
	var any interface{}
	if err := json.Unmarshal(manifestBytes, &any); err != nil {
		log.Fatalf("parse manifest JSON: %v", err)
	}

	if err := schema.Validate(any); err != nil {
		log.Fatalf("manifest invalid: %v", err)
	}

	fmt.Println("manifest ok")
}
