# transliter

`transliter` provides model-specific translation contracts behind one
extensible Go interface. It currently supports:

- Tencent Hy-MT2 1.8B
- Tencent Hy-MT2 7B
- Tencent Hy-MT2 30B-A3B
- Google TranslateGemma 4B IT
- Google TranslateGemma 12B IT
- Google TranslateGemma 27B IT

The project builds model inputs, separates official and experimental generation
options, validates structural output contracts, and can call a user-managed
OpenAI-compatible inference endpoint.

> The repository is named `transliter`, but the requested Go module path is
> `github.com/snowmerak/translter`.

## Design

The common library contains model-independent contracts:

```go
type Model interface {
	Descriptor() Descriptor
	Capabilities() Capabilities
	SupportsLanguage(Language) bool
	BuildInput(TranslationRequest) (ModelInput, error)
	Options(OptionProfile) (GenerationOptions, error)
}
```

Each model size is a separate package that implements this interface. A new
model integration can be added without changing CLI, REST, or inference
adapters.

## Repository layout

```text
.
├── lib/                              Shared interfaces, prompts, fences, validation
│   └── inference/
│       └── openai/                   OpenAI-compatible HTTP client
├── models/
│   ├── catalog/                      Built-in model discovery
│   ├── hymt2/
│   │   ├── v1p8b/
│   │   ├── v7b/
│   │   └── v30ba3b/
│   └── translategemma/
│       ├── v4b/
│       ├── v12b/
│       └── v27b/
└── docs/
    ├── architecture.md
    ├── model-settings.md
    └── prompt-catalog.md
```

Shared family implementation details live under `models/internal`; callers
import only `lib`, a concrete model package, or `models/catalog`.

## Install

Install only the model packages an application needs:

```bash
go get github.com/snowmerak/translter/lib
go get github.com/snowmerak/translter/models/hymt2/v30ba3b
```

## Build model input through the common interface

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"

	transliter "github.com/snowmerak/translter/lib"
	hymt2 "github.com/snowmerak/translter/models/hymt2/v30ba3b"
)

func main() {
	var model transliter.Model = hymt2.New()

	input, err := model.BuildInput(transliter.TranslationRequest{
		Source:         "The service is ready.",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
		Kind:           transliter.PromptText,
	})
	if err != nil {
		log.Fatal(err)
	}

	options, err := model.Options(transliter.ProfileOfficial)
	if err != nil {
		log.Fatal(err)
	}

	payload, _ := json.Marshal(input)
	fmt.Println(string(payload))
	fmt.Printf("%+v\n", options)
}
```

For Hy-MT2, the user message content is a plain string containing the complete
translation contract and a safe source fence.

For TranslateGemma, the same interface returns its official structured user
content:

```json
{
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "The service is ready.",
          "source_lang_code": "en",
          "target_lang_code": "ko"
        }
      ]
    }
  ]
}
```

The bundled OpenAI-compatible client serializes this `ModelInput` directly.

## Call an OpenAI-compatible server

`transliter` connects to an inference server that the user operates. It does
not download models or start, supervise, or stop Ollama, LM Studio,
`llama-server`, vLLM, or another model process.

Configure the endpoint with environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `TRANSLITER_API_BASE_URL` | `http://127.0.0.1:8080/v1` | OpenAI-compatible API base URL |
| `TRANSLITER_API_MODEL` | none | Server-side model name or alias |
| `TRANSLITER_API_TIMEOUT` | `2m` | Go duration for one HTTP request |
| `TRANSLITER_API_KEY` | none | Bearer token; read only from the environment |

The API key is intentionally absent from `openai.Config`. A CLI can load
environment defaults, call `openai.RegisterFlags`, and parse `--api-base-url`,
`--api-model`, and `--api-timeout`. The helper deliberately does not register an
API-key flag.

```go
package main

import (
	"context"
	"fmt"
	"log"

	transliter "github.com/snowmerak/translter/lib"
	"github.com/snowmerak/translter/lib/inference"
	"github.com/snowmerak/translter/lib/inference/openai"
	hymt2 "github.com/snowmerak/translter/models/hymt2/v30ba3b"
)

func main() {
	model := hymt2.New()
	input, err := model.BuildInput(transliter.TranslationRequest{
		Source:         "The service is ready.",
		SourceLanguage: transliter.LanguageEnglish,
		TargetLanguage: transliter.LanguageKorean,
		Kind:           transliter.PromptText,
	})
	if err != nil {
		log.Fatal(err)
	}
	options, err := model.Options(transliter.ProfileOfficial)
	if err != nil {
		log.Fatal(err)
	}

	config, err := openai.ConfigFromEnv()
	if err != nil {
		log.Fatal(err)
	}
	client, err := openai.New(config)
	if err != nil {
		log.Fatal(err)
	}
	response, err := client.Generate(
		context.Background(),
		inference.NewRequest("", input, options),
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Print(response.OutputText())
}
```

