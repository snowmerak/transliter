package restapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
	"github.com/snowmerak/transliter/models/catalog"
)

type processorFunc func(context.Context, jobs.Job) (jobs.Result, error)

func (function processorFunc) Process(ctx context.Context, job jobs.Job) (jobs.Result, error) {
	return function(ctx, job)
}

func TestAuthenticatedAsynchronousJobLifecycleAndHistory(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(map[string]string{
		"alice": "alice-key",
		"bob":   "bob-key",
	})
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(8)
	handler := &Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Catalog:       catalog.Resolver{},
		Retention:     24 * time.Hour,
	}
	routes, err := handler.Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	schedulerDone := make(chan error, 1)
	scheduler := &jobs.Scheduler{
		Queue: backend,
		Store: backend,
		Processor: processorFunc(func(context.Context, jobs.Job) (jobs.Result, error) {
			return jobs.Result{Translation: "안녕하세요"}, nil
		}),
	}
	go func() { schedulerDone <- scheduler.Run(ctx) }()
	defer func() {
		cancel()
		<-schedulerDone
	}()

	createBody := `{
		"model_catalog":"hymt2-1.8b",
		"translation":{
			"source":"Hello",
			"source_language":"English",
			"target_language":"Korean",
			"glossary":{}
		}
	}`
	create := apiRequest(t, http.MethodPost, server.URL+"/v1/jobs", "alice-key", createBody)
	if create.StatusCode != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", create.StatusCode, readBody(t, create))
	}
	var created jobs.Job
	decodeBody(t, create, &created)
	if created.ID == "" || created.Status != jobs.StatusQueued {
		t.Fatalf("unexpected created job: %+v", created)
	}

	var completed jobs.Job
	deadline := time.Now().Add(2 * time.Second)
	for {
		response := apiRequest(
			t,
			http.MethodGet,
			server.URL+"/v1/jobs/"+created.ID,
			"alice-key",
			"",
		)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get status=%d body=%s", response.StatusCode, readBody(t, response))
		}
		decodeBody(t, response, &completed)
		if completed.Status == jobs.StatusSucceeded {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete: %+v", completed)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if completed.Result == nil || completed.Result.Translation != "안녕하세요" {
		t.Fatalf("unexpected completed job: %+v", completed)
	}

	historyResponse := apiRequest(t, http.MethodGet, server.URL+"/v1/jobs", "alice-key", "")
	var history struct {
		Jobs []jobs.Job `json:"jobs"`
	}
	decodeBody(t, historyResponse, &history)
	if len(history.Jobs) != 1 || history.Jobs[0].ID != created.ID {
		t.Fatalf("unexpected history: %+v", history)
	}

	bobGet := apiRequest(
		t,
		http.MethodGet,
		server.URL+"/v1/jobs/"+created.ID,
		"bob-key",
		"",
	)
	if bobGet.StatusCode != http.StatusNotFound {
		t.Fatalf("cross-owner read returned %d", bobGet.StatusCode)
	}
	missingKey := apiRequest(t, http.MethodGet, server.URL+"/v1/jobs", "", "")
	if missingKey.StatusCode != http.StatusUnauthorized {
		t.Fatalf("missing key returned %d", missingKey.StatusCode)
	}
}

func TestOpenJobAccessWithoutAPIKeys(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(8)
	handler := &Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Catalog:       catalog.Resolver{},
		Retention:     24 * time.Hour,
	}
	routes, err := handler.Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	schedulerDone := make(chan error, 1)
	scheduler := &jobs.Scheduler{
		Queue: backend,
		Store: backend,
		Processor: processorFunc(func(context.Context, jobs.Job) (jobs.Result, error) {
			return jobs.Result{Translation: "안녕하세요"}, nil
		}),
	}
	go func() { schedulerDone <- scheduler.Run(ctx) }()
	defer func() {
		cancel()
		<-schedulerDone
	}()

	createBody := `{
		"model_catalog":"hymt2-1.8b",
		"translation":{
			"source":"Hello",
			"source_language":"English",
			"target_language":"Korean",
			"glossary":{}
		}
	}`
	create := apiRequest(t, http.MethodPost, server.URL+"/v1/jobs", "", createBody)
	if create.StatusCode != http.StatusAccepted {
		t.Fatalf("create status=%d body=%s", create.StatusCode, readBody(t, create))
	}
	var created jobs.Job
	decodeBody(t, create, &created)
	if created.ID == "" {
		t.Fatal("expected job id")
	}

	deadline := time.Now().Add(2 * time.Second)
	var completed jobs.Job
	for {
		response := apiRequest(t, http.MethodGet, server.URL+"/v1/jobs/"+created.ID, "", "")
		if response.StatusCode != http.StatusOK {
			t.Fatalf("get status=%d body=%s", response.StatusCode, readBody(t, response))
		}
		decodeBody(t, response, &completed)
		if completed.Status == jobs.StatusSucceeded {
			if completed.Result == nil || completed.Result.Translation != "안녕하세요" {
				t.Fatalf("unexpected result: %+v", completed.Result)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job did not complete: %+v", completed)
		}
		time.Sleep(10 * time.Millisecond)
	}

	list := apiRequest(t, http.MethodGet, server.URL+"/v1/jobs", "", "")
	if list.StatusCode != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.StatusCode, readBody(t, list))
	}
	var history struct {
		Jobs []jobs.Job `json:"jobs"`
	}
	decodeBody(t, list, &history)
	if len(history.Jobs) != 1 || history.Jobs[0].ID != created.ID {
		t.Fatalf("anonymous history: %+v", history)
	}
}

func TestCreateJobRequiresGlossaryObject(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(8)
	handler := &Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Catalog:       catalog.Resolver{},
		Retention:     24 * time.Hour,
	}
	routes, err := handler.Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	for _, body := range []string{
		`{"model_catalog":"hymt2-1.8b","translation":{"source":"Hello","source_language":"English","target_language":"Korean"}}`,
		`{"model_catalog":"hymt2-1.8b","translation":{"source":"Hello","source_language":"English","target_language":"Korean","glossary":null}}`,
	} {
		response := apiRequest(t, http.MethodPost, server.URL+"/v1/jobs", "", body)
		if response.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("missing glossary status=%d body=%s for %s", response.StatusCode, readBody(t, response), body)
		}
		message := readBody(t, response)
		if !strings.Contains(message, "glossary is required") {
			t.Fatalf("expected glossary required error, got %s", message)
		}
	}

	ok := apiRequest(t, http.MethodPost, server.URL+"/v1/jobs", "", `{
		"model_catalog":"hymt2-1.8b",
		"translation":{
			"source":"Hello",
			"source_language":"English",
			"target_language":"Korean",
			"glossary":{}
		}
	}`)
	if ok.StatusCode != http.StatusAccepted {
		t.Fatalf("empty glossary status=%d body=%s", ok.StatusCode, readBody(t, ok))
	}
	var created jobs.Job
	decodeBody(t, ok, &created)
	if created.Request.Translation.Glossary == nil {
		t.Fatal("expected non-nil empty glossary on accepted job")
	}
	if len(created.Request.Translation.Glossary) != 0 {
		t.Fatalf("expected empty glossary, got %+v", created.Request.Translation.Glossary)
	}
}

func apiRequest(
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

func decodeBody(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(fmt.Sprintf("%s", data))
}
