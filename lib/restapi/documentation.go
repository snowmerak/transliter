package restapi

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"gopkg.in/yaml.v3"
)

const scalarVersion = "1.63.0"

//go:embed openapi.yaml
var openAPIYAML []byte

var (
	openAPIOnce sync.Once
	openAPIJSON []byte
	openAPIErr  error
)

func registerDocumentation(mux *http.ServeMux) error {
	if _, err := openAPIDocumentJSON(); err != nil {
		return err
	}
	mux.HandleFunc("GET /openapi.json", serveOpenAPIJSON)
	mux.HandleFunc("GET /openapi.yaml", serveOpenAPIYAML)
	mux.HandleFunc("GET /docs", serveScalar)
	return nil
}

func openAPIDocumentJSON() ([]byte, error) {
	openAPIOnce.Do(func() {
		var document map[string]any
		if err := yaml.Unmarshal(openAPIYAML, &document); err != nil {
			openAPIErr = fmt.Errorf("parse embedded OpenAPI document: %w", err)
			return
		}
		if document["openapi"] != "3.1.0" {
			openAPIErr = fmt.Errorf("embedded OpenAPI document must use version 3.1.0")
			return
		}
		if _, ok := document["paths"].(map[string]any); !ok {
			openAPIErr = fmt.Errorf("embedded OpenAPI document has no paths")
			return
		}
		openAPIJSON, openAPIErr = json.MarshalIndent(document, "", "  ")
		if openAPIErr != nil {
			openAPIErr = fmt.Errorf("encode embedded OpenAPI document: %w", openAPIErr)
		}
	})
	return openAPIJSON, openAPIErr
}

func serveOpenAPIJSON(writer http.ResponseWriter, _ *http.Request) {
	document, err := openAPIDocumentJSON()
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "OpenAPI document unavailable")
		return
	}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = writer.Write(document)
}

func serveOpenAPIYAML(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "application/yaml")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = writer.Write(openAPIYAML)
}

func serveScalar(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	writer.Header().Set("Referrer-Policy", "no-referrer")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = writer.Write([]byte(scalarHTML))
}

const scalarHTML = `<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8">
    <meta name="viewport" content="width=device-width, initial-scale=1">
    <meta name="description" content="transliter REST API reference">
    <title>transliter API Reference</title>
  </head>
  <body>
    <div id="app"></div>
    <noscript>Enable JavaScript to view the interactive API reference.</noscript>
    <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@` + scalarVersion + `"></script>
    <script>
      Scalar.createApiReference('#app', {
        url: '/openapi.json',
        theme: 'purple',
        layout: 'modern',
        hideClientButton: false,
        defaultHttpClient: { targetKey: 'shell', clientKey: 'curl' }
      })
    </script>
  </body>
</html>
`
