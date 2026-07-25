package transliter

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

var (
	placeholderPattern = regexp.MustCompile(`\{\{[^{}\r\n]+\}\}|\$\{[^{}\r\n]+\}|%(?:\([^)]+\))?[#0\- +]?\d*(?:\.\d+)?[a-zA-Z]|\{(?:\d+|[A-Za-z_][A-Za-z0-9_.]*)\}`)
	urlPattern         = regexp.MustCompile(`https?://[A-Za-z0-9._~:/?#\[\]@!$&'()*+;=%-]*[A-Za-z0-9_~/#\]=%-]`)
	emailPattern       = regexp.MustCompile(`[A-Za-z0-9_.+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	windowsPathPattern = regexp.MustCompile(`(?:[A-Za-z]:\\|\\\\)(?:[A-Za-z0-9_.@+~-]+\\)*[A-Za-z0-9_.@+~-]*[A-Za-z0-9_@+~-]`)
	posixPathPattern   = regexp.MustCompile(`/(?:[A-Za-z0-9_.@+~-]+/)+[A-Za-z0-9_.@+~-]*[A-Za-z0-9_@+~-]`)
	entityPattern      = regexp.MustCompile(`&(?:#[0-9]+|#x[0-9A-Fa-f]+|[A-Za-z][A-Za-z0-9]+);`)
	leadingNoise       = regexp.MustCompile(`(?i)^\s*(?:translation|translated (?:text|result)|result|answer|note|explanation|here is (?:the )?translation)\s*:`)
)

// ValidationIssue is one mechanical contract failure.
type ValidationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ValidationResult contains all detected contract failures.
type ValidationResult struct {
	Issues []ValidationIssue `json:"issues"`
}

// OK reports whether no contract failures were found.
func (result ValidationResult) OK() bool {
	return len(result.Issues) == 0
}

// ValidationOptions selects format-specific and caller-supplied checks.
type ValidationOptions struct {
	Kind                   PromptKind
	Identifiers            []string
	Delimiters             []string
	TranslatableAttributes []string
}

// ValidateTranslation checks structure and output-contract preservation. It
// intentionally does not score linguistic quality.
func ValidateTranslation(source, output string, options ValidationOptions) ValidationResult {
	var issues []ValidationIssue
	if source == "" && output != "" {
		issues = append(issues, issue("empty_source_output", "empty source must produce empty output"))
	}
	if leadingNoise.MatchString(output) {
		issues = append(issues, issue("unexpected_prefix", "output starts with an unwanted label or explanation"))
	}

	issues = comparePattern(issues, "placeholders_changed", "placeholders", placeholderPattern, source, output)
	issues = comparePattern(issues, "urls_changed", "URLs", urlPattern, source, output)
	issues = comparePattern(issues, "emails_changed", "email addresses", emailPattern, source, output)
	issues = comparePattern(issues, "windows_paths_changed", "Windows paths", windowsPathPattern, source, output)
	issues = comparePattern(issues, "posix_paths_changed", "POSIX paths", posixPathPattern, source, output)

	sourceFences := markdownFences(source)
	outputFences := markdownFences(output)
	if !equalCounts(sourceFences, outputFences) {
		issues = append(issues, issue(
			"markdown_fences_changed",
			fmt.Sprintf("Markdown fence types or counts changed: expected %v, got %v", sourceFences, outputFences),
		))
	}

	for _, identifier := range options.Identifiers {
		expected := strings.Count(source, identifier)
		actual := strings.Count(output, identifier)
		if expected != actual {
			issues = append(issues, issue(
				"identifier_changed",
				fmt.Sprintf("identifier %q count changed from %d to %d", identifier, expected, actual),
			))
		}
	}

	expectedDelimiters := delimiterSequence(source, options.Delimiters)
	actualDelimiters := delimiterSequence(output, options.Delimiters)
	if !equalStrings(expectedDelimiters, actualDelimiters) {
		issues = append(issues, issue(
			"delimiters_changed",
			fmt.Sprintf("delimiter sequence changed: expected %v, got %v", expectedDelimiters, actualDelimiters),
		))
	}

	switch options.Kind {
	case PromptJSON:
		issues = addJSONChecks(issues, source, output)
	case PromptYAML:
		issues = addYAMLChecks(issues, source, output)
	case PromptHTMLXML:
		issues = addMarkupChecks(issues, source, output, options.TranslatableAttributes)
	}
	return ValidationResult{Issues: issues}
}

func issue(code, message string) ValidationIssue {
	return ValidationIssue{Code: code, Message: message}
}

func comparePattern(issues []ValidationIssue, code, label string, pattern *regexp.Regexp, source, output string) []ValidationIssue {
	expected := matchCounts(pattern, source)
	actual := matchCounts(pattern, output)
	if !equalCounts(expected, actual) {
		return append(issues, issue(code, fmt.Sprintf("%s changed: expected %v, got %v", label, expected, actual)))
	}
	return issues
}

func matchCounts(pattern *regexp.Regexp, text string) map[string]int {
	counts := make(map[string]int)
	for _, token := range pattern.FindAllString(text, -1) {
		counts[token]++
	}
	return counts
}

func equalCounts(left, right map[string]int) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func markdownFences(text string) map[string]int {
	counts := make(map[string]int)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		if indent > 3 || len(trimmed) < 3 {
			continue
		}
		marker := trimmed[0]
		if marker != '`' && marker != '~' {
			continue
		}
		length := 0
		for length < len(trimmed) && trimmed[length] == marker {
			length++
		}
		if length >= 3 {
			counts[string(marker)+strconv.Itoa(length)]++
		}
	}
	return counts
}

func delimiterSequence(text string, delimiters []string) []string {
	if len(delimiters) == 0 {
		return nil
	}
	unique := make(map[string]struct{})
	candidates := make([]string, 0, len(delimiters))
	for _, delimiter := range delimiters {
		if delimiter == "" {
			continue
		}
		if _, exists := unique[delimiter]; !exists {
			unique[delimiter] = struct{}{}
			candidates = append(candidates, delimiter)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i]) > len(candidates[j])
	})
	var sequence []string
	for offset := 0; offset < len(text); {
		match := ""
		for _, candidate := range candidates {
			if strings.HasPrefix(text[offset:], candidate) {
				match = candidate
				break
			}
		}
		if match == "" {
			offset++
			continue
		}
		sequence = append(sequence, match)
		offset += len(match)
	}
	return sequence
}

