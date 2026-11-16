// Package api is generated from shared/openapi.yaml. restgen writes the
// spec's REST endpoints for synced rows first, from the entity registry.
package api

//go:generate go run ../restgen -spec ../../../../shared/openapi.yaml -go ../rows_gen.go
//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.1 -config oapi-codegen.yaml ../../../../shared/openapi.yaml
