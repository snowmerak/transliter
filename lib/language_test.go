package transliter

import "testing"

func TestSupportedLanguagesAreUniqueAndValid(t *testing.T) {
	languages := SupportedLanguages()
	if len(languages) != 61 {
		t.Fatalf("SupportedLanguages returned %d languages, want 61", len(languages))
	}
	seen := make(map[Language]struct{}, len(languages))
	for _, language := range languages {
		if !language.Valid() {
			t.Errorf("declared language %q is not valid", language)
		}
		if _, exists := seen[language]; exists {
			t.Errorf("duplicate language %q", language)
		}
		seen[language] = struct{}{}
	}
}

func TestSupportedLanguagesReturnsCopy(t *testing.T) {
	languages := SupportedLanguages()
	languages[0] = Language("Changed")
	if SupportedLanguages()[0] != LanguageChinese {
		t.Fatal("SupportedLanguages exposed mutable package state")
	}
}

func TestUnknownLanguageIsInvalid(t *testing.T) {
	if Language("Klingon").Valid() {
		t.Fatal("unknown language reported as valid")
	}
}

func TestParseLanguage(t *testing.T) {
	language, err := ParseLanguage("Korean")
	if err != nil {
		t.Fatalf("ParseLanguage returned error: %v", err)
	}
	if language != LanguageKorean {
		t.Fatalf("ParseLanguage returned %q, want %q", language, LanguageKorean)
	}
	if _, err := ParseLanguage("korean"); err == nil {
		t.Fatal("ParseLanguage accepted a non-canonical language name")
	}
}