func addJSONChecks(issues []ValidationIssue, source, output string) []ValidationIssue {
	sourceValue, err := decodeJSON(source)
	if err != nil {
		return append(issues, issue("source_json_invalid", "source fixture is invalid JSON: "+err.Error()))
	}
	outputValue, err := decodeJSON(output)
	if err != nil {
		return append(issues, issue("json_invalid", "output is not valid JSON: "+err.Error()))
	}
	if jsonShape(sourceValue) != jsonShape(outputValue) {
		issues = append(issues, issue("json_structure_changed", "JSON keys, structure, or machine values changed"))
	}
	return issues
}

func decodeJSON(text string) (any, error) {
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("multiple JSON values")
		}
		return nil, err
	}
	return value, nil
}

func jsonShape(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var builder strings.Builder
		builder.WriteString("object{")
		for _, key := range keys {
			builder.WriteString(strconv.Quote(key))
			builder.WriteByte(':')
			builder.WriteString(jsonShape(typed[key]))
			builder.WriteByte(';')
		}
		builder.WriteByte('}')
		return builder.String()
	case []any:
		var builder strings.Builder
		builder.WriteString("array[")
		for _, child := range typed {
			builder.WriteString(jsonShape(child))
			builder.WriteByte(';')
		}
		builder.WriteByte(']')
		return builder.String()
	case json.Number:
		return "number:" + typed.String()
	case bool:
		return "boolean:" + strconv.FormatBool(typed)
	case nil:
		return "null"
	default:
		return "string"
	}
}

func addYAMLChecks(issues []ValidationIssue, source, output string) []ValidationIssue {
	var sourceNode yaml.Node
	if err := yaml.Unmarshal([]byte(source), &sourceNode); err != nil {
		return append(issues, issue("source_yaml_invalid", "source fixture is invalid YAML: "+err.Error()))
	}
	var outputNode yaml.Node
	if err := yaml.Unmarshal([]byte(output), &outputNode); err != nil {
		return append(issues, issue("yaml_invalid", "output is not valid YAML: "+err.Error()))
	}
	if yamlShape(&sourceNode, true) != yamlShape(&outputNode, true) {
		issues = append(issues, issue("yaml_structure_changed", "YAML keys, structure, or machine values changed"))
	}
	if !equalInts(indentationProfile(source), indentationProfile(output)) {
		issues = append(issues, issue("yaml_indentation_changed", "YAML line count or indentation changed"))
	}
	expectedTokens := yamlControlTokens(source)
	actualTokens := yamlControlTokens(output)
	if !equalCounts(expectedTokens, actualTokens) {
		issues = append(issues, issue("yaml_tokens_changed", "YAML anchors, aliases, or tags changed"))
	}
	return issues
}

