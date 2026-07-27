package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/snowmerak/transliter/lib/inference/openai"
	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/restapi"
	"github.com/snowmerak/transliter/models/catalog"
)

const (
	integrationBaseURL = "http://macmini:11888/v1"
	integrationAPIKey  = "integration-test-key"
)

// Default live cases for go test ./... when macmini:11888 is up.
// Extended Hy sizes / TranslateGemma are opt-in (env-gated) smokes.
var integrationModelCases = []struct {
	name               string
	catalogModel       string
	providerCandidates []string
}{
	{
		name:               "hymt2-1.8b",
		catalogModel:       "hymt2-1.8b",
		providerCandidates: []string{"Hy-MT2-1.8B", "hy-mt2-1.8b"},
	},
	{
		name:               "hymt2-7b",
		catalogModel:       "hymt2-7b",
		providerCandidates: []string{"Hy-MT2-7B-bf16", "hy-mt2-7b"},
	},
}

// Opt-in (TRANSLITER_INTEGRATION_HY_EXTENDED=1). 30B currently fails to load on
// macmini:11888 without trust_remote_code for hy_v3.py.
var hyExtendedDiagnosticCases = []struct {
	name               string
	catalogModel       string
	providerCandidates []string
}{
	{
		name:               "hymt2-30b-a3b",
		catalogModel:       "hymt2-30b-a3b",
		providerCandidates: []string{"Hy-MT2-30B-A3B-MLX-4bit", "hy-mt2-30b-a3b-mlx", "hy-mt2-30b-a3b"},
	},
}

// Opt-in only (TRANSLITER_INTEGRATION_TRANSLATEGEMMA=1).
// Uses the openai client completions bypass for structured TranslateGemma parts.
var translateGemmaDiagnosticCases = []struct {
	name               string
	catalogModel       string
	providerCandidates []string
}{
	{
		name:               "translategemma-4b",
		catalogModel:       "translategemma-4b",
		providerCandidates: []string{"translategemma-4b-it-4bit", "translategemma-4b-it-8bit", "translategemma-4b-it", "translategemma-4b"},
	},
	{
		name:               "translategemma-12b",
		catalogModel:       "translategemma-12b",
		providerCandidates: []string{"translategemma-12b-it-8bit", "translategemma-12b-it-4bit", "translategemma-12b-it", "translategemma-12b"},
	},
	{
		name:               "translategemma-27b",
		catalogModel:       "translategemma-27b",
		providerCandidates: []string{"translategemma-27b-it-8bit", "translategemma-27b-it", "translategemma-27b"},
	},
}

