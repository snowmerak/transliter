package jobs

import (
	"context"
	"fmt"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/inference"
)

type ModelResolver interface {
	Find(transliter.ModelID) (transliter.Model, bool)
}

type Processor interface {
	Process(context.Context, Job) (Result, error)
}

type TranslationProcessor struct {
	models ModelResolver
	client inference.Client
}

func NewTranslationProcessor(models ModelResolver, client inference.Client) *TranslationProcessor {
	return &TranslationProcessor{models: models, client: client}
}

func (processor *TranslationProcessor) Process(ctx context.Context, job Job) (Result, error) {
	model, ok := processor.models.Find(job.Request.Model)
	if !ok {
		return Result{}, fmt.Errorf("unknown model %q", job.Request.Model)
	}
	request := job.Request.Translation
	if !model.SupportsLanguage(request.TargetLanguage) {
		return Result{}, fmt.Errorf(
			"model %q does not support target language %q",
			job.Request.Model,
			request.TargetLanguage,
		)
	}
	if request.SourceLanguage != "" && !model.SupportsLanguage(request.SourceLanguage) {
		return Result{}, fmt.Errorf(
			"model %q does not support source language %q",
			job.Request.Model,
			request.SourceLanguage,
		)
	}
	input, err := model.BuildInput(request)
	if err != nil {
		return Result{}, err
	}
	profile := job.Request.Profile
	if profile == "" {
		profile = transliter.ProfileOfficial
	}
	options, err := model.Options(profile)
	if err != nil {
		return Result{}, err
	}
	response, err := processor.client.Generate(
		ctx,
		inference.NewRequest(job.Request.ProviderModel, input, options),
	)
	if err != nil {
		return Result{}, err
	}
	usage := response.TokenUsage()
	return Result{
		Translation:   response.OutputText(),
		ProviderModel: response.ProviderModel(),
		FinishReason:  response.FinishReason(),
		PromptTokens:  usage.PromptTokens,
		OutputTokens:  usage.CompletionTokens,
		TotalTokens:   usage.TotalTokens,
	}, nil
}
