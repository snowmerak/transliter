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

	"github.com/snowmerak/translter/lib/jobs"
	"github.com/snowmerak/translter/lib/jobs/memory"
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
		"model":"hymt2-1.8b",
		"translation":{
			"source":"Hello",
			"source_language":"English",
			"target_language":"Korean"
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
