package inference

import (
	"testing"

	transliter "github.com/snowmerak/translter/lib"
)

func TestRequestReturnsDefensiveCopies(t *testing.T) {
	temperature := 0.7
	originalInput := transliter.ModelInput{Messages: []transliter.Message{{
		Role: transliter.RoleUser,
		Parts: []transliter.ContentPart{{
			Type: "text",
			Text: "hello",
		}},
	}}}
	originalOptions := transliter.GenerationOptions{
		Temperature: &temperature,
		Stop:        []string{"stop"},
	}

	request := NewRequest("local-model", originalInput, originalOptions)
	firstInput := request.ModelInput()
	firstOptions := request.GenerationOptions()
	firstInput.Messages[0].Parts[0].Text = "changed"
	*firstOptions.Temperature = 1
	firstOptions.Stop[0] = "changed"

	secondInput := request.ModelInput()
	secondOptions := request.GenerationOptions()
	if secondInput.Messages[0].Parts[0].Text != "hello" {
		t.Fatal("request input was mutated through an accessor")
	}
	if *secondOptions.Temperature != 0.7 || secondOptions.Stop[0] != "stop" {
		t.Fatal("request options were mutated through an accessor")
	}
}

func TestResponseContract(t *testing.T) {
	response := NewResponse("안녕하세요", "local-model", "stop", Usage{
		PromptTokens:     10,
		CompletionTokens: 3,
		TotalTokens:      13,
	})

	if response.OutputText() != "안녕하세요" {
		t.Fatalf("unexpected output: %q", response.OutputText())
	}
	if response.ProviderModel() != "local-model" || response.FinishReason() != "stop" {
		t.Fatal("response metadata was not preserved")
	}
	if response.TokenUsage().TotalTokens != 13 {
		t.Fatal("response usage was not preserved")
	}
}
