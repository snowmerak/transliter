package transliter

import (
	"strings"
	"testing"
)

func TestRequiredFenceLength(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		minimum int
		want    int
	}{
		{name: "plain source", source: "plain", minimum: 4, want: 4},
		{name: "triple and quadruple", source: "```\ncode\n```\n````\ninner\n````", minimum: 4, want: 5},
		{name: "long run", source: strings.Repeat("`", 11), minimum: 4, want: 12},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := RequiredFenceLength(test.source, test.minimum)
			if err != nil {
				t.Fatalf("RequiredFenceLength returned error: %v", err)
			}
			if got != test.want {
				t.Fatalf("RequiredFenceLength() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestRequiredFenceLengthRejectsInvalidMinimum(t *testing.T) {
	if _, err := RequiredFenceLength("source", 0); err == nil {
		t.Fatal("RequiredFenceLength accepted zero minimum")
	}
}

func TestFenceSourceUsesMatchingSafeFence(t *testing.T) {
	source := "````\ninner\n````"
	got, err := FenceSource(source)
	if err != nil {
		t.Fatalf("FenceSource returned error: %v", err)
	}
	if !strings.HasPrefix(got, "`````\n") || !strings.HasSuffix(got, "\n`````") {
		t.Fatalf("FenceSource used an unsafe or mismatched fence:\n%s", got)
	}
}

func TestFenceSourceRejectsInvalidLabel(t *testing.T) {
	if _, err := FenceSource("source", "bad\nlabel"); err == nil {
		t.Fatal("FenceSource accepted a multiline label")
	}
}
