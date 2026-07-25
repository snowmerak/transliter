package transliter

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMessageMarshalsPlainTextContent(t *testing.T) {
	data, err := json.Marshal(Message{Role: RoleUser, Text: "prompt"})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	if string(data) != `{"role":"user","content":"prompt"}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
}

func TestMessageMarshalsStructuredContent(t *testing.T) {
	data, err := json.Marshal(Message{
		Role: RoleUser,
		Parts: []ContentPart{{
			Type:               "text",
			Text:               "Hello",
			SourceLanguageCode: "en",
			TargetLanguageCode: "ko",
		}},
	})
	if err != nil {
		t.Fatalf("Marshal returned error: %v", err)
	}
	for _, expected := range []string{`"content":[`, `"source_lang_code":"en"`, `"target_lang_code":"ko"`} {
		if !strings.Contains(string(data), expected) {
			t.Fatalf("JSON missing %s: %s", expected, data)
		}
	}
}

func TestMessageRejectsMixedContent(t *testing.T) {
	_, err := json.Marshal(Message{
		Role:  RoleUser,
		Text:  "prompt",
		Parts: []ContentPart{{Type: "text", Text: "source"}},
	})
	if err == nil {
		t.Fatal("Marshal accepted both text and parts")
	}
}
