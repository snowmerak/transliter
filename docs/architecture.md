# Application architecture

This document describes how to add CLI and REST runtimes without coupling
transport or inference details to a model family.

## Package boundaries

The `lib` package owns model-independent behavior:

- the extensible `Model` interface;
- neutral message and generation-option types;
- translation request types;
- shared prompt components;
- safe source-fence generation;
- structural output validation.

Packages under `models/` own model-specific behavior:

- model IDs, repositories, and parameter metadata;
- supported languages and prompt kinds;
- plain or structured user-message content;
- official and experimental generation settings.

Runtime code will own:

- CLI flags and stdout/stderr policy;
- HTTP routing, status codes, and response DTOs;
- inference server URLs and authentication;
- network timeouts and retries;
- logging, metrics, tracing, and persistence.

## Model extension interface

Every model package implements:

```go
type Model interface {
	Descriptor() Descriptor
	Capabilities() Capabilities
	SupportsLanguage(Language) bool
	BuildInput(TranslationRequest) (ModelInput, error)
	Options(OptionProfile) (GenerationOptions, error)
}
```

The interface returns neutral chat messages rather than a prompt string because
model templates are not interchangeable:

- Hy-MT2 receives one user message with string content.
- TranslateGemma receives one user message with exactly one structured content
  part containing locale codes and source text.

`models/catalog` exposes the built-in implementations for CLI and REST model
discovery. Applications can also supply third-party `Model` values.

## Planned runtime layout

```text
cmd/
├── transliter/
│   └── main.go                CLI composition root
└── transliter-server/
    └── main.go                REST server composition root
internal/
├── application/
│   ├── service.go             Translation orchestration
│   ├── models.go              Model registry and selection
│   └── policy.go              Retry and validation policy
├── inference/
│   ├── client.go              Backend-neutral client interface
│   ├── openai.go              OpenAI-compatible adapter
│   └── llamacpp.go            llama.cpp configuration
└── httpapi/
    ├── handler.go             HTTP transport mapping
    └── types.go               Request and response DTOs
api/
└── openapi.yaml               Optional versioned REST contract
```

Empty runtime directories are not committed until executable behavior exists.

## Dependency direction

```text
cmd/* -> internal/application -> lib.Model
  |              |                 ^
  |              |                 |
  |              +-----------> models/*
  |              |
  |              +-----------> internal/inference interface
  |
  +-> internal/httpapi -> internal/application

internal/inference adapters -> external model servers
```

The `lib` package never imports a concrete model, transport, or model client.
Model packages import `lib`. Application code selects concrete models through
the common interface.

## Shared translation flow

CLI and REST entry points should call the same application service:

1. Parse transport input.
2. Resolve a concrete `transliter.Model`.
3. Parse source and target languages.
4. Check model capabilities and language support.
5. Build chat input with `model.BuildInput`.
6. Select a generation profile with `model.Options`.
7. Send neutral input and options through an inference adapter.
8. Validate raw output with `transliter.ValidateTranslation`.
9. Retry or fail according to application policy.
10. Return raw translation to the caller.
11. Add a JSON envelope only at the HTTP boundary.

Language input should use `transliter.ParseLanguage`, then be checked against
the selected model:

```go
if !model.SupportsLanguage(source) || !model.SupportsLanguage(target) {
	return ErrUnsupportedLanguage
}
```

## Suggested inference interface

```go
type ModelClient interface {
	Generate(
		ctx context.Context,
		descriptor transliter.Descriptor,
		input transliter.ModelInput,
		options transliter.GenerationOptions,
	) (string, error)
}

type Translator interface {
	Translate(ctx context.Context, request TranslateRequest) (Translation, error)
}
```

An adapter maps neutral fields into llama.cpp, vLLM, SGLang, or another
OpenAI-compatible API. Backend configuration does not belong in
`TranslationRequest`.

## CLI guidance

A first CLI can support:

- model selection from `models/catalog`;
- model and language listing;
- source and target language flags;
- prompt kind selection;
- stdin or file input;
- stdout or file output;
- glossary and style options where the model declares support;
- official or deterministic option profiles;
- validation-only mode;
- non-zero exit codes for inference and contract failures.

Raw translation should be the default stdout format. Diagnostics belong on
stderr so shell pipelines remain reliable.

## REST guidance

A first REST API can expose:

- `POST /v1/translations`;
- `GET /v1/models`;
- `GET /v1/languages?model=<id>`;
- `GET /healthz`;
- optional `GET /readyz`.

The model returns raw translation. The HTTP handler can wrap it afterward:

```json
{
  "translation": "서비스가 준비되었습니다."
}
```

Request size limits, timeouts, cancellation, authentication, concurrency
limits, and rate limiting belong in the server.

## Backend adapters

Start with an OpenAI-compatible adapter if the selected server exposes
`/v1/chat/completions`. Keep these differences inside the adapter:

- model identifier and base URL;
- authentication headers;
- string versus structured message content;
- `top_k` disable semantics;
- `max_tokens` versus `max_new_tokens`;
- sampling and EOS behavior;
- context and output limits;
- backend-specific errors.

## Adding another model

An integration package must:

1. implement every `transliter.Model` method;
2. return stable descriptor metadata;
3. declare capabilities and language support;
4. produce the exact content required by its official template;
5. separate option profiles by provenance;
6. add contract tests;
7. optionally register itself in `models/catalog`.

No CLI, REST handler, or inference adapter change should be needed unless the
model introduces a genuinely new transport capability.
