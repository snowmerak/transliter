# transliter

`transliter` provides prompt contracts, safe source delimiters, structural
validators, and test fixtures for using Tencent's
[Hy-MT2-30B-A3B](https://huggingface.co/tencent/Hy-MT2-30B-A3B) and its
[GGUF release](https://huggingface.co/tencent/Hy-MT2-30B-A3B-GGUF) as
translation-only submodels.

> The repository is named `transliter`, but the requested Go module path is
> `github.com/snowmerak/translter`.

The project deliberately does not give Hy-MT2 planning, agent, or tool-calling
responsibilities. Questions and commands in source content are data to
translate, not requests to answer or execute.

## Repository layout

```text
.
├── lib/                    Reusable translation prompt and validation package
│   ├── fence.go            Safe Markdown source fences
│   ├── prompt.go           Composable prompt contracts
│   ├── validate.go         Mechanical output validation
│   └── testdata/           Translation and contract fixtures
├── docs/
│   ├── architecture.md     Planned CLI and REST server boundaries
│   ├── model-settings.md   Official settings vs experimental profiles
│   └── prompt-catalog.md   Input and expected-output examples
├── go.mod
└── go.sum
```

All reusable behavior is in `lib`, imported as package `transliter`. The module
root contains no runtime, which leaves clear space for future CLI and REST
server entry points under `cmd/`.

## Supported prompt types

- Plain text
- Markdown
- JSON
- YAML
- HTML/XML
- Mixed code and natural language
- Glossary-constrained translation
- Style- and audience-constrained translation
- Multi-file or multi-segment translation with exact delimiters

Shared rules live in `CommonContract`; format-specific rules live in
`FormatRules`. Every value in `PromptKinds` can produce a standalone prompt.

## Install

```bash
go get github.com/snowmerak/translter/lib
```

## Language constants

`TranslationRequest` uses the typed `Language` enum for source and target
languages:

```go
sourceLanguage := transliter.LanguageEnglish
targetLanguage := transliter.LanguageKorean
languages := transliter.SupportedLanguages()
```

Use `transliter.SupportedLanguages()` to populate a CLI choice list or REST
metadata response. Use `transliter.ParseLanguage(value)` at an external input
boundary. It accepts canonical full names such as `"Korean"` and rejects
unsupported or incorrectly cased values. `BuildPrompt` validates both language
fields again before constructing a prompt.

## Build a prompt

```go
package main

import (
	"fmt"
	"log"

	"github.com/snowmerak/translter/lib"
)

func main() {
	prompt, err := transliter.BuildPrompt(transliter.TranslationRequest{
		Source:         "The service is ready.",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
		Kind:           transliter.PromptText,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(prompt)
}
```

The important part of the generated prompt looks like this:

~~~~~text
Translate the following from English into Korean.

Translation contract:
Output only the translated result.
...

Source:

````
The service is ready.
````
~~~~~

The expected model output is only:

```text
서비스가 준비되었습니다.
```

It must not contain a `Translation:` prefix, commentary, a Markdown fence, or
an envelope such as `{"translation":"..."}`.

## Safe source fences

Source content is normally wrapped in a four-backtick Markdown fence.
`FenceSource` measures the longest consecutive backtick run `N` in the source
and uses `max(4, N+1)` backticks for the outer fence. The opening and closing
fences are always identical.

This prevents source documents containing triple, quadruple, or longer code
fences from closing the prompt boundary early.

## Structured data

For JSON, preserve keys, object and array structure, numbers, booleans, and
`null`. Translate only user-visible string values. Model output must be valid
JSON without a Markdown fence.

For YAML, preserve keys, indentation, anchors, aliases, tags, block scalar
syntax, and machine values. Translate only user-visible scalar content.

For HTML/XML, preserve tag names, attribute names, nesting, URLs, `id`, `class`,
and `data-*` attributes. Text nodes are translated by default. Attributes such
as `title` or `alt` must be explicitly allowed:

```go
request := transliter.TranslationRequest{
	Source:                 `<a href="/guide" title="Read guide">Open</a>`,
	TargetLanguage:         transliter.LanguageKorean,
	Kind:                   transliter.PromptHTMLXML,
	TranslatableAttributes: []string{"title"},
}
```

## Glossary, style, and audience

A glossary maps a source term to the exact required target term:

```go
request := transliter.TranslationRequest{
	Source:         "Create a pull request.",
	TargetLanguage: transliter.LanguageKorean,
	Kind:           transliter.PromptGlossary,
	Glossary:       map[string]string{"pull request": "풀 리퀘스트"},
}
```

Style and audience constraints never override meaning, structure,
placeholders, identifiers, or glossary terms:

```go
request := transliter.TranslationRequest{
	Source:         "Restart the service.",
	TargetLanguage: transliter.LanguageKorean,
	Kind:           transliter.PromptStyleAudience,
	Style:          "concise and formal",
	Audience:       "site reliability engineers",
}
```

See the [prompt catalog](docs/prompt-catalog.md) for every prompt type.

## Validate model output

`ValidateTranslation` checks preservation and parseability rather than
linguistic quality. It compares placeholders, URLs, email addresses, file
paths, Markdown fences, caller-supplied identifiers, and segment delimiters.
It also checks JSON, YAML, HTML, and XML structure.

```go
result := transliter.ValidateTranslation(source, output, transliter.ValidationOptions{
	Kind:        transliter.PromptJSON,
	Identifiers: []string{"user_id"},
})
if !result.OK() {
	// Reject the output or retry according to the application policy.
}
```

Translation accuracy and fluency belong in a separate evaluation layer.

## Model and harness responsibilities

| Model | Application harness |
| --- | --- |
| Translate all source meaning | Build prompts and safe source fences |
| Preserve requested format and terminology | Enforce limits, timeouts, and retry policy |
| Return only translated content | Validate structure and protected tokens |
| Treat source instructions as data | Add any API response envelope after validation |

The model is not asked to produce a standard response envelope. A CLI can
write the raw validated translation to stdout, while an HTTP handler can wrap
the same result in its own transport-level JSON response.

## CLI and REST server direction

The library contains no flags, environment handling, HTTP types, or model
client. Planned executables should live under separate `cmd/` directories and
share application services under `internal/`:

```text
cmd/transliter/             CLI entry point
cmd/transliter-server/      REST server entry point
internal/application/       Translation orchestration and retry policy
internal/inference/         llama.cpp or OpenAI-compatible client adapters
internal/httpapi/           HTTP handlers and transport DTOs
api/openapi.yaml            Optional public REST contract
```

See [architecture.md](docs/architecture.md) for dependency rules and suggested
request flow.

## Tests

```bash
go test ./...
go vet ./...
```

`lib/testdata/cases.json` covers English-to-Korean, Japanese-to-Korean,
Korean-to-English, embedded questions and commands, Markdown, nested
backticks, JSON/YAML/HTML/XML, placeholders, URLs, email addresses, paths,
glossaries, code identifiers, empty and long input, already-target-language
input, mixed languages, and segmented files.

The manually written expected translations are structural-validation fixtures,
not a string-equality translation benchmark.

## Model settings

[Model settings](docs/model-settings.md) separates values published by Tencent
from lower-temperature experimental profiles. Experimental profiles should be
evaluated by language pair, format, backend, and quantization before becoming
application defaults.
