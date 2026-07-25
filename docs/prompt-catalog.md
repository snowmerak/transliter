# Prompt catalog

Every example can be constructed independently with `TranslationRequest` and
`BuildPrompt`. Expected outputs below show the raw model response: no envelope,
label, or explanation.

## Plain text

Source:

```text
The service is ready.
```

Expected output:

```text
서비스가 준비되었습니다.
```

Use `Kind: PromptText`. A question or command in the source must be translated,
not answered or executed.

## Markdown

Source:

```markdown
# Setup

- Read the [guide](https://example.com).
- Run `install_app`.
```

Expected output:

```markdown
# 설정

- [가이드](https://example.com)를 읽으세요.
- `install_app`을 실행하세요.
```

Use `Kind: PromptMarkdown`. Preserve link destinations, inline code, fenced
code, fence lengths, list markers, tables, and document order.

## JSON

Source:

```json
{"title":"Welcome, {{username}}","retry":3,"enabled":true}
```

Expected output:

```json
{"title":"환영합니다, {{username}}","retry":3,"enabled":true}
```

Use `Kind: PromptJSON`. Keys and non-string machine values must remain
unchanged. The model output must not include a Markdown fence.

## YAML

Source:

```yaml
defaults: &defaults
  retries: 3
  message: Welcome
production:
  <<: *defaults
```

Expected output:

```yaml
defaults: &defaults
  retries: 3
  message: 환영합니다
production:
  <<: *defaults
```

Use `Kind: PromptYAML`. Preserve keys, indentation, anchors, aliases, tags,
block-scalar syntax, and machine values.

## HTML/XML

Source:

```html
<a id="guide" href="/docs" title="Read guide">Open</a>
```

With `TranslatableAttributes: []string{"title"}`, expected output is:

```html
<a id="guide" href="/docs" title="가이드 읽기">열기</a>
```

Without the explicit attribute allowlist, the `title` value must also remain
unchanged. Input with an XML declaration is validated as well-formed XML.

## Mixed code and natural language

Source:

````markdown
Call `loadUser(user_id)` to load the user.

```ts
const user = loadUser(user_id);
```
````

Expected output:

````markdown
사용자를 불러오려면 `loadUser(user_id)`를 호출하세요.

```ts
const user = loadUser(user_id);
```
````

Use `Kind: PromptMixedCode` and pass protected identifiers to
`ValidationOptions.Identifiers`.

## Glossary

```go
request := transliter.TranslationRequest{
	Source:         "Create a pull request in the repository.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptGlossary,
	Glossary: map[string]string{
		"pull request": "풀 리퀘스트",
		"repository":   "저장소",
	},
}
```

Expected output:

```text
저장소에 풀 리퀘스트를 생성하세요.
```

The target glossary terms are mandatory, not stylistic suggestions.

## Style and audience

```go
request := transliter.TranslationRequest{
	Source:         "Restart the service now.",
	TargetLanguage: "Korean",
	Kind:           transliter.PromptStyleAudience,
	Style:          "formal operations manual",
	Audience:       "site reliability engineers",
}
```

Expected output shape:

```text
서비스를 즉시 재시작하십시오.
```

Style cannot override meaning, structure, placeholders, identifiers, or
glossary terms.

## Multiple files or segments

Source:

```text
<<<FILE:README.md>>>
Welcome.
<<<SECTION:2>>>
Install now.
<<<FILE:guide.md>>>
Read the guide.
```

Expected output:

```text
<<<FILE:README.md>>>
환영합니다.
<<<SECTION:2>>>
지금 설치하세요.
<<<FILE:guide.md>>>
가이드를 읽으세요.
```

Use `Kind: PromptSegmented` and provide the exact `Delimiters` list.
Validation checks delimiter count and order.
