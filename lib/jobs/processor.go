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
	catalogModel, ok := processor.models.Find(job.Request.ModelCatalog)
	if !ok {
		return Result{}, fmt.Errorf("unknown model catalog %q", job.Request.ModelCatalog)
	}
	request := job.Request.Translation
	if !catalogModel.SupportsLanguage(request.TargetLanguage) {
		return Result{}, fmt.Errorf(
			"model catalog %q does not support target language %q",
			job.Request.ModelCatalog,
			request.TargetLanguage,
		)
	}
	if request.SourceLanguage != "" && !catalogModel.SupportsLanguage(request.SourceLanguage) {
		return Result{}, fmt.Errorf(
			"model catalog %q does not support source language %q",
			job.Request.ModelCatalog,
			request.SourceLanguage,
		)
	}
	input, err := catalogModel.BuildInput(request)
	if err != nil {
		return Result{}, err
	}
	profile := job.Request.Profile
	if profile == "" {
		profile = transliter.ProfileOfficial
	}
	options, err := catalogModel.Options(profile)
	if err != nil {
		return Result{}, err
	}
	response, err := processor.client.Generate(
		ctx,
		inference.NewRequest(job.Request.Model, input, options),
	)
	if err != nil {
		return Result{}, err
	}
	usage := response.TokenUsage()
	return Result{
		Translation:  response.OutputText(),
		Model:        response.ProviderModel(),
		FinishReason: response.FinishReason(),
		PromptTokens: usage.PromptTokens,
		OutputTokens: usage.CompletionTokens,
		TotalTokens:  usage.TotalTokens,
	}, nil
}
