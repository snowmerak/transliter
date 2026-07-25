package restapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
	"gopkg.in/yaml.v3"
)

func TestDocumentationEndpointsArePublicAndConsistent(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"owner": "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(1)
	handler, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	jsonResponse, err := http.Get(server.URL + "/openapi.json")
	if err != nil {
		t.Fatal(err)
	}
	defer jsonResponse.Body.Close()
	if jsonResponse.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI JSON returned %d", jsonResponse.StatusCode)
	}
	if jsonResponse.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected JSON content type: %q", jsonResponse.Header.Get("Content-Type"))
	}
	var jsonDocument map[string]any
	if err := json.NewDecoder(jsonResponse.Body).Decode(&jsonDocument); err != nil {
		t.Fatal(err)
	}
	assertDocumentRoutes(t, jsonDocument)

	yamlResponse, err := http.Get(server.URL + "/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer yamlResponse.Body.Close()
	if yamlResponse.StatusCode != http.StatusOK {
		t.Fatalf("OpenAPI YAML returned %d", yamlResponse.StatusCode)
	}
	var yamlDocument map[string]any
	if err := yaml.NewDecoder(yamlResponse.Body).Decode(&yamlDocument); err != nil {
		t.Fatal(err)
	}
	assertDocumentRoutes(t, yamlDocument)

	docsResponse, err := http.Get(server.URL + "/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer docsResponse.Body.Close()
	if docsResponse.StatusCode != http.StatusOK {
		t.Fatalf("Scalar documentation returned %d", docsResponse.StatusCode)
	}
	html, err := io.ReadAll(docsResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(html)
	if !strings.Contains(page, "@scalar/api-reference@"+scalarVersion) {
		t.Fatal("Scalar package version is not pinned in documentation page")
	}
	if !strings.Contains(page, "url: '/openapi.json'") {
		t.Fatal("Scalar page does not use the served OpenAPI document")
	}
}

func assertDocumentRoutes(t *testing.T, document map[string]any) {
	t.Helper()
	if document["openapi"] != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version: %v", document["openapi"])
	}
	paths, ok := document["paths"].(map[string]any)
	if !ok {
		t.Fatal("OpenAPI paths are missing")
	}
	for _, path := range []string{
		"/v1/jobs",
		"/v1/jobs/{id}",
		"/healthz",
		"/openapi.json",
		"/openapi.yaml",
		"/docs",
	} {
		if _, ok := paths[path]; !ok {
			t.Errorf("OpenAPI path %q is missing", path)
		}
	}
}
