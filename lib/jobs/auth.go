package jobs

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"os"
)

const EnvServerAPIKeys = "TRANSLITER_SERVER_API_KEYS"

var ErrUnauthorized = fmt.Errorf("invalid API key")

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
	if len(authenticator.keys) == 0 {
		return nil, fmt.Errorf("at least one server API key is required")
	}
	return authenticator, nil
}

// StaticAuthenticatorFromEnv expects a JSON object mapping owner IDs to API
// keys. Raw keys remain environment-only and are not returned from this call.
func StaticAuthenticatorFromEnv() (*StaticAuthenticator, error) {
	raw := os.Getenv(EnvServerAPIKeys)
	if raw == "" {
		return nil, fmt.Errorf("%s must be set", EnvServerAPIKeys)
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
