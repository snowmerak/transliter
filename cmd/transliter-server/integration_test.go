package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	integrationBaseURL = "http://macmini:1234/v1"
	integrationAPIKey  = "integration-test-key"
)

// catalogID -> preferred provider model ids on macmini:1234 (first match wins).
var integrationModelCases = []struct {
	name               string
	catalogModel       string
	providerCandidates []string
}{
	{
		name:               "hymt2-1.8b",
		catalogModel:       "hymt2-1.8b",
		providerCandidates: []string{"hy-mt2-1.8b"},
	},
	{
		name:               "hymt2-7b",
		catalogModel:       "hymt2-7b",
		providerCandidates: []string{"hy-mt2-7b"},
	},
	{
		name:               "hymt2-30b-a3b",
		catalogModel:       "hymt2-30b-a3b",
		providerCandidates: []string{"hy-mt2-30b-a3b-mlx", "hy-mt2-30b-a3b"},
	},
	{
		name:               "translategemma-4b",
		catalogModel:       "translategemma-4b",
		providerCandidates: []string{"translategemma-4b-it", "translategemma-4b"},
	},
	{
		name:               "translategemma-12b",
		catalogModel:       "translategemma-12b",
		providerCandidates: []string{"translategemma-12b-it", "translategemma-12b"},
	},
	{
		name:               "translategemma-27b",
		catalogModel:       "translategemma-27b",
		providerCandidates: []string{"translategemma-27b-it", "translategemma-27b"},
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

func TestIntegrationTranslationJobDefaultBackends(t *testing.T) {
	available := requireIntegrationModels(t)

	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"alice": integrationAPIKey,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backends, err := buildBackends(ctx, serverConfig{
		QueueBackend:       "nats-embedded",
		StoreBackend:       "sqlite",
		SQLitePath:         filepath.Join(t.TempDir(), "jobs.db"),
		NATSEmbeddedMemory: true,
		JobTimeout:         5 * time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer backends.Close()

	// Default client model is only a fallback; each job sets provider_model.
	client, err := openai.New(openai.Config{
		BaseURL: integrationBaseURL,
		Model:   "hy-mt2-1.8b",
		Timeout: 5 * time.Minute,
	})
	if err != nil {
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
	defer func() {
		cancel()
		<-schedulerDone
	}()

	handler := &restapi.Handler{
		Authenticator: authenticator,
		Queue:         backends.queue,
		Store:         backends.store,
		Retention:     time.Hour,
		Validate:      validateJobRequest,
	}
	routes, err := handler.Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	health := integrationAPIRequest(t, http.MethodGet, server.URL+"/healthz", "", "")
	if health.StatusCode != http.StatusOK {
		t.Fatalf("healthz status=%d body=%s", health.StatusCode, integrationReadBody(t, health))
	}
	var healthBody struct {
		Status string `json:"status"`
	}
	integrationDecodeBody(t, health, &healthBody)
	if healthBody.Status != "ok" {
		t.Fatalf("unexpected healthz body: %+v", healthBody)
	}

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
			runIntegrationTranslationJob(t, server.URL, tc.catalogModel, providerModel)
		})
	}
	if ran == 0 {
		t.Fatal("no catalog provider models available on inference server")
	}
}

func runIntegrationTranslationJob(t *testing.T, baseURL, catalogModel, providerModel string) {
	t.Helper()

	createBody := fmt.Sprintf(`{
		"model": %q,
		"provider_model": %q,
		"profile": "official",
		"translation": {
			"source": "The service is ready.",
			"source_language": "English",
			"target_language": "Korean",
			"kind": "text"
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
