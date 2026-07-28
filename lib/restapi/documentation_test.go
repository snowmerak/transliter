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
	"github.com/snowmerak/transliter/models/catalog"
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
		Catalog:       catalog.Resolver{},
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

	uiResponse, err := http.Get(server.URL + "/ui/")
	if err != nil {
		t.Fatal(err)
	}
	defer uiResponse.Body.Close()
	if uiResponse.StatusCode != http.StatusOK {
		t.Fatalf("web UI returned %d", uiResponse.StatusCode)
	}
	uiHTML, err := io.ReadAll(uiResponse.Body)
	if err != nil {
		t.Fatal(err)
	}
	pageUI := string(uiHTML)
	if !strings.Contains(pageUI, "Job console") {
		t.Fatal("web UI page missing expected title copy")
	}
	if !strings.Contains(pageUI, "api-key") {
		t.Fatal("web UI page missing optional API key field")
	}

	for _, asset := range []struct {
		path        string
		contentType string
		snippet     string
	}{
		{path: "/ui/app.js", contentType: "javascript", snippet: "buildPayload"},
		{path: "/ui/app.css", contentType: "text/css", snippet: ".ui-shell"},
		{path: "/ui/vendor/style.css", contentType: "text/css", snippet: `./styles/tokens.css`},
		{path: "/ui/vendor/styles/tokens.css", contentType: "text/css", snippet: "--mp-bg-canvas"},
	} {
		response, err := http.Get(server.URL + asset.path)
		if err != nil {
			t.Fatalf("GET %s: %v", asset.path, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatalf("read %s: %v", asset.path, err)
		}
		if response.StatusCode != http.StatusOK {
			t.Fatalf("%s status=%d", asset.path, response.StatusCode)
		}
		if ct := response.Header.Get("Content-Type"); !strings.Contains(ct, asset.contentType) {
			t.Fatalf("%s content-type=%q want substring %q", asset.path, ct, asset.contentType)
		}
		if !strings.Contains(string(body), asset.snippet) {
			t.Fatalf("%s missing snippet %q", asset.path, asset.snippet)
		}
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
		"/v1/models",
		"/v1/model-catalogs",
		"/v1/model-catalogs/{id}",
		"/v1/model-catalogs/{id}/preview",
		"/healthz",
		"/openapi.json",
		"/openapi.yaml",
		"/docs",
		"/ui",
		"/ui/",
	} {
		if _, ok := paths[path]; !ok {
			t.Errorf("OpenAPI path %q is missing", path)
		}
	}
}