func requireIntegrationModels(t *testing.T) map[string]struct{} {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, integrationBaseURL+"/models", nil)
	if err != nil {
		t.Skipf("macmini inference unavailable: %v", err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Skipf("macmini inference unavailable: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Skipf("macmini inference unavailable: status %d", response.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Skipf("macmini inference unavailable: decode models: %v", err)
	}

	ids := make(map[string]struct{}, len(payload.Data))
	for _, model := range payload.Data {
		id := strings.TrimSpace(model.ID)
		if id == "" {
			continue
		}
		ids[id] = struct{}{}
	}
	if len(ids) == 0 {
		t.Skip("no models on inference server")
	}
	return ids
}

func resolveProviderModel(available map[string]struct{}, candidates []string) (string, bool) {
	if override := strings.TrimSpace(os.Getenv("TRANSLITER_API_MODEL")); override != "" {
		for _, candidate := range candidates {
			if override == candidate {
				if _, ok := available[override]; ok {
					return override, true
				}
				return "", false
			}
		}
		// Override targets a different model; keep scanning candidates.
	}
	for _, candidate := range candidates {
		if _, ok := available[candidate]; ok {
			return candidate, true
		}
	}
	return "", false
}

func integrationOrigin() string {
	return strings.TrimSuffix(integrationBaseURL, "/v1")
}

func unloadOMLXModel(t *testing.T, modelID string) {
	t.Helper()
	if strings.TrimSpace(modelID) == "" {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	endpoint := integrationOrigin() + "/admin/api/models/" + url.PathEscape(modelID) + "/unload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(nil))
	if err != nil {
		t.Fatalf("create unload request for %s: %v", modelID, err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unload %s: %v", modelID, err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	text := strings.TrimSpace(string(body))
	switch response.StatusCode {
	case http.StatusOK:
		t.Logf("omlx unload %s status=%d body=%s", modelID, response.StatusCode, text)
		return
	case http.StatusBadRequest:
		// Idempotent: already unloaded.
		if strings.Contains(text, "Model not loaded:") {
			t.Logf("omlx unload %s already unloaded: %s", modelID, text)
			return
		}
	}
	t.Fatalf("unload %s status=%d body=%s", modelID, response.StatusCode, text)
}

func listLoadedOMLXModels(t *testing.T) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, integrationOrigin()+"/admin/api/models", nil)
	if err != nil {
		t.Fatalf("create admin models request: %v", err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list admin models: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("list admin models status=%d body=%s", response.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Models []struct {
			ID     string `json:"id"`
			Loaded bool   `json:"loaded"`
		} `json:"models"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode admin models: %v", err)
	}

	loaded := make([]string, 0)
	for _, model := range payload.Models {
		if model.Loaded && strings.TrimSpace(model.ID) != "" {
			loaded = append(loaded, model.ID)
		}
	}
	return loaded
}

// freeOMLXModelSlot unloads every currently loaded model so the next provider
// can cold-load without blowing memory limits.
func freeOMLXModelSlot(t *testing.T) {
	t.Helper()
	for _, modelID := range listLoadedOMLXModels(t) {
		unloadOMLXModel(t, modelID)
	}
}
func TestIntegrationTranslationJobDefaultBackends(t *testing.T) {
	available := requireIntegrationModels(t)
	baseURL, cleanup := startIntegrationStack(t)
	defer cleanup()
	t.Cleanup(func() { freeOMLXModelSlot(t) })

	ran := 0
	for _, tc := range integrationModelCases {
		tc := tc
		providerModel, ok := resolveProviderModel(available, tc.providerCandidates)
		if !ok {
			t.Logf("skip %s: provider not mounted (candidates=%v on %s)", tc.name, tc.providerCandidates, integrationBaseURL)
			continue
		}
		ran++
		t.Run(tc.name, func(t *testing.T) {
			freeOMLXModelSlot(t)
			t.Cleanup(func() { freeOMLXModelSlot(t) })
			runIntegrationTranslationJob(t, baseURL, tc.catalogModel, providerModel)
		})
	}
	if ran == 0 {
		t.Fatal("no catalog provider models available on inference server")
	}
}

// Opt-in diagnostics for Hy sizes that are advertised but may fail to load.
// Enable with TRANSLITER_INTEGRATION_HY_EXTENDED=1.
func TestIntegrationHyExtendedDiagnostics(t *testing.T) {
	if os.Getenv("TRANSLITER_INTEGRATION_HY_EXTENDED") != "1" {
		t.Skip("set TRANSLITER_INTEGRATION_HY_EXTENDED=1 to run extended Hy diagnostics")
	}

	available := requireIntegrationModels(t)
	baseURL, cleanup := startIntegrationStack(t)
	defer cleanup()
	t.Cleanup(func() { freeOMLXModelSlot(t) })

	ran := 0
	for _, tc := range hyExtendedDiagnosticCases {
		tc := tc
		providerModel, ok := resolveProviderModel(available, tc.providerCandidates)
		if !ok {
			t.Logf("skip %s: provider not mounted (candidates=%v on %s)", tc.name, tc.providerCandidates, integrationBaseURL)
			continue
		}
		ran++
		t.Run(tc.name, func(t *testing.T) {
			freeOMLXModelSlot(t)
			t.Cleanup(func() { freeOMLXModelSlot(t) })
			runIntegrationTranslationJob(t, baseURL, tc.catalogModel, providerModel)
		})
	}
	if ran == 0 {
		t.Fatal("no extended Hy provider models mounted on inference server")
	}
}

// Opt-in TranslateGemma smoke against macmini:11888.
// Enable with TRANSLITER_INTEGRATION_TRANSLATEGEMMA=1.
// Structured parts are sent via /v1/completions (client-side chat template),
// not /v1/chat/completions — oMLX rejects the custom content mapping on chat.
func TestIntegrationTranslateGemmaSmoke(t *testing.T) {
	if os.Getenv("TRANSLITER_INTEGRATION_TRANSLATEGEMMA") != "1" {
		t.Skip("set TRANSLITER_INTEGRATION_TRANSLATEGEMMA=1 to run TranslateGemma smoke")
	}

	available := requireIntegrationModels(t)
	baseURL, cleanup := startIntegrationStack(t)
	defer cleanup()
	t.Cleanup(func() { freeOMLXModelSlot(t) })

	ran := 0
	for _, tc := range translateGemmaDiagnosticCases {
		tc := tc
		providerModel, ok := resolveProviderModel(available, tc.providerCandidates)
		if !ok {
			t.Logf("skip %s: provider not mounted (candidates=%v on %s)", tc.name, tc.providerCandidates, integrationBaseURL)
			continue
		}
		ran++
		t.Run(tc.name, func(t *testing.T) {
			freeOMLXModelSlot(t)
			t.Cleanup(func() { freeOMLXModelSlot(t) })
			runIntegrationTranslationJob(t, baseURL, tc.catalogModel, providerModel)
		})
	}
	if ran == 0 {
		t.Fatal("no TranslateGemma provider models mounted on inference server")
	}
}

func startIntegrationStack(t *testing.T) (baseURL string, cleanup func()) {
	t.Helper()

	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"alice": integrationAPIKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	backends, err := buildBackends(ctx, serverConfig{
		QueueBackend:       "nats-embedded",
		StoreBackend:       "sqlite",
		SQLitePath:         filepath.Join(t.TempDir(), "jobs.db"),
		NATSEmbeddedMemory: true,
		JobTimeout:         5 * time.Minute,
	})
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	client, err := openai.New(openai.Config{
		BaseURL: integrationBaseURL,
		Model:   "hy-mt2-1.8b",
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		backends.Close()
		cancel()
		t.Fatal(err)
	}

	processor := jobs.NewTranslationProcessor(catalog.Resolver{}, client)
	schedulerDone := make(chan error, 1)
	scheduler := &jobs.Scheduler{
		Queue:       backends.queue,
		Store:       backends.store,
		Processor:   processor,
		Concurrency: 1,
		JobTimeout:  5 * time.Minute,
	}
	go func() { schedulerDone <- scheduler.Run(ctx) }()

	handler := &restapi.Handler{
		Authenticator: authenticator,
		Queue:         backends.queue,
		Store:         backends.store,
		Catalog:       catalog.Resolver{},
		Retention:     time.Hour,
		Validate:      validateJobRequest,
	}
	routes, err := handler.Routes()
	if err != nil {
		cancel()
		<-schedulerDone
		backends.Close()
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)

	health := integrationAPIRequest(t, http.MethodGet, server.URL+"/healthz", "", "")
	if health.StatusCode != http.StatusOK {
		server.Close()
		cancel()
		<-schedulerDone
		backends.Close()
		t.Fatalf("healthz status=%d body=%s", health.StatusCode, integrationReadBody(t, health))
	}
	var healthBody struct {
		Status string `json:"status"`
	}
	integrationDecodeBody(t, health, &healthBody)
	if healthBody.Status != "ok" {
		server.Close()
		cancel()
		<-schedulerDone
		backends.Close()
		t.Fatalf("unexpected healthz body: %+v", healthBody)
	}

	cleanup = func() {
		server.Close()
		cancel()
		<-schedulerDone
		backends.Close()
	}
	return server.URL, cleanup
}

func runIntegrationTranslationJob(t *testing.T, baseURL, catalogModel, providerModel string) {
	t.Helper()

	createBody := fmt.Sprintf(`{
		"model_catalog": %q,
		"model": %q,
		"profile": "official",
		"translation": {
			"source": "The service is ready.",
			"source_language": "English",
			"target_language": "Korean",
			"kind": "text",
			"glossary": {}
		}
	}`, catalogModel, providerModel)
	create := integrationAPIRequest(
		t,
		http.MethodPost,
		baseURL+"/v1/jobs",
		integrationAPIKey,
		createBody,
	)
	if create.StatusCode != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", create.StatusCode, integrationReadBody(t, create))
	}

	var created jobs.Job
	integrationDecodeBody(t, create, &created)
	if created.ID == "" || created.Status != jobs.StatusQueued {
		t.Fatalf("unexpected created job: %+v", created)
	}

	var completed jobs.Job
	deadline := time.Now().Add(5 * time.Minute)
	for {
		response := integrationAPIRequest(
			t,
			http.MethodGet,
			baseURL+"/v1/jobs/"+created.ID,
			integrationAPIKey,
			"",
		)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get status=%d body=%s", response.StatusCode, integrationReadBody(t, response))
		}
		integrationDecodeBody(t, response, &completed)
		switch completed.Status {
		case jobs.StatusSucceeded:
			if completed.Result == nil || strings.TrimSpace(completed.Result.Translation) == "" {
				t.Fatalf("succeeded job missing translation: %+v", completed)
			}
			t.Logf(
				"catalog=%s provider=%s translation=%q",
				catalogModel,
				providerModel,
				completed.Result.Translation,
			)
			return
		case jobs.StatusFailed:
			t.Fatalf("job failed: %+v", completed)
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete before deadline: %+v", completed)
		}
		time.Sleep(200 * time.Millisecond)
	}
}


func integrationAPIRequest(
	t *testing.T,
	method, url, key, body string,
) *http.Response {
	t.Helper()
	request, err := http.NewRequest(method, url, bytes.NewBufferString(body))
	if err != nil {
		t.Fatal(err)
	}
	if key != "" {
		request.Header.Set("Authorization", "Bearer "+key)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func integrationDecodeBody(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func integrationReadBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}
