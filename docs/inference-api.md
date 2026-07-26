# OpenAI-compatible inference

`transliter` uses one remote inference path: non-streaming OpenAI-compatible
chat completions.

The user is responsible for running and configuring the server. The library
does not start a local executable, manage model files, select GPU layers, or
control the lifetime of Ollama, LM Studio, `llama-server`, vLLM, or another
service.

## Packages

- `lib/inference` defines transport-neutral request, response, usage, and client
  interfaces.
- `lib/inference/openai` implements an HTTP client for
  `POST {base_url}/chat/completions`.
- `lib` and `models/*` remain independent of HTTP and authentication.

The separation is intentional:

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

`inference.NewRequest` and `inference.NewResponse` provide immutable default
implementations. Applications may supply other implementations without
changing the OpenAI-compatible client.

## Request and response codecs

HTTP request generation and HTTP response interpretation are separate
extension points:

```go
type RequestEncoder interface {
	EncodeRequest(inference.Request, string) (io.Reader, error)
}

type ResponseDecoder interface {
	DecodeResponse(io.Reader) (inference.Response, error)
	DecodeError(int, io.Reader) error
}
```

The default `JSONRequestEncoder` emits a non-streaming
`/v1/chat/completions`-style body. `JSONResponseDecoder` reads the first choice,
finish reason, provider model name, and token usage.

This boundary matters because “OpenAI-compatible” implementations vary:

- some reject fields outside the core OpenAI schema;
- some use different names for repetition penalties;
- some accept model-specific structured message parts while others validate a
  fixed content schema;
- some return additional usage or timing fields.

Replace only the encoder or decoder needed by a deployment. Do not add
provider-specific rules to a model package.

## Environment

| Variable | Required | Default |
| --- | --- | --- |
| `TRANSLITER_API_BASE_URL` | no | `http://127.0.0.1:8080/v1` |
| `TRANSLITER_API_MODEL` | if the request has no model | none |
| `TRANSLITER_API_TIMEOUT` | no | `2m` |
| `TRANSLITER_API_KEY` | only when required by the server | none |

`TRANSLITER_API_TIMEOUT` uses Go duration syntax such as `30s`, `2m`, or
`1m30s`.

The model name is the name expected by the server. It is not required to equal
the stable catalog id (`model_catalog` / `transliter.ModelID`). For example, an
oMLX or LM Studio identifier can be selected independently from the prompt adapter.

## Configuration precedence

The library reads environment defaults with `openai.ConfigFromEnv`. A CLI can
then call `openai.RegisterFlags(flagSet, &config)` before parsing arguments. A
CLI or REST composition root should apply:

1. explicit non-secret CLI flag;
2. corresponding environment variable;
3. library default.

The registered flags are:

```text
--api-base-url
--api-model
--api-timeout
```

There must be no `--api-key` flag. The API key is read directly from
`TRANSLITER_API_KEY` when `openai.New` creates the client. It is not exposed by
`openai.Config`, error messages, or request/response values.

## Parameter mapping

The default JSON encoder maps:

- `Temperature` to `temperature`;
- `TopP` to `top_p`;
- `TopK` to `top_k`;
- `RepetitionPenalty` to `repetition_penalty`;
- `DoSample` to `do_sample`;
- `MaxOutputTokens` to `max_tokens`;
- `Stop` to `stop`.

The first, second, sixth, and seventh fields are common OpenAI chat-completions
parameters. The other fields are local-server extensions. If a server uses
`repeat_penalty`, ignores `do_sample`, or rejects extension fields, provide a
server-specific `RequestEncoder` in `openai.Config`.

Option provenance remains attached to `GenerationOptions` inside the
application, but it is not sent to the inference server.

## TranslateGemma compatibility

TranslateGemma uses a structured content part containing
`source_lang_code`, `target_lang_code`, and `text`. The default encoder
preserves that JSON exactly.

Compatibility still depends on the selected server and its chat-template
implementation. A server that rejects custom content fields cannot run the
official TranslateGemma request through its standard chat-completions route.
The library reports that server error; it does not silently convert the request
to an unofficial prompt.

## Error policy

The client:

- uses the caller context for cancellation;
- applies a default two-minute HTTP timeout;
- limits response bodies to 4 MiB;
- rejects malformed or trailing JSON;
- rejects successful responses without a choice;
- exposes non-2xx responses as `openai.APIError`;
- never retries automatically;
- never includes the API key in an error.

Retry and output-validation policy belong in the application layer because
translation requests may not be safe to repeat blindly.
