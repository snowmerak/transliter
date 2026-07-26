package jobs

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const EnvServerAPIKeys = "TRANSLITER_SERVER_API_KEYS"

var ErrUnauthorized = fmt.Errorf("invalid API key")

// AnonymousOwnerID is used when no server API keys are configured.
const AnonymousOwnerID = "anonymous"

type Principal struct {
	ID string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type StaticAuthenticator struct {
	keys []staticKey
}

type staticKey struct {
	ownerID string
	digest  [sha256.Size]byte
}

// NewStaticAuthenticator stores only SHA-256 key digests in memory.
// An empty map enables open access: Authenticate accepts a missing key and
// returns AnonymousOwnerID.
func NewStaticAuthenticator(keysByOwner map[string]string) (*StaticAuthenticator, error) {
	authenticator := &StaticAuthenticator{
		keys: make([]staticKey, 0, len(keysByOwner)),
	}
	for ownerID, key := range keysByOwner {
		if ownerID == "" || key == "" {
			return nil, fmt.Errorf("API key owner and value must not be empty")
		}
		authenticator.keys = append(authenticator.keys, staticKey{
			ownerID: ownerID,
			digest:  sha256.Sum256([]byte(key)),
		})
	}
	return authenticator, nil
}

// StaticAuthenticatorFromEnv expects a JSON object mapping owner IDs to API
// keys. When unset or empty, the server runs without inbound API-key auth.
// Raw keys remain environment-only and are not returned from this call.
func StaticAuthenticatorFromEnv() (*StaticAuthenticator, error) {
	raw := strings.TrimSpace(os.Getenv(EnvServerAPIKeys))
	if raw == "" {
		return NewStaticAuthenticator(nil)
	}
	var keys map[string]string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, fmt.Errorf("parse %s: %w", EnvServerAPIKeys, err)
	}
	return NewStaticAuthenticator(keys)
}

func (authenticator *StaticAuthenticator) Authenticate(
	_ context.Context,
	apiKey string,
) (Principal, error) {
	if authenticator == nil || len(authenticator.keys) == 0 {
		return Principal{ID: AnonymousOwnerID}, nil
	}
	if apiKey == "" {
		return Principal{}, ErrUnauthorized
	}
	digest := sha256.Sum256([]byte(apiKey))
	matchedOwner := ""
	matched := 0
	for _, candidate := range authenticator.keys {
		equal := subtle.ConstantTimeCompare(digest[:], candidate.digest[:])
		if equal == 1 {
			matchedOwner = candidate.ownerID
		}
		matched |= equal
	}
	if matched != 1 {
		return Principal{}, ErrUnauthorized
	}
	return Principal{ID: matchedOwner}, nil
}

// Open reports whether inbound API-key authentication is disabled.
func (authenticator *StaticAuthenticator) Open() bool {
	return authenticator == nil || len(authenticator.keys) == 0
}