func yamlShape(node *yaml.Node, preserveScalar bool) string {
	if node == nil {
		return "nil"
	}
	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) == 0 {
			return "document"
		}
		return "document:" + yamlShape(node.Content[0], true)
	case yaml.MappingNode:
		var builder strings.Builder
		builder.WriteString("mapping{")
		for i := 0; i+1 < len(node.Content); i += 2 {
			builder.WriteString(yamlShape(node.Content[i], true))
			builder.WriteByte(':')
			builder.WriteString(yamlShape(node.Content[i+1], false))
			builder.WriteByte(';')
		}
		builder.WriteByte('}')
		return builder.String()
	case yaml.SequenceNode:
		var builder strings.Builder
		builder.WriteString("sequence[")
		for _, child := range node.Content {
			builder.WriteString(yamlShape(child, false))
			builder.WriteByte(';')
		}
		builder.WriteByte(']')
		return builder.String()
	case yaml.AliasNode:
		return "alias:" + node.Value
	case yaml.ScalarNode:
		if preserveScalar || (node.Tag != "!!str" && strings.HasPrefix(node.Tag, "!!")) {
			return "scalar:" + node.Tag + ":" + node.Value
		}
		return "scalar:" + node.Tag
	default:
		return fmt.Sprintf("kind:%d", node.Kind)
	}
}

func indentationProfile(text string) []int {
	lines := strings.Split(text, "\n")
	profile := make([]int, len(lines))
	for i, line := range lines {
		profile[i] = len(line) - len(strings.TrimLeft(line, " "))
	}
	return profile
}

func equalInts(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func yamlControlTokens(text string) map[string]int {
	counts := make(map[string]int)
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, "[],{}")
		if len(field) > 1 && (field[0] == '&' || field[0] == '*' || field[0] == '!') {
			counts[field]++
		}
	}
	return counts
}

func addMarkupChecks(issues []ValidationIssue, source, output string, translatableAttributes []string) []ValidationIssue {
	if strings.HasPrefix(strings.TrimSpace(source), "<?xml") {
		if err := validateXML(output); err != nil {
			issues = append(issues, issue("xml_invalid", "output is not valid XML: "+err.Error()))
		}
	}
	allowed := make(map[string]struct{}, len(translatableAttributes))
	for _, attribute := range translatableAttributes {
		allowed[strings.ToLower(attribute)] = struct{}{}
	}
	sourceEvents, sourceErrors := htmlStructure(source, allowed)
	outputEvents, outputErrors := htmlStructure(output, allowed)
	if len(sourceErrors) > 0 {
		issues = append(issues, issue("source_markup_invalid", strings.Join(sourceErrors, "; ")))
		return issues
	}
	if len(outputErrors) > 0 {
		issues = append(issues, issue("markup_invalid", strings.Join(outputErrors, "; ")))
	}
	if !equalStrings(sourceEvents, outputEvents) {
		issues = append(issues, issue("markup_structure_changed", "tag order, nesting, or preserved attributes changed"))
	}
	issues = comparePattern(issues, "entities_changed", "HTML/XML entities", entityPattern, source, output)
	return issues
}

func validateXML(text string) error {
	decoder := xml.NewDecoder(strings.NewReader(text))
	for {
		_, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func htmlStructure(text string, translatableAttributes map[string]struct{}) ([]string, []string) {
	tokenizer := html.NewTokenizer(bytes.NewBufferString(text))
	var events []string
	var stack []string
	var errors []string
	for {
		tokenType := tokenizer.Next()
		switch tokenType {
		case html.ErrorToken:
			if err := tokenizer.Err(); err != io.EOF {
				errors = append(errors, err.Error())
			}
			if len(stack) > 0 {
				errors = append(errors, fmt.Sprintf("unclosed tags: %v", stack))
			}
			return events, errors
		case html.StartTagToken, html.SelfClosingTagToken:
			token := tokenizer.Token()
			tag := strings.ToLower(token.Data)
			attributes := make([]string, 0, len(token.Attr))
			for _, attribute := range token.Attr {
				key := strings.ToLower(attribute.Key)
				value := attribute.Val
				if _, ok := translatableAttributes[key]; ok {
					value = "<translated>"
				}
				attributes = append(attributes, key+"="+strconv.Quote(value))
			}
			sort.Strings(attributes)
			kind := "start"
			if tokenType == html.SelfClosingTagToken {
				kind = "empty"
			}
			events = append(events, kind+":"+tag+":"+strings.Join(attributes, ","))
			if tokenType == html.StartTagToken && !voidHTMLTag(tag) {
				stack = append(stack, tag)
			}
		case html.EndTagToken:
			tag := strings.ToLower(tokenizer.Token().Data)
			events = append(events, "end:"+tag)
			if len(stack) == 0 || stack[len(stack)-1] != tag {
				errors = append(errors, "unexpected closing tag </"+tag+">")
				continue
			}
			stack = stack[:len(stack)-1]
		case html.CommentToken:
			events = append(events, "comment:"+tokenizer.Token().Data)
		case html.DoctypeToken:
			events = append(events, "doctype:"+tokenizer.Token().Data)
		}
	}
}

func voidHTMLTag(tag string) bool {
	switch tag {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}
