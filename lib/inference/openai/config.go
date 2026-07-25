// Package openai provides an OpenAI-compatible chat-completions client.
//
// It connects to a server managed by the caller, such as Ollama, LM Studio,
// llama-server, vLLM, or another compatible service. It never starts or stops
// an inference process.
package openai

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	EnvBaseURL = "TRANSLITER_API_BASE_URL"
	EnvModel   = "TRANSLITER_API_MODEL"
	EnvTimeout = "TRANSLITER_API_TIMEOUT"
	EnvAPIKey  = "TRANSLITER_API_KEY"

	DefaultBaseURL = "http://127.0.0.1:8080/v1"
	DefaultTimeout = 120 * time.Second
)

// Config contains non-secret client configuration. API keys are deliberately
// excluded and are read only from TRANSLITER_API_KEY when a Client is created.
type Config struct {
	BaseURL    string
	Model      string
	Timeout    time.Duration
	HTTPClient *http.Client

	RequestEncoder  RequestEncoder
	ResponseDecoder ResponseDecoder
}

// RegisterFlags binds non-secret OpenAI-compatible client settings to a flag
// set. Load environment defaults with ConfigFromEnv before calling it.
//
// API keys are intentionally not registered as flags.
func RegisterFlags(flags *flag.FlagSet, config *Config) {
	flags.StringVar(
		&config.BaseURL,
		"api-base-url",
		config.BaseURL,
		"OpenAI-compatible API base URL",
	)
	flags.StringVar(
		&config.Model,
		"api-model",
		config.Model,
		"model name or alias understood by the inference server",
	)
	flags.DurationVar(
		&config.Timeout,
		"api-timeout",
		config.Timeout,
		"timeout for one inference API request",
	)
}

// ConfigFromEnv reads non-secret defaults from the process environment.
//
// A future CLI may override these fields with flags. API keys must remain
// environment-only and are therefore not returned in Config.
func ConfigFromEnv() (Config, error) {
	config := Config{
		BaseURL: os.Getenv(EnvBaseURL),
		Model:   os.Getenv(EnvModel),
	}
	if value := os.Getenv(EnvTimeout); value != "" {
		timeout, err := time.ParseDuration(value)
		if err != nil {
			return Config{}, fmt.Errorf("%s: %w", EnvTimeout, err)
		}
		config.Timeout = timeout
	}
	return normalizeConfig(config)
}

func normalizeConfig(config Config) (Config, error) {
	if config.BaseURL == "" {
		config.BaseURL = DefaultBaseURL
	}
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")
	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return Config{}, fmt.Errorf("parse API base URL: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return Config{}, fmt.Errorf("API base URL must be an absolute HTTP or HTTPS URL")
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return Config{}, fmt.Errorf("API base URL must not contain a query or fragment")
	}
	if config.Timeout == 0 {
		config.Timeout = DefaultTimeout
	}
	if config.Timeout < 0 {
		return Config{}, fmt.Errorf("API timeout must not be negative")
	}
	if config.RequestEncoder == nil {
		config.RequestEncoder = JSONRequestEncoder{}
	}
	if config.ResponseDecoder == nil {
		config.ResponseDecoder = JSONResponseDecoder{}
	}
	return config, nil
}
