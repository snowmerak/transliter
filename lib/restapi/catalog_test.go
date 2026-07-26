package restapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
	"github.com/snowmerak/transliter/models/catalog"
)

func TestModelCatalogListDetailAndPreview(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"alice": "alice-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(1)
	routes, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Catalog:       catalog.Resolver{},
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	listResponse, err := http.Get(server.URL + "/v1/model-catalogs")
	if err != nil {
		t.Fatal(err)
	}
	defer listResponse.Body.Close()
	if listResponse.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d", listResponse.StatusCode)
	}
	var list struct {
		ModelCatalogs []struct {
			ID           string `json:"id"`
			Family       string `json:"family"`
			Capabilities struct {
				PromptKinds           []string `json:"prompt_kinds"`
				StructuredUserContent bool     `json:"structured_user_content"`
				MaxInputTokens        int      `json:"max_input_tokens"`
				AuxiliaryFields       struct {
					Glossary bool `json:"glossary"`
					Style    bool `json:"style"`
				} `json:"auxiliary_fields"`
			} `json:"capabilities"`
			Profiles  []string `json:"profiles"`
			Languages []string `json:"languages"`
		} `json:"model_catalogs"`
	}
	if err := json.NewDecoder(listResponse.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.ModelCatalogs) != 6 {
		t.Fatalf("expected 6 catalogs, got %d", len(list.ModelCatalogs))
	}

	var hy, gemma bool
	for _, item := range list.ModelCatalogs {
		switch item.ID {
		case "hymt2-1.8b":
			hy = true
			if !item.Capabilities.AuxiliaryFields.Glossary || !item.Capabilities.AuxiliaryFields.Style {
				t.Fatalf("hy auxiliary fields = %+v", item.Capabilities.AuxiliaryFields)
			}
			if item.Capabilities.StructuredUserContent {
				t.Fatal("hy should not use structured user content")
			}
			if len(item.Profiles) != 2 || len(item.Languages) == 0 {
				t.Fatalf("hy profiles/languages incomplete: %+v", item)
			}
		case "translategemma-12b":
			gemma = true
			if item.Capabilities.AuxiliaryFields.Glossary || item.Capabilities.AuxiliaryFields.Style {
				t.Fatalf("gemma auxiliary fields should be false: %+v", item.Capabilities.AuxiliaryFields)
			}
			if !item.Capabilities.StructuredUserContent {
				t.Fatal("gemma should use structured user content")
			}
			if item.Capabilities.MaxInputTokens != 2048 {
				t.Fatalf("gemma max_input_tokens=%d", item.Capabilities.MaxInputTokens)
			}
		}
	}
	if !hy || !gemma {
		t.Fatalf("missing expected catalogs in %+v", list.ModelCatalogs)
	}

	detailResponse, err := http.Get(server.URL + "/v1/model-catalogs/translategemma-12b")
	if err != nil {
		t.Fatal(err)
	}
	defer detailResponse.Body.Close()
	if detailResponse.StatusCode != http.StatusOK {
		t.Fatalf("detail status=%d", detailResponse.StatusCode)
	}
	var detail struct {
		ID             string `json:"id"`
		ProfileOptions map[string]struct {
			DoSample        *bool `json:"do_sample"`
			MaxOutputTokens int   `json:"max_output_tokens"`
		} `json:"profile_options"`
	}
	if err := json.NewDecoder(detailResponse.Body).Decode(&detail); err != nil {
		t.Fatal(err)
	}
	if detail.ID != "translategemma-12b" {
		t.Fatalf("detail id=%q", detail.ID)
	}
	official, ok := detail.ProfileOptions["official"]
	if !ok || official.DoSample == nil || *official.DoSample || official.MaxOutputTokens != 200 {
		t.Fatalf("unexpected official options: %+v", detail.ProfileOptions)
	}

	previewBody := `{
		"profile":"official",
		"translation":{
			"source":"The service is ready.",
			"source_language":"English",
			"target_language":"Korean",
			"kind":"text"
		}
	}`
	previewResponse, err := http.Post(
		server.URL+"/v1/model-catalogs/translategemma-12b/preview",
		"application/json",
		bytes.NewBufferString(previewBody),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer previewResponse.Body.Close()
	if previewResponse.StatusCode != http.StatusOK {
		t.Fatalf("preview status=%d", previewResponse.StatusCode)
	}
	var preview struct {
		ModelCatalog string `json:"model_catalog"`
		Profile      string `json:"profile"`
		Input        struct {
			Messages []struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		} `json:"input"`
	}
	if err := json.NewDecoder(previewResponse.Body).Decode(&preview); err != nil {
		t.Fatal(err)
	}
	if preview.ModelCatalog != "translategemma-12b" || preview.Profile != "official" {
		t.Fatalf("unexpected preview header: %+v", preview)
	}
	if len(preview.Input.Messages) != 1 || preview.Input.Messages[0].Role != "user" {
		t.Fatalf("unexpected preview messages: %+v", preview.Input.Messages)
	}
	var parts []map[string]any
	if err := json.Unmarshal(preview.Input.Messages[0].Content, &parts); err != nil {
		t.Fatalf("gemma preview content should be structured parts: %v %s", err, preview.Input.Messages[0].Content)
	}
	if len(parts) != 1 || parts[0]["source_lang_code"] != "en" || parts[0]["target_lang_code"] != "ko" {
		t.Fatalf("unexpected structured parts: %+v", parts)
	}

	hyPreview, err := http.Post(
		server.URL+"/v1/model-catalogs/hymt2-1.8b/preview",
		"application/json",
		bytes.NewBufferString(previewBody),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer hyPreview.Body.Close()
	if hyPreview.StatusCode != http.StatusOK {
		t.Fatalf("hy preview status=%d", hyPreview.StatusCode)
	}
	var hyBody struct {
		Input struct {
			Messages []struct {
				Content json.RawMessage `json:"content"`
			} `json:"messages"`
		} `json:"input"`
	}
	if err := json.NewDecoder(hyPreview.Body).Decode(&hyBody); err != nil {
		t.Fatal(err)
	}
	var prompt string
	if err := json.Unmarshal(hyBody.Input.Messages[0].Content, &prompt); err != nil {
		t.Fatalf("hy preview content should be string: %v %s", err, hyBody.Input.Messages[0].Content)
	}
	if prompt == "" || !strings.Contains(prompt, "The service is ready.") {
		t.Fatalf("unexpected hy prompt: %q", prompt)
	}

	badJSON, err := http.Post(
		server.URL+"/v1/model-catalogs/hymt2-1.8b/preview",
		"application/json",
		bytes.NewBufferString(`{"profile":"official","extra":true,"translation":{"target_language":"Korean"}}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer badJSON.Body.Close()
	if badJSON.StatusCode != http.StatusBadRequest {
		t.Fatalf("unknown field status=%d", badJSON.StatusCode)
	}

	missing, err := http.Get(server.URL + "/v1/model-catalogs/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing catalog status=%d", missing.StatusCode)
	}
}
