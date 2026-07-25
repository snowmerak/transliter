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

The executable composition root owns:

- CLI flags and stdout/stderr policy;
- backend selection and lifecycle;
- inference server URLs and non-secret configuration;
- network and job timeouts;
- logging, metrics, and tracing.

`lib/inference` owns transport-neutral request and response contracts.
`lib/inference/openai` owns the reusable OpenAI-compatible HTTP client and reads
its API key directly from the environment.

`lib/jobs` owns asynchronous job, queue, store, authentication, processor, and
scheduler contracts. Backend packages depend on those contracts independently.
`lib/restapi` owns only authenticated HTTP mapping and does not select a
database or queue.

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

## Runtime layout

```text
cmd/
└── transliter-server/         REST and scheduler composition root
lib/
├── inference/                 Model-server request/response contracts
├── jobs/
│   ├── memory/                Queue and store
│   ├── redis/                 Queue and store
│   ├── postgres/              Queue and store
│   ├── natsjs/                External or embedded queue
│   └── mysql/                 Store
└── restapi/                   Authenticated asynchronous HTTP API
```

## Dependency direction

```text
cmd/transliter-server
  ├──> lib/restapi ──> lib/jobs.Queue + lib/jobs.Store
  ├──> lib/jobs.Scheduler ──> lib/jobs.Processor
  ├──> lib/jobs backends
  ├──> models/catalog ──> lib.Model
  └──> lib/inference/openai ──> user-managed model server
```

The `lib` package never imports a concrete model, transport, or model client.
Model packages import `lib`. Application code selects concrete models through
the common interface.

## Shared asynchronous translation flow

1. Authenticate an inbound API key to an owner ID.
2. Validate the model request.
3. Store a queued job and enqueue only its ID.
4. Return `202 Accepted`.
5. Receive the job ID in a scheduler worker.
6. Load the job and mark it running.
7. Build model input and generation options.
8. Call the OpenAI-compatible model server.
9. Persist success or failure.
10. Acknowledge the queue delivery.
11. Serve the job or owner-scoped history until expiration.

Language input should use `transliter.ParseLanguage`, then be checked against
the selected model:

```go
if !model.SupportsLanguage(source) || !model.SupportsLanguage(target) {
	return ErrUnsupportedLanguage
}
```

## Inference interfaces

```go
type Request interface {
	ModelName() string
	ModelInput() transliter.ModelInput
	GenerationOptions() transliter.GenerationOptions
}

type Response interface {
	OutputText() string
	ProviderModel() string
	FinishReason() string
	TokenUsage() Usage
}

type Client interface {
	Generate(context.Context, Request) (Response, error)
}
```

The request and response sides are deliberately independent. The OpenAI client
also exposes separate `RequestEncoder` and `ResponseDecoder` interfaces so a
deployment can adapt one side without replacing transport or model packages.

The library calls only a user-managed OpenAI-compatible API. It does not launch
or supervise llama.cpp or any other model process. Backend configuration does
not belong in `TranslationRequest`.

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
- OpenAI-compatible API base URL, server model, and timeout;
- non-zero exit codes for inference and contract failures.

Raw translation should be the default stdout format. Diagnostics belong on
stderr so shell pipelines remain reliable.

The API key must be read from `TRANSLITER_API_KEY`; it must not be accepted as a
CLI flag.

## REST API

The asynchronous REST API exposes:

- `POST /v1/jobs`;
- `GET /v1/jobs/{id}`;
- `GET /v1/jobs`;
- `GET /healthz`.

The job result wraps raw model output only after inference:

```json
{
  "status": "succeeded",
  "result": {
    "translation": "서비스가 준비되었습니다."
  }
}
```

Authentication, request size limits, owner isolation, and transport DTOs live
in `lib/restapi`. Queueing, persistence, concurrency, and expiration remain
behind `lib/jobs` contracts.

## OpenAI-compatible transport

`lib/inference/openai` calls `POST {base_url}/chat/completions`. The same path
can target Ollama, LM Studio, llama-server, vLLM, or another compatible server
operated by the user. Keep these differences inside request or response codecs:

- model identifier and base URL;
- authentication headers;
- string versus structured message content;
- `top_k` disable semantics;
- `max_tokens` versus `max_new_tokens`;
- sampling and EOS behavior;
- context and output limits;
- backend-specific errors.

There is no llama.cpp-specific client or process manager. If a server accepts a
different request extension or response shape, supply another
`RequestEncoder` or `ResponseDecoder`.

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
