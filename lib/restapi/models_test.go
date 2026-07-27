package restapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/snowmerak/transliter/lib/jobs"
	"github.com/snowmerak/transliter/lib/jobs/memory"
)

type modelsFunc func(context.Context) (int, string, []byte, error)

func (function modelsFunc) ListModels(ctx context.Context) (int, string, []byte, error) {
	return function(ctx)
}

func TestListModelsProxiesUpstreamResponse(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(1)
	const body = `{"object":"list","data":[{"id":"hy-mt2"}]}`
	routes, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Models: modelsFunc(func(context.Context) (int, string, []byte, error) {
			return http.StatusOK, "application/json", []byte(body), nil
		}),
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type=%q", response.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("body=%s", data)
	}
}

func TestListModelsDefaultsContentTypeAndForwardsUpstreamErrors(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(1)
	const body = `{"error":"nope"}`
	routes, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Models: modelsFunc(func(context.Context) (int, string, []byte, error) {
			return http.StatusBadGateway, "", []byte(body), nil
		}),
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(routes)
	defer server.Close()

	response, err := http.Get(server.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("status=%d", response.StatusCode)
	}
	if response.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type=%q", response.Header.Get("Content-Type"))
	}
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != body {
		t.Fatalf("body=%s", data)
	}
}

func TestListModelsUnavailableAndUnreachable(t *testing.T) {
	authenticator, err := jobs.NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	backend := memory.New(1)

	unavailable, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	unavailableServer := httptest.NewServer(unavailable)
	defer unavailableServer.Close()
	response, err := http.Get(unavailableServer.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("unavailable status=%d", response.StatusCode)
	}

	unreachable, err := (&Handler{
		Authenticator: authenticator,
		Queue:         backend,
		Store:         backend,
		Models: modelsFunc(func(context.Context) (int, string, []byte, error) {
			return 0, "", nil, errors.New("dial failed")
		}),
	}).Routes()
	if err != nil {
		t.Fatal(err)
	}
	unreachableServer := httptest.NewServer(unreachable)
	defer unreachableServer.Close()
	response, err = http.Get(unreachableServer.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadGateway {
		t.Fatalf("unreachable status=%d", response.StatusCode)
	}
}
