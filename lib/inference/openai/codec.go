package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	transliter "github.com/snowmerak/translter/lib"
	"github.com/snowmerak/translter/lib/inference"
)

// RequestEncoder maps a transport-neutral request to an OpenAI-compatible
// request body. Implementations can adapt optional parameters for a particular
// compatible server without changing the HTTP client.
type RequestEncoder interface {
	EncodeRequest(inference.Request, string) (io.Reader, error)
}

// ResponseDecoder maps OpenAI-compatible success and error bodies separately.
// This keeps provider response differences out of request construction.
type ResponseDecoder interface {
	DecodeResponse(io.Reader) (inference.Response, error)
	DecodeError(int, io.Reader) error
}

// JSONRequestEncoder emits a non-streaming chat-completions request.
//
// top_k, repetition_penalty, and do_sample are common local-server extensions,
// not core OpenAI parameters. Replace this encoder when a server uses different
// names or rejects extensions.
type JSONRequestEncoder struct{}

func (JSONRequestEncoder) EncodeRequest(
	request inference.Request,
	defaultModel string,
) (io.Reader, error) {
	modelName := request.ModelName()
	if modelName == "" {
		modelName = defaultModel
	}
	if modelName == "" {
		return nil, fmt.Errorf("inference model name must not be empty")
	}

	input := request.ModelInput()
	if len(input.Messages) == 0 {
		return nil, fmt.Errorf("inference request must contain at least one message")
	}
	options := request.GenerationOptions()
	payload := chatCompletionRequest{
		Model:             modelName,
		Messages:          input.Messages,
		Stream:            false,
		Temperature:       options.Temperature,
		TopP:              options.TopP,
		TopK:              options.TopK,
		RepetitionPenalty: options.RepetitionPenalty,
		DoSample:          options.DoSample,
		MaxTokens:         options.MaxOutputTokens,
		Stop:              options.Stop,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode chat-completions request: %w", err)
	}
	return bytes.NewReader(encoded), nil
}

type chatCompletionRequest struct {
	Model             string               `json:"model"`
	Messages          []transliter.Message `json:"messages"`
	Stream            bool                 `json:"stream"`
	Temperature       *float64             `json:"temperature,omitempty"`
	TopP              *float64             `json:"top_p,omitempty"`
	TopK              *int                 `json:"top_k,omitempty"`
	RepetitionPenalty *float64             `json:"repetition_penalty,omitempty"`
	DoSample          *bool                `json:"do_sample,omitempty"`
	MaxTokens         int                  `json:"max_tokens,omitempty"`
	Stop              []string             `json:"stop,omitempty"`
}

// JSONResponseDecoder decodes the common OpenAI chat-completions response.
type JSONResponseDecoder struct{}

func (JSONResponseDecoder) DecodeResponse(reader io.Reader) (inference.Response, error) {
	var payload chatCompletionResponse
	if err := decodeSingleJSON(reader, &payload); err != nil {
		return nil, fmt.Errorf("decode chat-completions response: %w", err)
	}
	if len(payload.Choices) == 0 {
		return nil, fmt.Errorf("chat-completions response contains no choices")
	}
	choice := payload.Choices[0]
	return inference.NewResponse(
		choice.Message.Content,
		payload.Model,
		choice.FinishReason,
		inference.Usage{
			PromptTokens:     payload.Usage.PromptTokens,
			CompletionTokens: payload.Usage.CompletionTokens,
			TotalTokens:      payload.Usage.TotalTokens,
		},
	), nil
}

func (JSONResponseDecoder) DecodeError(statusCode int, reader io.Reader) error {
	var payload errorResponse
	if err := decodeSingleJSON(reader, &payload); err != nil {
		return &APIError{
			StatusCode: statusCode,
			Message:    "inference server returned a non-JSON error",
		}
	}
	message := payload.Error.Message
	if message == "" {
		message = "inference server returned an error"
	}
	return &APIError{
		StatusCode: statusCode,
		Code:       payload.Error.Code,
		Type:       payload.Error.Type,
		Message:    message,
	}
}

func decodeSingleJSON(reader io.Reader, target any) error {
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("response contains trailing JSON")
		}
		return err
	}
	return nil
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}
