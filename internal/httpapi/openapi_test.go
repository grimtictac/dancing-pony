package httpapi

import (
	"slices"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"

	"the-dancing-pony-v2/internal/apidocs"
)

// TestOpenAPIListsEveryRoute keeps the published spec honest: every route
// the API serves is documented, and every documented route exists.
func TestOpenAPIListsEveryRoute(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(apidocs.Spec, &spec); err != nil {
		t.Fatalf("openapi.yaml: %v", err)
	}
	var documented []string
	for path, item := range spec.Paths {
		for method := range item {
			switch method {
			case "get", "put", "post", "delete", "patch":
				documented = append(documented, strings.ToUpper(method)+" "+path)
			}
		}
	}

	var served []string
	for _, rt := range (&api{}).routes() {
		// "GET /{$}" is Go's pattern for exactly "/".
		served = append(served, strings.Replace(rt.pattern, "{$}", "", 1))
	}

	for _, r := range served {
		if !slices.Contains(documented, r) {
			t.Errorf("%s is served but not in openapi.yaml", r)
		}
	}
	for _, d := range documented {
		if !slices.Contains(served, d) {
			t.Errorf("%s is in openapi.yaml but not served", d)
		}
	}
}
