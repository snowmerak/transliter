package jobs

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStaticAuthenticator(t *testing.T) {
	authenticator, err := NewStaticAuthenticator(map[string]string{
		"alice": "alice-secret",
		"bob":   "bob-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	principal, err := authenticator.Authenticate(context.Background(), "bob-secret")
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID != "bob" {
		t.Fatalf("unexpected principal: %+v", principal)
	}
	if _, err := authenticator.Authenticate(context.Background(), "wrong"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected unauthorized error, got %v", err)
	}
}

func TestStaticAuthenticatorFromEnvDoesNotExposeKeyInError(t *testing.T) {
	const secret = "must-not-leak"
	t.Setenv(EnvServerAPIKeys, `{"alice":"`+secret)
	_, err := StaticAuthenticatorFromEnv()
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("API key leaked through parse error")
	}
}

func TestStaticAuthenticatorOpenMode(t *testing.T) {
	authenticator, err := NewStaticAuthenticator(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !authenticator.Open() {
		t.Fatal("empty authenticator should be open")
	}
	principal, err := authenticator.Authenticate(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if principal.ID != AnonymousOwnerID {
		t.Fatalf("principal = %+v", principal)
	}

	t.Setenv(EnvServerAPIKeys, "")
	fromEnv, err := StaticAuthenticatorFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !fromEnv.Open() {
		t.Fatal("unset env should yield open authenticator")
	}
}
