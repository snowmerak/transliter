package openai

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/inference"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv(EnvBaseURL, "http://localhost:1234/v1/")
	t.Setenv(EnvModel, "local-model")
	t.Setenv(EnvTimeout, "45s")
	t.Setenv(EnvAPIKey, "secret")

	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if config.BaseURL != "http://localhost:1234/v1" {
		t.Fatalf("unexpected base URL: %q", config.BaseURL)
	}
	if config.Model != "local-model" || config.Timeout != 45*time.Second {
		t.Fatalf("unexpected config: %+v", config)
	}
}

func TestRegisterFlagsOverridesEnvironmentDefaultsWithoutAPIKeyFlag(t *testing.T) {
	t.Setenv(EnvBaseURL, "http://localhost:8080/v1")
	t.Setenv(EnvModel, "environment-model")
	t.Setenv(EnvTimeout, "2m")
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}

	flags := flag.NewFlagSet("test", flag.ContinueOnError)
	RegisterFlags(flags, &config)
	if err := flags.Parse([]string{
		"--api-base-url", "http://localhost:1234/v1",
		"--api-model", "flag-model",
		"--api-timeout", "30s",
	}); err != nil {
		t.Fatal(err)
	}

	if config.BaseURL != "http://localhost:1234/v1" ||
		config.Model != "flag-model" ||
		config.Timeout != 30*time.Second {
		t.Fatalf("flags did not override environment defaults: %+v", config)
	}
	if flags.Lookup("api-key") != nil {
		t.Fatal("API key must not be registered as a flag")
	}
}

func TestClientSendsChatCompletionAndDecodesResponse(t *testing.T) {
	const apiKey = "not-for-logs"
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Errorf("unexpected authorization header")
		}
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{
			"model":"served-model",
			"choices":[{"message":{"content":"서비스가 준비되었습니다."},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":20,"completion_tokens":7,"total_tokens":27}
		}`))
	}))
	defer server.Close()
	t.Setenv(EnvAPIKey, apiKey)

	client, err := New(Config{BaseURL: server.URL + "/v1", Model: "default-model"})
	if err != nil {
		t.Fatal(err)
	}
	generationRequest := inference.NewRequest(
		"",
		transliter.ModelInput{Messages: []transliter.Message{{
			Role: transliter.RoleUser,
			Text: "Translate this.",
		}}},
		transliter.GenerationOptions{
			Temperature:       transliter.Pointer(0.7),
			TopP:              transliter.Pointer(0.6),
			TopK:              transliter.Pointer(20),
			RepetitionPenalty: transliter.Pointer(1.05),
			MaxOutputTokens:   4096,
			Stop:              []string{"</s>"},
		},
	)
	response, err := client.Generate(context.Background(), generationRequest)
	if err != nil {
		t.Fatal(err)
	}

	if received["model"] != "default-model" || received["stream"] != false {
		t.Fatalf("unexpected request body: %+v", received)
	}
	if received["top_k"] != float64(20) || received["repetition_penalty"] != 1.05 {
		t.Fatalf("model options missing from request: %+v", received)
	}
	if response.OutputText() != "서비스가 준비되었습니다." {
		t.Fatalf("unexpected output: %q", response.OutputText())
	}
	if response.ProviderModel() != "served-model" || response.TokenUsage().TotalTokens != 27 {
		t.Fatal("response metadata was not decoded")
	}
}

func TestClientPreservesStructuredMessageContent(t *testing.T) {
	var received map[string]any
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		if err := json.NewDecoder(request.Body).Decode(&received); err != nil {
			t.Errorf("decode request: %v", err)
			writer.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = writer.Write([]byte(`{"model":"translategemma","choices":[{"text":"안녕","finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`))
	}))
	defer server.Close()
	t.Setenv(EnvAPIKey, "")

	client, err := New(Config{BaseURL: server.URL + "/v1"})
	if err != nil {
		t.Fatal(err)
	}
	request := inference.NewRequest(
		"translategemma",
		transliter.ModelInput{Messages: []transliter.Message{{
			Role: transliter.RoleUser,
			Parts: []transliter.ContentPart{{
				Type:               "text",
				Text:               "hello",
				SourceLanguageCode: "en",
				TargetLanguageCode: "ko",
			}},
		}}},
		transliter.GenerationOptions{},
	)
	response, err := client.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/v1/completions" {
		t.Fatalf("structured content path = %s, want /v1/completions", path)
	}
	prompt, _ := received["prompt"].(string)
	if !strings.Contains(prompt, "source_lang_code: en") ||
		!strings.Contains(prompt, "target_lang_code: ko") ||
		!strings.Contains(prompt, "text: hello") ||
		!strings.Contains(prompt, "<start_of_turn>model\n") {
		t.Fatalf("structured completion prompt missing fields: %q", prompt)
	}
	stops, _ := received["stop"].([]any)
	if len(stops) == 0 || stops[0] != "<end_of_turn>" {
		t.Fatalf("expected <end_of_turn> stop, got %#v", received["stop"])
	}
	if response.OutputText() != "안녕" {
		t.Fatalf("unexpected output: %q", response.OutputText())
	}
}

