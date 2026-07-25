# Application architecture

This document describes how to add a CLI and REST API without coupling
transport or inference details to the reusable translation contract.

## Current boundary

The `lib` package owns deterministic, side-effect-free behavior:

- prompt selection and composition;
- safe source-fence generation;
- translation request rules;
- structural validation of model output;
- reusable validation result types.

It does not own:

- command-line flags or stdout/stderr policy;
- HTTP routing, status codes, or response DTOs;
- model server URLs, authentication, or network retries;
- configuration files or environment variables;
- logging, metrics, tracing, or persistence.

Its import path is:

```text
github.com/snowmerak/translter/lib
```

The directory is named `lib`, while the declared package remains `transliter`
so calling code reads naturally.

## Planned layout

Add application layers only when their behavior is implemented:

```text
cmd/
├── transliter/
│   └── main.go                CLI composition root
└── transliter-server/
    └── main.go                REST server composition root
internal/
├── application/
│   ├── service.go             Translation use case
│   └── policy.go              Retry and validation policy
├── inference/
│   ├── client.go              Backend-neutral interface
│   ├── openai.go              OpenAI-compatible HTTP adapter
│   └── llamacpp.go            llama.cpp-specific configuration
└── httpapi/
    ├── handler.go             HTTP transport mapping
    └── types.go               Request and response DTOs
api/
└── openapi.yaml               Optional versioned REST contract
```

Empty placeholder directories are intentionally not committed. Add a layer
when there is executable behavior and a test for it.

## Dependency direction

Dependencies should point inward:

```text
cmd/* -> internal/application -> lib
  |              |
  |              +-----------> internal/inference interface
  |
  +-> internal/httpapi -> internal/application

internal/inference adapters -> external model servers
```

The `lib` package must not import `cmd`, `internal`, HTTP frameworks, or model
clients. Application code may import `lib`.

## Shared translation flow

Both CLI and REST entry points should call the same application service:

1. Parse and validate transport input.
2. Convert it into an application request.
3. Build the model prompt with `transliter.BuildPrompt`.
4. Send one user message through an inference client.
5. Validate raw model output with `transliter.ValidateTranslation`.
6. Retry or fail according to application policy.
7. Return the raw translation to the caller.
8. Add a JSON envelope only at the HTTP transport boundary.

This keeps CLI output pipe-friendly while allowing the server to return
structured metadata.

CLI flags and REST string fields should be converted with
`transliter.ParseLanguage`. Choice lists or metadata endpoints can use
`transliter.SupportedLanguages`.

## Suggested application interface

The application layer can define its own model client without exposing an HTTP
SDK to the library:

```go
type ModelClient interface {
	Generate(ctx context.Context, prompt string, options GenerateOptions) (string, error)
}

type Translator interface {
	Translate(ctx context.Context, request TranslateRequest) (Translation, error)
}
```

`TranslateRequest` should contain prompt concerns such as source, target
language, prompt kind, glossary, style, audience, protected identifiers, and
delimiters. Backend configuration belongs in `GenerateOptions` or adapter
configuration, not in `lib.TranslationRequest`.

## CLI guidance

A first CLI can support:

- source and target language flags;
- prompt kind selection;
- stdin or file input;
- stdout or file output;
- glossary and style options;
- a validation-only mode;
- non-zero exit codes for inference and contract failures.

Raw translation should be the default stdout format. Diagnostics belong on
stderr so shell pipelines remain reliable.

## REST guidance

A first REST API can expose:

- `POST /v1/translations`;
- `GET /healthz`;
- optional `GET /readyz` when backend readiness differs from process health.

The HTTP request may be JSON, but this is a transport contract. The model
should still return a raw translation. The handler can wrap the validated
output afterward:

```json
{
  "translation": "서비스가 준비되었습니다."
}
```

Request size limits, timeouts, cancellation, authentication, concurrency
limits, and rate limiting belong in the server layer.

## Backend adapters

Start with an OpenAI-compatible HTTP adapter if the selected llama.cpp, vLLM,
or SGLang server exposes `/v1/chat/completions`. Keep backend differences in
the adapter:

- model identifier and base URL;
- authentication headers;
- chat template assumptions;
- `top_k` disable semantics;
- EOS and stop behavior;
- context and output token limits;
- backend-specific error mapping.

Do not add these fields to the prompt-building API unless they change the
translation contract itself.
