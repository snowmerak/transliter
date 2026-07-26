package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/snowmerak/transliter/lib/inference"
)

const defaultMaxResponseBytes = 4 << 20

// APIError is a sanitized error returned by an OpenAI-compatible server.
type APIError struct {
	StatusCode int
	Code       any
	Type       string
	Message    string
}

func (value *APIError) Error() string {
	details := ""
	if value.Type != "" {
		details = " (" + value.Type + ")"
	}
	return fmt.Sprintf("inference API returned HTTP %d%s: %s", value.StatusCode, details, value.Message)
}

// Client calls a user-managed OpenAI-compatible generation endpoint.
type Client struct {
	baseURL         string
	defaultModel    string
	apiKey          string
	httpClient      *http.Client
	requestEncoder  RequestEncoder
	responseDecoder ResponseDecoder
	maxResponseSize int64
}

var _ inference.Client = (*Client)(nil)

// New creates a client. The API key can only come from TRANSLITER_API_KEY;
// Config deliberately has no API-key field.
func New(config Config) (*Client, error) {
	config, err := normalizeConfig(config)
	if err != nil {
		return nil, err
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: config.Timeout}
	}
	return &Client{
		baseURL:         config.BaseURL,
		defaultModel:    config.Model,
		apiKey:          os.Getenv(EnvAPIKey),
		httpClient:      httpClient,
		requestEncoder:  config.RequestEncoder,
		responseDecoder: config.ResponseDecoder,
		maxResponseSize: defaultMaxResponseBytes,
	}, nil
}

// Generate performs one non-streaming generation request.
// Plain chat uses /chat/completions. Structured TranslateGemma content is
// rendered client-side and posted to /completions when the encoder routes there.
func (client *Client) Generate(
	ctx context.Context,
	request inference.Request,
) (inference.Response, error) {
	if request == nil {
		return nil, fmt.Errorf("inference request must not be nil")
	}
	body, err := client.requestEncoder.EncodeRequest(request, client.defaultModel)
	if err != nil {
		return nil, err
	}
	path := chatCompletionsPath
	if router, ok := client.requestEncoder.(RequestRouter); ok {
		if routed := router.RequestPath(request, client.defaultModel); routed != "" {
			path = routed
		}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("create inference HTTP request: %w", err)
	}
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Content-Type", "application/json")
	if client.apiKey != "" {
		httpRequest.Header.Set("Authorization", "Bearer "+client.apiKey)
	}

	httpResponse, err := client.httpClient.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("call inference API: %w", err)
	}
	defer httpResponse.Body.Close()

	limited := io.LimitReader(httpResponse.Body, client.maxResponseSize+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("read inference API response: %w", err)
	}
	if int64(len(data)) > client.maxResponseSize {
		return nil, fmt.Errorf("inference API response exceeds %s bytes", strconv.FormatInt(client.maxResponseSize, 10))
	}
	if httpResponse.StatusCode < http.StatusOK || httpResponse.StatusCode >= http.StatusMultipleChoices {
		return nil, client.responseDecoder.DecodeError(httpResponse.StatusCode, strings.NewReader(string(data)))
	}
	response, err := client.responseDecoder.DecodeResponse(strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	return response, nil
}

// IsAPIError reports whether err contains a server-side API error.
func IsAPIError(err error) bool {
	var apiError *APIError
	return errors.As(err, &apiError)
}