func TestClientReturnsSanitizedAPIError(t *testing.T) {
	const apiKey = "must-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(`{
			"error":{"message":"invalid credential","type":"authentication_error","code":"bad_key"}
		}`))
	}))
	defer server.Close()
	t.Setenv(EnvAPIKey, apiKey)

	client, err := New(Config{BaseURL: server.URL + "/v1", Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Generate(context.Background(), inference.NewRequest(
		"",
		transliter.ModelInput{Messages: []transliter.Message{{Role: transliter.RoleUser, Text: "hello"}}},
		transliter.GenerationOptions{},
	))
	if err == nil || !IsAPIError(err) {
		t.Fatalf("expected API error, got %v", err)
	}
	if strings.Contains(err.Error(), apiKey) {
		t.Fatal("API key leaked through error text")
	}
	var apiError *APIError
	if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected API error: %v", err)
	}
}

func TestClientListModelsProxiesUpstreamBody(t *testing.T) {
	const apiKey = "models-key"
	const body = `{"object":"list","data":[{"id":"local-model","object":"model"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", request.Method)
		}
		if request.URL.Path != "/v1/models" {
			t.Errorf("unexpected path: %s", request.URL.Path)
		}
		if request.Header.Get("Authorization") != "Bearer "+apiKey {
			t.Errorf("unexpected authorization header")
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()
	t.Setenv(EnvAPIKey, apiKey)

	client, err := New(Config{BaseURL: server.URL + "/v1", Model: "default-model"})
	if err != nil {
		t.Fatal(err)
	}
	status, contentType, data, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusOK {
		t.Fatalf("status=%d", status)
	}
	if !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type=%q", contentType)
	}
	if string(data) != body {
		t.Fatalf("body=%s", data)
	}
}

func TestClientListModelsForwardsNon2xx(t *testing.T) {
	const body = `{"error":{"message":"no models"}}`
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusUnauthorized)
		_, _ = writer.Write([]byte(body))
	}))
	defer server.Close()

	client, err := New(Config{BaseURL: server.URL + "/v1", Model: "default-model"})
	if err != nil {
		t.Fatal(err)
	}
	status, _, data, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("status=%d", status)
	}
	if string(data) != body {
		t.Fatalf("body=%s", data)
	}
}

func TestJSONRequestEncoderRequiresModelAndMessages(t *testing.T) {
	encoder := JSONRequestEncoder{}
	_, err := encoder.EncodeRequest(
		inference.NewRequest("", transliter.ModelInput{}, transliter.GenerationOptions{}),
		"",
	)
	if err == nil {
		t.Fatal("expected missing model error")
	}

	_, err = encoder.EncodeRequest(
		inference.NewRequest("model", transliter.ModelInput{}, transliter.GenerationOptions{}),
		"",
	)
	if err == nil {
		t.Fatal("expected missing messages error")
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	t.Setenv(EnvBaseURL, "file:///tmp/model")
	t.Setenv(EnvTimeout, "")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected invalid URL error")
	}

	t.Setenv(EnvBaseURL, "http://localhost:8080/v1")
	t.Setenv(EnvTimeout, "not-a-duration")
	if _, err := ConfigFromEnv(); err == nil {
		t.Fatal("expected invalid timeout error")
	}
}
