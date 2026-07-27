package openai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/inference"
)

// RequestEncoder maps a transport-neutral request to an OpenAI-compatible
// request body. Implementations can adapt optional parameters for a particular
// compatible server without changing the HTTP client.
type RequestEncoder interface {
	EncodeRequest(inference.Request, string) (io.Reader, error)
}

// RequestRouter selects the OpenAI-compatible path under a /v1 base URL.
// Encoders that need /completions instead of /chat/completions implement it.
type RequestRouter interface {
	RequestPath(inference.Request, string) string
}

// ResponseDecoder maps OpenAI-compatible success and error bodies separately.
// This keeps provider response differences out of request construction.
type ResponseDecoder interface {
	DecodeResponse(io.Reader) (inference.Response, error)
	DecodeError(int, io.Reader) error
}

const (
	chatCompletionsPath = "/chat/completions"
	completionsPath     = "/completions"
	modelsPath          = "/models"
)

// JSONRequestEncoder emits OpenAI-compatible request bodies.
//
// Plain chat messages use /chat/completions. TranslateGemma-style structured
// content (source_lang_code/target_lang_code parts) is rendered client-side and
// sent to /completions, because several OpenAI-compatible servers (including
// oMLX) strip or reject that custom chat content shape.
//
// top_k, repetition_penalty, and do_sample are common local-server extensions,
// not core OpenAI parameters.
type JSONRequestEncoder struct{}

func (JSONRequestEncoder) RequestPath(request inference.Request, _ string) string {
	if request != nil && requiresCompletionBypass(request.ModelInput()) {
		return completionsPath
	}
	return chatCompletionsPath
}

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

	if requiresCompletionBypass(input) {
		prompt, err := renderStructuredCompletionPrompt(input)
		if err != nil {
			return nil, err
		}
		payload := completionRequest{
			Model:             modelName,
			Prompt:            prompt,
			Stream:            false,
			Temperature:       options.Temperature,
			TopP:              options.TopP,
			TopK:              options.TopK,
			RepetitionPenalty: options.RepetitionPenalty,
			DoSample:          options.DoSample,
			MaxTokens:         options.MaxOutputTokens,
			Stop:              completionStopSequences(options.Stop),
		}
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("encode completions request: %w", err)
		}
		return bytes.NewReader(encoded), nil
	}

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

func requiresCompletionBypass(input transliter.ModelInput) bool {
	for _, message := range input.Messages {
		for _, part := range message.Parts {
			if part.SourceLanguageCode != "" || part.TargetLanguageCode != "" {
				return true
			}
		}
	}
	return false
}

// renderStructuredCompletionPrompt applies a Gemma-style chat template locally.
// oMLX maintainers recommend this when /chat/completions cannot carry custom
// content mappings through to the model template.
func renderStructuredCompletionPrompt(input transliter.ModelInput) (string, error) {
	if len(input.Messages) != 1 {
		return "", fmt.Errorf("structured completion prompt requires exactly one message")
	}
	message := input.Messages[0]
	if message.Role != transliter.RoleUser {
		return "", fmt.Errorf("structured completion prompt requires a user message")
	}
	if message.Text != "" {
		return "", fmt.Errorf("structured completion prompt requires content parts, not plain text")
	}
	if len(message.Parts) != 1 {
		return "", fmt.Errorf("structured completion prompt requires exactly one content part")
	}
	part := message.Parts[0]
	if part.Type != "" && part.Type != "text" {
		return "", fmt.Errorf("structured completion prompt supports only text parts, got %q", part.Type)
	}
	if strings.TrimSpace(part.SourceLanguageCode) == "" || strings.TrimSpace(part.TargetLanguageCode) == "" {
		return "", fmt.Errorf("structured completion prompt requires source_lang_code and target_lang_code")
	}
	if part.Text == "" {
		return "", fmt.Errorf("structured completion prompt requires non-empty text")
	}

	var builder strings.Builder
	builder.WriteString("<bos><start_of_turn>user\n")
	builder.WriteString("source_lang_code: ")
	builder.WriteString(part.SourceLanguageCode)
	builder.WriteByte('\n')
	builder.WriteString("target_lang_code: ")
	builder.WriteString(part.TargetLanguageCode)
	builder.WriteByte('\n')
	builder.WriteString("text: ")
	builder.WriteString(part.Text)
	builder.WriteString("<end_of_turn>\n<start_of_turn>model\n")
	return builder.String(), nil
}

func completionStopSequences(stop []string) []string {
	const endOfTurn = "<end_of_turn>"
	for _, value := range stop {
		if value == endOfTurn {
			return stop
		}
	}
	out := make([]string, 0, len(stop)+1)
	out = append(out, stop...)
	out = append(out, endOfTurn)
	return out
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

type completionRequest struct {
	Model             string   `json:"model"`
	Prompt            string   `json:"prompt"`
	Stream            bool     `json:"stream"`
	Temperature       *float64 `json:"temperature,omitempty"`
	TopP              *float64 `json:"top_p,omitempty"`
	TopK              *int     `json:"top_k,omitempty"`
	RepetitionPenalty *float64 `json:"repetition_penalty,omitempty"`
	DoSample          *bool    `json:"do_sample,omitempty"`
	MaxTokens         int      `json:"max_tokens,omitempty"`
	Stop              []string `json:"stop,omitempty"`
}

// JSONResponseDecoder decodes chat-completions and text-completions responses.
type JSONResponseDecoder struct{}

func (JSONResponseDecoder) DecodeResponse(reader io.Reader) (inference.Response, error) {
	var payload openAIGenerationResponse
	if err := decodeSingleJSON(reader, &payload); err != nil {
		return nil, fmt.Errorf("decode generation response: %w", err)
	}
	if len(payload.Choices) == 0 {
		return nil, fmt.Errorf("generation response contains no choices")
	}
	choice := payload.Choices[0]
	output := choice.Message.Content
	if output == "" {
		output = choice.Text
	}
	return inference.NewResponse(
		output,
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

type openAIGenerationResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Text    string `json:"text"`
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
