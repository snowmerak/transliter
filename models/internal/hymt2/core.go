package hymt2

import (
	"fmt"

	transliter "github.com/snowmerak/transliter/lib"
)

// Core implements behavior shared by the three Hy-MT2 size packages.
type Core struct {
	descriptor transliter.Descriptor
	official   transliter.GenerationOptions
}

// New constructs a size-specific Hy-MT2 core.
func New(descriptor transliter.Descriptor, official transliter.GenerationOptions) Core {
	return Core{descriptor: descriptor, official: official}
}

func (core Core) Descriptor() transliter.Descriptor {
	return core.descriptor
}

func (Core) Capabilities() transliter.Capabilities {
	kinds := make([]transliter.PromptKind, len(transliter.PromptKinds))
	copy(kinds, transliter.PromptKinds)
	return transliter.Capabilities{
		PromptKinds:            kinds,
		Glossary:               true,
		Style:                  true,
		Audience:               true,
		TranslatableAttributes: true,
		Delimiters:             true,
	}
}

func (Core) SupportsLanguage(language transliter.Language) bool {
	_, ok := supportedLanguages[language]
	return ok
}

func (core Core) BuildInput(request transliter.TranslationRequest) (transliter.ModelInput, error) {
	if !core.SupportsLanguage(request.TargetLanguage) {
		return transliter.ModelInput{}, fmt.Errorf("%s does not support target language %q", core.descriptor.ID, request.TargetLanguage)
	}
	if request.SourceLanguage != "" && !core.SupportsLanguage(request.SourceLanguage) {
		return transliter.ModelInput{}, fmt.Errorf("%s does not support source language %q", core.descriptor.ID, request.SourceLanguage)
	}
	prompt, err := transliter.BuildPrompt(request)
	if err != nil {
		return transliter.ModelInput{}, err
	}
	return transliter.ModelInput{Messages: []transliter.Message{{
		Role: transliter.RoleUser,
		Text: prompt,
	}}}, nil
}

func (core Core) Options(profile transliter.OptionProfile) (transliter.GenerationOptions, error) {
	switch profile {
	case transliter.ProfileOfficial:
		return cloneOptions(core.official), nil
	case transliter.ProfileDeterministic:
		options := cloneOptions(core.official)
		options.Temperature = transliter.Pointer(0.1)
		options.Provenance = transliter.ProvenanceProjectExperimental
		return options, nil
	default:
		return transliter.GenerationOptions{}, fmt.Errorf("unsupported option profile %q", profile)
	}
}

func cloneOptions(options transliter.GenerationOptions) transliter.GenerationOptions {
	clone := options
	clone.Temperature = clonePointer(options.Temperature)
	clone.TopP = clonePointer(options.TopP)
	clone.TopK = clonePointer(options.TopK)
	clone.RepetitionPenalty = clonePointer(options.RepetitionPenalty)
	clone.DoSample = clonePointer(options.DoSample)
	clone.Stop = append([]string(nil), options.Stop...)
	return clone
}

func clonePointer[T any](value *T) *T {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

var supportedLanguages = map[transliter.Language]struct{}{
	transliter.LanguageChinese:            {},
	transliter.LanguageEnglish:            {},
	transliter.LanguageFrench:             {},
	transliter.LanguagePortuguese:         {},
	transliter.LanguageSpanish:            {},
	transliter.LanguageJapanese:           {},
	transliter.LanguageTurkish:            {},
	transliter.LanguageRussian:            {},
	transliter.LanguageArabic:             {},
	transliter.LanguageKorean:             {},
	transliter.LanguageThai:               {},
	transliter.LanguageItalian:            {},
	transliter.LanguageGerman:             {},
	transliter.LanguageVietnamese:         {},
	transliter.LanguageMalay:              {},
	transliter.LanguageIndonesian:         {},
	transliter.LanguageFilipino:           {},
	transliter.LanguageHindi:              {},
	transliter.LanguageTraditionalChinese: {},
	transliter.LanguagePolish:             {},
	transliter.LanguageCzech:              {},
	transliter.LanguageDutch:              {},
	transliter.LanguageKhmer:              {},
	transliter.LanguageBurmese:            {},
	transliter.LanguagePersian:            {},
	transliter.LanguageGujarati:           {},
	transliter.LanguageUrdu:               {},
	transliter.LanguageTelugu:             {},
	transliter.LanguageMarathi:            {},
	transliter.LanguageHebrew:             {},
	transliter.LanguageBengali:            {},
	transliter.LanguageTamil:              {},
	transliter.LanguageUkrainian:          {},
	transliter.LanguageTibetan:            {},
	transliter.LanguageKazakh:             {},
	transliter.LanguageMongolian:          {},
	transliter.LanguageUyghur:             {},
	transliter.LanguageCantonese:          {},
}
