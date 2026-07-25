package transliter

import (
	"encoding/json"
	"os"
	"testing"
)

type fixtureCase struct {
	ID                     string            `json:"id"`
	Kind                   PromptKind        `json:"kind"`
	Source                 string            `json:"source"`
	Expected               string            `json:"expected"`
	Identifiers            []string          `json:"identifiers"`
	Delimiters             []string          `json:"delimiters"`
	TranslatableAttributes []string          `json:"translatable_attributes"`
	Glossary               map[string]string `json:"glossary"`
	Concerns               []string          `json:"concerns"`
	SourceLanguage         string            `json:"source_language"`
	TargetLanguage         string            `json:"target_language"`
}

func loadFixtureCases(t *testing.T) []fixtureCase {
	t.Helper()
	data, err := os.ReadFile("testdata/cases.json")
	if err != nil {
		t.Fatalf("read fixtures: %v", err)
	}
	var cases []fixtureCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("parse fixtures: %v", err)
	}
	return cases
}

func TestManualFixturesSatisfyStructuralContract(t *testing.T) {
	for _, fixture := range loadFixtureCases(t) {
		t.Run(fixture.ID, func(t *testing.T) {
			result := ValidateTranslation(fixture.Source, fixture.Expected, ValidationOptions{
				Kind:                   fixture.Kind,
				Identifiers:            fixture.Identifiers,
				Delimiters:             fixture.Delimiters,
				TranslatableAttributes: fixture.TranslatableAttributes,
			})
			if !result.OK() {
				t.Fatalf("fixture failed validation: %+v", result.Issues)
			}
		})
	}
}

func TestValidatorDetectsPreservationAndPrefixFailures(t *testing.T) {
	source := "Visit https://example.com for {{username}} using user_id."
	output := "Translation: https://wrong.example에서 사용자에게 안내합니다."
	result := ValidateTranslation(source, output, ValidationOptions{Identifiers: []string{"user_id"}})
	requireIssueCodes(t, result,
		"unexpected_prefix",
		"placeholders_changed",
		"urls_changed",
		"identifier_changed",
	)
}

func TestValidatorDetectsInvalidJSONAndMachineValueChanges(t *testing.T) {
	invalid := ValidateTranslation(`{"count":2}`, `{"count":`, ValidationOptions{Kind: PromptJSON})
	requireIssueCodes(t, invalid, "json_invalid")

	changed := ValidateTranslation(`{"count":2}`, `{"count":3}`, ValidationOptions{Kind: PromptJSON})
	requireIssueCodes(t, changed, "json_structure_changed")
}

func TestValidatorDetectsInvalidYAML(t *testing.T) {
	result := ValidateTranslation("message: hello", "message: [", ValidationOptions{Kind: PromptYAML})
	requireIssueCodes(t, result, "yaml_invalid")
}

func TestValidatorDetectsYAMLIndentationAndTagChanges(t *testing.T) {
	source := "message: !i18n Hello\nnested:\n  value: text"
	output := "message: Hello\nnested:\n value: 텍스트"
	result := ValidateTranslation(source, output, ValidationOptions{Kind: PromptYAML})
	requireIssueCodes(t, result, "yaml_indentation_changed", "yaml_tokens_changed")
}

func TestValidatorDetectsChangedMarkdownFences(t *testing.T) {
	result := ValidateTranslation("```\ncode\n```", "````\ncode\n````", ValidationOptions{})
	requireIssueCodes(t, result, "markdown_fences_changed")
}

func TestValidatorDetectsChangedHTMLAttribute(t *testing.T) {
	source := `<a href="https://example.com" id="guide">Read</a>`
	output := `<a href="https://example.com" id="안내">읽기</a>`
	result := ValidateTranslation(source, output, ValidationOptions{Kind: PromptHTMLXML})
	requireIssueCodes(t, result, "markup_structure_changed")
}

func TestValidatorDetectsInvalidXML(t *testing.T) {
	source := `<?xml version="1.0"?><message><text>Hello</text></message>`
	output := `<?xml version="1.0"?><message><text>안녕하세요</message>`
	result := ValidateTranslation(source, output, ValidationOptions{Kind: PromptHTMLXML})
	requireIssueCodes(t, result, "xml_invalid")
}

func TestValidatorDetectsDelimiterReordering(t *testing.T) {
	result := ValidateTranslation(
		"<A>one<B>two",
		"<B>하나<A>둘",
		ValidationOptions{Delimiters: []string{"<A>", "<B>"}},
	)
	requireIssueCodes(t, result, "delimiters_changed")
}

func TestFixturesCoverRequiredScenarios(t *testing.T) {
	concerns := make(map[string]struct{})
	for _, fixture := range loadFixtureCases(t) {
		for _, concern := range fixture.Concerns {
			concerns[concern] = struct{}{}
		}
	}
	required := []string{
		"general English to Korean",
		"Japanese to Korean",
		"Korean to English",
		"question is translated, not answered",
		"triple backticks",
		"quadruple backticks",
		"keys",
		"indentation",
		"tags",
		"mustache",
		"URL",
		"required terminology",
		"code",
		"empty input",
		"very short input",
		"long input",
		"already in target language",
		"multilingual input",
		"multiple files",
	}
	for _, concern := range required {
		if _, ok := concerns[concern]; !ok {
			t.Errorf("fixtures do not cover %q", concern)
		}
	}
}

func requireIssueCodes(t *testing.T, result ValidationResult, required ...string) {
	t.Helper()
	codes := make(map[string]struct{}, len(result.Issues))
	for _, validationIssue := range result.Issues {
		codes[validationIssue.Code] = struct{}{}
	}
	for _, code := range required {
		if _, ok := codes[code]; !ok {
			t.Errorf("missing issue code %q in %+v", code, result.Issues)
		}
	}
}
