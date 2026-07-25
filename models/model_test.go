package models_test

import (
	"encoding/json"
	"strings"
	"testing"

	transliter "github.com/snowmerak/transliter/lib"
	hymt2v1p8b "github.com/snowmerak/transliter/models/hymt2/v1p8b"
	hymt2v30ba3b "github.com/snowmerak/transliter/models/hymt2/v30ba3b"
	hymt2v7b "github.com/snowmerak/transliter/models/hymt2/v7b"
	gemmav12b "github.com/snowmerak/transliter/models/translategemma/v12b"
	gemmav27b "github.com/snowmerak/transliter/models/translategemma/v27b"
	gemmav4b "github.com/snowmerak/transliter/models/translategemma/v4b"
)

func TestEveryPackageImplementsModelWithUniqueID(t *testing.T) {
	models := []transliter.Model{
		hymt2v1p8b.New(),
		hymt2v7b.New(),
		hymt2v30ba3b.New(),
		gemmav4b.New(),
		gemmav12b.New(),
		gemmav27b.New(),
	}
	seen := make(map[transliter.ModelID]struct{}, len(models))
	for _, model := range models {
		descriptor := model.Descriptor()
		if descriptor.ID == "" || descriptor.Repository == "" {
			t.Errorf("incomplete descriptor: %+v", descriptor)
		}
		if _, exists := seen[descriptor.ID]; exists {
			t.Errorf("duplicate model ID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
	}
}

func TestHyMT2PublishedOptionsAreSeparatedBySize(t *testing.T) {
	small, err := hymt2v1p8b.New().Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("1.8B options: %v", err)
	}
	medium, err := hymt2v7b.New().Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("7B options: %v", err)
	}
	large, err := hymt2v30ba3b.New().Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("30B-A3B options: %v", err)
	}
	assertFloat(t, "1.8B top_p", small.TopP, 0.6)
	assertInt(t, "1.8B top_k", small.TopK, 20)
	assertFloat(t, "1.8B repetition penalty", small.RepetitionPenalty, 1.05)
	assertFloat(t, "7B top_p", medium.TopP, 0.6)
	assertInt(t, "7B top_k", medium.TopK, 20)
	assertFloat(t, "30B top_p", large.TopP, 1.0)
	assertInt(t, "30B top_k", large.TopK, -1)
	assertFloat(t, "30B repetition penalty", large.RepetitionPenalty, 1.0)
	for _, options := range []transliter.GenerationOptions{small, medium, large} {
		assertFloat(t, "temperature", options.Temperature, 0.7)
		if options.MaxOutputTokens != 4096 {
			t.Errorf("MaxOutputTokens = %d, want 4096", options.MaxOutputTokens)
		}
		if options.Provenance != transliter.ProvenanceOfficialRecommendation {
			t.Errorf("unexpected provenance %q", options.Provenance)
		}
	}
}

func TestModelOptionsDoNotExposeMutableState(t *testing.T) {
	model := hymt2v1p8b.New()
	options, err := model.Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("Options returned error: %v", err)
	}
	*options.Temperature = 9
	fresh, err := model.Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("Options returned error: %v", err)
	}
	assertFloat(t, "fresh temperature", fresh.Temperature, 0.7)
}

func TestHyMT2BuildsPlainUserPrompt(t *testing.T) {
	input, err := hymt2v30ba3b.New().BuildInput(transliter.TranslationRequest{
		Source:         "Hello",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
		Kind:           transliter.PromptText,
	})
	if err != nil {
		t.Fatalf("BuildInput returned error: %v", err)
	}
	if len(input.Messages) != 1 || input.Messages[0].Text == "" || len(input.Messages[0].Parts) != 0 {
		t.Fatalf("unexpected Hy-MT2 input: %+v", input)
	}
	if !strings.Contains(input.Messages[0].Text, "Translation contract:") {
		t.Fatalf("Hy-MT2 prompt omitted contract: %s", input.Messages[0].Text)
	}
}

func TestTranslateGemmaBuildsOfficialStructuredContent(t *testing.T) {
	model := gemmav12b.New()
	input, err := model.BuildInput(transliter.TranslationRequest{
		Source:         "Hello",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
		Kind:           transliter.PromptText,
	})
	if err != nil {
		t.Fatalf("BuildInput returned error: %v", err)
	}
	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	jsonText := string(data)
	for _, expected := range []string{
		`"content":[`,
		`"type":"text"`,
		`"text":"Hello"`,
		`"source_lang_code":"en"`,
		`"target_lang_code":"ko"`,
	} {
		if !strings.Contains(jsonText, expected) {
			t.Errorf("TranslateGemma input missing %s: %s", expected, jsonText)
		}
	}
	if strings.Contains(jsonText, "Translation contract") {
		t.Fatalf("TranslateGemma content must contain only source text: %s", jsonText)
	}
}

func TestTranslateGemmaEnforcesOfficialTemplateLimits(t *testing.T) {
	model := gemmav4b.New()
	base := transliter.TranslationRequest{
		Source:         "Hello",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
	}
	tests := []transliter.TranslationRequest{
		func() transliter.TranslationRequest {
			request := base
			request.Kind = transliter.PromptMarkdown
			return request
		}(),
		func() transliter.TranslationRequest {
			request := base
			request.Glossary = map[string]string{"Hello": "안녕하세요"}
			return request
		}(),
		func() transliter.TranslationRequest {
			request := base
			request.SourceLanguage = ""
			return request
		}(),
		func() transliter.TranslationRequest {
			request := base
			request.SourceLanguage = transliter.LanguageBurmese
			return request
		}(),
	}
	for _, request := range tests {
		if _, err := model.BuildInput(request); err == nil {
			t.Errorf("BuildInput accepted unsupported request: %+v", request)
		}
	}
}

func TestModelLanguageSubsets(t *testing.T) {
	hymt2 := hymt2v7b.New()
	gemma := gemmav27b.New()
	var hyCount, gemmaCount int
	for _, language := range transliter.SupportedLanguages() {
		if hymt2.SupportsLanguage(language) {
			hyCount++
		}
		if gemma.SupportsLanguage(language) {
			gemmaCount++
		}
	}
	if hyCount != 38 {
		t.Errorf("Hy-MT2 language count = %d, want 38", hyCount)
	}
	if gemmaCount != 55 {
		t.Errorf("TranslateGemma language count = %d, want 55", gemmaCount)
	}
}

func TestTranslateGemmaOfficialOptionsFollowModelCardExample(t *testing.T) {
	options, err := gemmav4b.New().Options(transliter.ProfileOfficial)
	if err != nil {
		t.Fatalf("Options returned error: %v", err)
	}
	if options.DoSample == nil || *options.DoSample {
		t.Errorf("DoSample = %v, want false", options.DoSample)
	}
	if options.MaxOutputTokens != 200 {
		t.Errorf("MaxOutputTokens = %d, want 200", options.MaxOutputTokens)
	}
	if options.Provenance != transliter.ProvenanceOfficialExample {
		t.Errorf("Provenance = %q, want official example", options.Provenance)
	}
}

func assertFloat(t *testing.T, name string, value *float64, want float64) {
	t.Helper()
	if value == nil || *value != want {
		t.Errorf("%s = %v, want %v", name, value, want)
	}
}

func assertInt(t *testing.T, name string, value *int, want int) {
	t.Helper()
	if value == nil || *value != want {
		t.Errorf("%s = %v, want %v", name, value, want)
	}
}
