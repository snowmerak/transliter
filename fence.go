package transliter

import (
	"fmt"
	"strings"
)

// LongestBacktickRun returns the longest consecutive backtick run in text.
func LongestBacktickRun(text string) int {
	longest := 0
	current := 0
	for _, r := range text {
		if r == '`' {
			current++
			if current > longest {
				longest = current
			}
			continue
		}
		current = 0
	}
	return longest
}

// RequiredFenceLength returns a fence length that is at least minimum and
// strictly longer than every backtick run in text.
func RequiredFenceLength(text string, minimum int) (int, error) {
	if minimum < 1 {
		return 0, fmt.Errorf("minimum must be positive")
	}
	length := LongestBacktickRun(text) + 1
	if length < minimum {
		length = minimum
	}
	return length, nil
}

// FenceSource wraps source text in a dynamically sized Markdown backtick
// fence. The optional label must be a single line without backticks.
func FenceSource(text string, label ...string) (string, error) {
	info := ""
	if len(label) > 1 {
		return "", fmt.Errorf("at most one fence label is allowed")
	}
	if len(label) == 1 {
		info = label[0]
	}
	if strings.ContainsAny(info, "`\r\n") {
		return "", fmt.Errorf("fence label must be a single line without backticks")
	}
	length, err := RequiredFenceLength(text, 4)
	if err != nil {
		return "", err
	}
	fence := strings.Repeat("`", length)
	return fence + info + "\n" + text + "\n" + fence, nil
}
