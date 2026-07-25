// Package v27b provides the TranslateGemma 27B model contract.
package v27b

import (
	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/models/internal/translategemma"
)

// Model implements transliter.Model for Google TranslateGemma 27B IT.
type Model struct{ core translategemma.Core }

// New constructs the TranslateGemma 27B model contract.
func New() Model {
	return Model{core: translategemma.New(transliter.Descriptor{
		ID:         "translategemma-27b",
		Family:     "TranslateGemma",
		Parameters: "27B",
		Repository: "google/translategemma-27b-it",
	})}
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