An explicit model name passed to `inference.NewRequest` takes precedence over
`TRANSLITER_API_MODEL`. This is useful when one application talks to a
multi-model server.

The default request encoder includes `top_k`, `repetition_penalty`, and
`do_sample` as common local-server extensions. They are not core OpenAI
parameters. A server that rejects or renames them can provide another
`openai.RequestEncoder`; response differences are handled independently through
`openai.ResponseDecoder`.

See [OpenAI-compatible inference](docs/inference-api.md) for the contracts and
configuration policy.

## Built-in model discovery

`models/catalog` provides the six bundled implementations:

```go
models := catalog.All()
model, ok := catalog.Find("hymt2-30b-a3b")
```

Applications can also construct their own `[]transliter.Model`, including
third-party implementations.

## Model packages and capabilities

| Model ID | Package | User content | Prompt kinds |
| --- | --- | --- | --- |
| `hymt2-1.8b` | `models/hymt2/v1p8b` | Plain string | All supported kinds |
| `hymt2-7b` | `models/hymt2/v7b` | Plain string | All supported kinds |
| `hymt2-30b-a3b` | `models/hymt2/v30ba3b` | Plain string | All supported kinds |
| `translategemma-4b` | `models/translategemma/v4b` | Structured part | Plain text |
| `translategemma-12b` | `models/translategemma/v12b` | Structured part | Plain text |
| `translategemma-27b` | `models/translategemma/v27b` | Structured part | Plain text |

TranslateGemma's official chat template requires a source locale, a target
locale, and source text as exactly one structured content part. Glossary,
Markdown, style, and segmented prompts would require an unofficial manual
prompt path, so the official integration rejects them instead of silently
ignoring constraints.

## Generation options

Every model package owns its settings. `ProfileOfficial` returns values
published as either an official recommendation or an official model-card
example. Check `GenerationOptions.Provenance` before presenting a value as a
recommendation.

`ProfileDeterministic` is a project experimental profile. It is never labeled
as an official recommendation.

See [model settings](docs/model-settings.md) for exact values.

## Languages

`Language` is a typed string enum containing the union of languages supported
by built-in integrations:

```go
source := transliter.LanguageEnglish
target := transliter.LanguageKorean
language, err := transliter.ParseLanguage("Japanese")
```

`SupportedLanguages()` returns the global union. A model-specific UI must call
`model.SupportsLanguage(language)` because Hy-MT2 and TranslateGemma have
different language sets and language-code formats.

## Safe source fences

Hy-MT2 source content is wrapped in a dynamically sized Markdown fence.
`FenceSource` measures the longest consecutive backtick run `N` and uses
`max(4, N+1)` backticks for the outer fence.

TranslateGemma does not use this fence because its official template receives
source text in a dedicated structured field.

## Output validation

`ValidateTranslation` checks preservation and parseability rather than
linguistic quality. It compares placeholders, URLs, email addresses, paths,
Markdown fences, identifiers, and segment delimiters, and validates
JSON/YAML/HTML/XML structure.

```go
result := transliter.ValidateTranslation(source, output, transliter.ValidationOptions{
	Kind:        transliter.PromptJSON,
	Identifiers: []string{"user_id"},
})
if !result.OK() {
	// Reject or retry according to application policy.
}
```

The application must select validation options from the original translation
request. Model output remains raw translated content; a REST handler can add a
transport envelope after validation.

## Future CLI and REST server

The planned runtime layers remain separate:

```text
cmd/transliter/             CLI entry point
cmd/transliter-server/      REST server entry point
internal/application/       Model selection, orchestration, validation, retry
lib/inference/              Transport-neutral request and response contracts
lib/inference/openai/       OpenAI-compatible HTTP client and codecs
internal/httpapi/           HTTP handlers and DTOs
api/openapi.yaml            Optional public REST contract
```

The application layer depends only on `transliter.Model`, so selecting a
different model does not change transport or inference code.

See [application architecture](docs/architecture.md) for the complete request
flow.

## Tests

```bash
go test ./...
go vet ./...
go mod verify
```

`lib/testdata/cases.json` covers language pairs, embedded instructions,
Markdown, nested backticks, structured formats, placeholders, addresses,
glossaries, identifiers, empty and long input, mixed languages, and segmented
files.
