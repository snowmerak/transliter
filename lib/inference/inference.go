// Package inference defines transport-neutral generation contracts.
package inference

import (
	"context"

	transliter "github.com/snowmerak/transliter/lib"
)

// Request supplies the model input and generation settings to a Client.
//
// ModelName is the name or alias understood by the remote inference server. It
// is intentionally separate from transliter.Descriptor.ID.
type Request interface {
	ModelName() string
	ModelInput() transliter.ModelInput
	GenerationOptions() transliter.GenerationOptions
}

// Response is the transport-neutral result returned by a Client.
type Response interface {
	OutputText() string
	ProviderModel() string
	FinishReason() string
	TokenUsage() Usage
}

// Client sends a generation request to an inference service.
type Client interface {
	Generate(context.Context, Request) (Response, error)
}

// Usage contains token counts reported by the inference service.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

type request struct {
	modelName string
	input     transliter.ModelInput
	options   transliter.GenerationOptions
}

// NewRequest creates an immutable transport-neutral request.
func NewRequest(
	modelName string,
	input transliter.ModelInput,
	options transliter.GenerationOptions,
) Request {
	return request{
		modelName: modelName,
		input:     cloneInput(input),
		options:   cloneOptions(options),
	}
}

func (value request) ModelName() string {
	return value.modelName
}

func (value request) ModelInput() transliter.ModelInput {
	return cloneInput(value.input)
}

func (value request) GenerationOptions() transliter.GenerationOptions {
	return cloneOptions(value.options)
}

type response struct {
	outputText    string
	providerModel string
	finishReason  string
	usage         Usage
}

// NewResponse creates an immutable transport-neutral response.
func NewResponse(outputText, providerModel, finishReason string, usage Usage) Response {
	return response{
		outputText:    outputText,
		providerModel: providerModel,
		finishReason:  finishReason,
		usage:         usage,
	}
}

func (value response) OutputText() string {
	return value.outputText
}

func (value response) ProviderModel() string {
	return value.providerModel
}

func (value response) FinishReason() string {
	return value.finishReason
}

func (value response) TokenUsage() Usage {
	return value.usage
}

func cloneInput(input transliter.ModelInput) transliter.ModelInput {
	messages := make([]transliter.Message, len(input.Messages))
	for index, message := range input.Messages {
		messages[index] = message
		messages[index].Parts = append([]transliter.ContentPart(nil), message.Parts...)
	}
	return transliter.ModelInput{Messages: messages}
}

func cloneOptions(options transliter.GenerationOptions) transliter.GenerationOptions {
	cloned := options
	cloned.Temperature = clonePointer(options.Temperature)
	cloned.TopP = clonePointer(options.TopP)
	cloned.TopK = clonePointer(options.TopK)
	cloned.RepetitionPenalty = clonePointer(options.RepetitionPenalty)
	cloned.DoSample = clonePointer(options.DoSample)
	cloned.Stop = append([]string(nil), options.Stop...)
	return cloned
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
