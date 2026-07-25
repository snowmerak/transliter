// Package v7b provides the Hy-MT2 7B model contract.
package v7b

import (
	transliter "github.com/snowmerak/translter/lib"
	"github.com/snowmerak/translter/models/internal/hymt2"
)

// Model implements transliter.Model for Tencent Hy-MT2 7B.
type Model struct{ core hymt2.Core }

// New constructs the Hy-MT2 7B model contract.
func New() Model {
	return Model{core: hymt2.New(
		transliter.Descriptor{
			ID:             "hymt2-7b",
			Family:         "Hy-MT2",
			Parameters:     "7B",
			Repository:     "tencent/Hy-MT2-7B",
			GGUFRepository: "tencent/Hy-MT2-7B-GGUF",
		},
		transliter.GenerationOptions{
			Temperature:       transliter.Pointer(0.7),
			TopP:              transliter.Pointer(0.6),
			TopK:              transliter.Pointer(20),
			RepetitionPenalty: transliter.Pointer(1.05),
			MaxOutputTokens:   4096,
			Provenance:        transliter.ProvenanceOfficialRecommendation,
		},
	)}
}

func (model Model) Descriptor() transliter.Descriptor     { return model.core.Descriptor() }
func (model Model) Capabilities() transliter.Capabilities { return model.core.Capabilities() }
func (model Model) SupportsLanguage(language transliter.Language) bool {
	return model.core.SupportsLanguage(language)
}
func (model Model) BuildInput(request transliter.TranslationRequest) (transliter.ModelInput, error) {
	return model.core.BuildInput(request)
}
func (model Model) Options(profile transliter.OptionProfile) (transliter.GenerationOptions, error) {
	return model.core.Options(profile)
}

var _ transliter.Model = Model{}
