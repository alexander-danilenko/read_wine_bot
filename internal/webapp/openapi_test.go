package webapp_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"github.com/alexander-danilenko/read_wine_bot/internal/webapp"
)

// specPath is the committed spec the Mini App generates its API types from.
const specPath = "../../web/openapi.json"

// TestOpenAPISpec keeps web/openapi.json in step with the operations. After changing the API,
// run UPDATE_OPENAPI=1 go test ./internal/webapp/ -run OpenAPI to rewrite it.
func TestOpenAPISpec(t *testing.T) {
	api := webapp.NewAPI(http.NewServeMux(), "spec")
	(&webapp.Operations{}).Register(api)

	got, err := json.MarshalIndent(api.OpenAPI(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	got = append(got, '\n')

	if os.Getenv("UPDATE_OPENAPI") != "" {
		if err = os.WriteFile(specPath, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatalf("read %s: %v (run with UPDATE_OPENAPI=1 to create it)", specPath, err)
	}

	if !bytes.Equal(got, want) {
		t.Errorf("%s is out of date: run UPDATE_OPENAPI=1 go test ./internal/webapp/ -run OpenAPI", specPath)
	}
}
