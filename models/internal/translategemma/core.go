package translategemma

import (
	"fmt"

	transliter "github.com/snowmerak/transliter/lib"
)

// Core implements the official text chat-template contract shared by all
// TranslateGemma sizes.
type Core struct {
	descriptor transliter.Descriptor
}

// New constructs a size-specific TranslateGemma core.
func New(descriptor transliter.Descriptor) Core {
	return Core{descriptor: descriptor}
}

func (core Core) Descriptor() transliter.Descriptor {
	return core.descriptor
}

func (Core) Capabilities() transliter.Capabilities {
	return transliter.Capabilities{
		PromptKinds:            []transliter.PromptKind{transliter.PromptText},
		RequiresSourceLanguage: true,
		StructuredUserContent:  true,
		MaxInputTokens:         2048,
	}
}

func (Core) SupportsLanguage(language transliter.Language) bool {
	_, ok := languageCodes[language]
	return ok
}

func (core Core) BuildInput(request transliter.TranslationRequest) (transliter.ModelInput, error) {
	if err := transliter.ValidateTranslationRequest(request); err != nil {
		return transliter.ModelInput{}, err
	}
	kind := request.Kind
	if kind == "" {
		kind = transliter.PromptText
	}
	if kind != transliter.PromptText {
		return transliter.ModelInput{}, fmt.Errorf("%s official chat template supports only text translation", core.descriptor.ID)
	}
	if request.SourceLanguage == "" {
		return transliter.ModelInput{}, fmt.Errorf("%s requires a source language", core.descriptor.ID)
	}
	if hasExtendedInstructions(request) {
		return transliter.ModelInput{}, fmt.Errorf("%s official chat template does not support glossary, style, audience, attributes, or delimiters", core.descriptor.ID)
	}
	sourceCode, sourceOK := languageCodes[request.SourceLanguage]
	if !sourceOK {
		return transliter.ModelInput{}, fmt.Errorf("%s does not support source language %q", core.descriptor.ID, request.SourceLanguage)
	}
	targetCode, targetOK := languageCodes[request.TargetLanguage]
	if !targetOK {
		return transliter.ModelInput{}, fmt.Errorf("%s does not support target language %q", core.descriptor.ID, request.TargetLanguage)
	}
	return transliter.ModelInput{Messages: []transliter.Message{{
		Role: transliter.RoleUser,
		Parts: []transliter.ContentPart{{
			Type:               "text",
			Text:               request.Source,
			SourceLanguageCode: sourceCode,
			TargetLanguageCode: targetCode,
		}},
	}}}, nil
}

func (Core) Options(profile transliter.OptionProfile) (transliter.GenerationOptions, error) {
	switch profile {
	case transliter.ProfileOfficial:
		return transliter.GenerationOptions{
			DoSample:        transliter.Pointer(false),
			MaxOutputTokens: 200,
			Provenance:      transliter.ProvenanceOfficialExample,
		}, nil
	case transliter.ProfileDeterministic:
		return transliter.GenerationOptions{
			DoSample:        transliter.Pointer(false),
			MaxOutputTokens: 200,
			Provenance:      transliter.ProvenanceProjectExperimental,
		}, nil
	default:
		return transliter.GenerationOptions{}, fmt.Errorf("unsupported option profile %q", profile)
	}
}

func hasExtendedInstructions(request transliter.TranslationRequest) bool {
	return len(request.Glossary) > 0 ||
		request.Style != "" ||
		request.Audience != "" ||
		len(request.TranslatableAttributes) > 0 ||
		len(request.Delimiters) > 0
}

var languageCodes = map[transliter.Language]string{
	transliter.LanguageEnglish:            "en",
	transliter.LanguageSpanish:            "es",
	transliter.LanguageFrench:             "fr",
	transliter.LanguageGerman:             "de",
	transliter.LanguageItalian:            "it",
	transliter.LanguagePortuguese:         "pt",
	transliter.LanguageDutch:              "nl",
	transliter.LanguageSwedish:            "sv",
	transliter.LanguageNorwegian:          "no",
	transliter.LanguageDanish:             "da",
	transliter.LanguageFinnish:            "fi",
	transliter.LanguageIcelandic:          "is",
	transliter.LanguageRussian:            "ru",
	transliter.LanguageUkrainian:          "uk",
	transliter.LanguagePolish:             "pl",
	transliter.LanguageCzech:              "cs",
	transliter.LanguageSlovak:             "sk",
	transliter.LanguageHungarian:          "hu",
	transliter.LanguageRomanian:           "ro",
	transliter.LanguageBulgarian:          "bg",
	transliter.LanguageCroatian:           "hr",
	transliter.LanguageSerbian:            "sr",
	transliter.LanguageBosnian:            "bs",
	transliter.LanguageSlovenian:          "sl",
	transliter.LanguageGreek:              "el",
	transliter.LanguageTurkish:            "tr",
	transliter.LanguageArabic:             "ar",
	transliter.LanguageHebrew:             "he",
	transliter.LanguagePersian:            "fa",
	transliter.LanguageHindi:              "hi",
	transliter.LanguageUrdu:               "ur",
	transliter.LanguageBengali:            "bn",
	transliter.LanguagePunjabi:            "pa",
	transliter.LanguageGujarati:           "gu",
	transliter.LanguageMarathi:            "mr",
	transliter.LanguageTamil:              "ta",
	transliter.LanguageTelugu:             "te",
	transliter.LanguageKannada:            "kn",
	transliter.LanguageMalayalam:          "ml",
	transliter.LanguageSinhala:            "si",
	transliter.LanguageNepali:             "ne",
	transliter.LanguageIndonesian:         "id",
	transliter.LanguageMalay:              "ms",
	transliter.LanguageFilipino:           "tl",
	transliter.LanguageThai:               "th",
	transliter.LanguageVietnamese:         "vi",
	transliter.LanguageKhmer:              "km",
	transliter.LanguageChinese:            "zh",
	transliter.LanguageTraditionalChinese: "zh-TW",
	transliter.LanguageJapanese:           "ja",
	transliter.LanguageKorean:             "ko",
	transliter.LanguageSwahili:            "sw",
	transliter.LanguageAmharic:            "am",
	transliter.LanguageYoruba:             "yo",
	transliter.LanguageHausa:              "ha",
}
