// Package v30ba3b provides the Hy-MT2 30B-A3B model contract.
package v30ba3b

import (
	transliter "github.com/snowmerak/translter/lib"
	"github.com/snowmerak/translter/models/internal/hymt2"
)

// Model implements transliter.Model for Tencent Hy-MT2 30B-A3B.
type Model struct{ core hymt2.Core }

// New constructs the Hy-MT2 30B-A3B model contract.
func New() Model {
	return Model{core: hymt2.New(
		transliter.Descriptor{
			ID:             "hymt2-30b-a3b",
			Family:         "Hy-MT2",
			Parameters:     "30B-A3B",
			Repository:     "tencent/Hy-MT2-30B-A3B",
			GGUFRepository: "tencent/Hy-MT2-30B-A3B-GGUF",
		},
		transliter.GenerationOptions{
			Temperature:       transliter.Pointer(0.7),
			TopP:              transliter.Pointer(1.0),
			TopK:              transliter.Pointer(-1),
			RepetitionPenalty: transliter.Pointer(1.0),
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
