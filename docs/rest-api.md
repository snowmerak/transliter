# REST API and asynchronous jobs

`cmd/transliter-server` exposes an asynchronous translation API backed by the
same model packages and OpenAI-compatible inference client as the Go library.

The server does not start a model runtime. Ollama, LM Studio, llama-server,
vLLM, or another compatible endpoint must already be running.

## Lifecycle

1. Optionally authenticate the inbound API key when keys are configured.
2. Resolve the caller to a stable owner ID (`anonymous` when open).
3. Validate the model catalog, language pair, prompt kind, and generation profile.
4. Allocate a cryptographically random 128-bit job ID.
5. Store the queued job with an expiration time.
6. enqueue only the job ID.
7. Let an independent scheduler worker receive the ID.
8. Mark the job running and call the OpenAI-compatible model server.
9. Persist success or failure and acknowledge the queue delivery.
10. Serve the job and owner-scoped history until retention expires.

Raw inbound API keys are never stored in jobs. The static authenticator keeps
only SHA-256 digests in memory. Queue messages contain only job IDs, never
source text, model-server credentials, or owner credentials.

## Endpoints

| Method | Path | Authentication | Purpose |
| --- | --- | --- | --- |
| `POST` | `/v1/jobs` | optional* | Validate and enqueue a translation |
| `GET` | `/v1/jobs/{id}` | optional* | Read one job owned by the caller |
| `GET` | `/v1/jobs?limit=20&before=<time>` | optional* | List previous jobs for the caller |
| `GET` | `/v1/model-catalogs` | public | List built-in prompt adapters |
| `GET` | `/v1/model-catalogs/{id}` | public | Catalog detail, profiles, options |
| `POST` | `/v1/model-catalogs/{id}/preview` | public | Dry-run BuildInput + Options |
| `GET` | `/healthz` | public | Process health |
| `GET` | `/openapi.json` | public | OpenAPI 3.1 document as JSON |
| `GET` | `/openapi.yaml` | public | OpenAPI 3.1 document as YAML |
| `GET` | `/docs` | public | Interactive Scalar API reference |
| `GET` | `/ui/` | public | Simple job console web UI |

When `TRANSLITER_SERVER_API_KEYS` is set, use either `Authorization: Bearer <key>`
or `X-API-Key: <key>`. Authorization headers take precedence. When unset, job
endpoints are open and jobs are owned by `anonymous`.

Cross-owner reads return `404`, rather than revealing that another owner's job
exists.

## Interactive API reference

The OpenAPI YAML source is embedded in the Go binary. `/openapi.json` is
generated from that source at startup, so the JSON and YAML endpoints cannot
drift independently.

`/docs` uses Scalar's standalone browser integration and loads the served
`/openapi.json` document. The Scalar CDN dependency is pinned to an explicit
version. The documentation endpoints are public; using the interactive request
client against job endpoints still requires one of the documented API-key
security schemes.

## Model catalogs

Built-in prompt adapters are discoverable without authentication:

```http
GET /v1/model-catalogs
GET /v1/model-catalogs/{id}
POST /v1/model-catalogs/{id}/preview
```

- List/detail expose descriptor metadata, languages, profiles, generation
  options, and `capabilities.auxiliary_fields` (glossary/style/audience/...).
- Preview runs `BuildInput` + `Options` only. It does not call an inference
  server. Hy-MT2 returns plain string message content; TranslateGemma returns
  structured content parts.


## Create a job

```http
POST /v1/jobs HTTP/1.1
Authorization: Bearer client-secret
Content-Type: application/json

{
  "model_catalog": "hymt2-30b-a3b",
  "model": "hy-mt2-local",
  "profile": "deterministic",
  "translation": {
    "source": "The service is ready.",
    "source_language": "English",
    "target_language": "Korean",
    "kind": "text",
    "glossary": {}
  }
}
```

`model_catalog` selects the built-in prompt/options adapter. `model` is the
inference-server name/alias and is optional; when omitted, `TRANSLITER_API_MODEL`
supplies it.

Successful submission returns `202 Accepted` and a `Location` header:

```json
{
  "id": "52d14df7b8b5a7d231b2295790994332",
  "status": "queued",
  "request": {
    "model_catalog": "hymt2-30b-a3b",
    "model": "hy-mt2-local",
    "profile": "deterministic",
    "translation": {
      "source": "The service is ready.",
      "source_language": "English",
      "target_language": "Korean",
      "kind": "text",
      "glossary": {}
    }
  },
  "created_at": "2026-07-25T10:00:00Z",
  "updated_at": "2026-07-25T10:00:00Z",
  "expires_at": "2026-08-24T10:00:00Z"
}
```

Statuses are `queued`, `running`, `succeeded`, and `failed`. Successful jobs
include the raw translation plus provider model, finish reason, and token usage.
Failed jobs include a sanitized error string.

The server accepts one JSON object, rejects unknown fields, and limits request
bodies to 2 MiB by default.

## Client keys and model-server keys

The two credentials have different purposes:

| Variable | Used for |
| --- | --- |
| `TRANSLITER_SERVER_API_KEYS` | Authenticating callers of transliter |
| `TRANSLITER_API_KEY` | Authenticating transliter to the model server |

`TRANSLITER_SERVER_API_KEYS` is optional. When set it must be a JSON object:

```text
{"alice":"client-secret-a","team-b":"client-secret-b"}
```

When unset or empty, inbound job APIs do not require a key. Both credential
variables are environment-only. There are no flags for credentials.

## Queue and store matrix

Queue and job storage are selected independently:

| Backend | Queue | Store | Notes |
| --- | :---: | :---: | --- |
| In-memory | yes | yes | Development and single-process ephemeral use |
| Redis via rueidis | yes | yes | Streams consumer group plus TTL job records |
| PostgreSQL via pgx | yes | yes | Leased queue with `SKIP LOCKED`; JSONB jobs |
| NATS JetStream | yes | no | External durable work queue |
| Embedded NATS JetStream | yes | no | In-process memory or file-backed queue |
| MySQL | no | yes | InnoDB job records and owner history |
| SQLite via modernc.org/sqlite | no | yes | Pure-Go file or memory DSN; no CGO |

Examples:

```text
--queue-backend memory         --store-backend memory
--queue-backend redis          --store-backend redis
--queue-backend postgres       --store-backend postgres
--queue-backend nats           --store-backend mysql
--queue-backend nats-embedded  --store-backend sqlite
--queue-backend memory         --store-backend sqlite
```

The PostgreSQL queue table intentionally has no foreign key to the PostgreSQL
job table, allowing PostgreSQL queue + MySQL store or other mixed deployments.

## Server configuration

Non-secret flags:

```text
--http-address
--queue-backend
--store-backend
--workers
--job-timeout
--job-retention
--api-base-url
--api-model
--api-timeout
```

Environment variables:

| Variable | Default |
| --- | --- |
| `TRANSLITER_HTTP_ADDRESS` | `:8080` |
| `TRANSLITER_QUEUE_BACKEND` | `nats-embedded` |
| `TRANSLITER_STORE_BACKEND` | `sqlite` |
| `TRANSLITER_WORKERS` | `1` |
| `TRANSLITER_JOB_TIMEOUT` | `10m` |
| `TRANSLITER_JOB_RETENTION` | `720h` |
| `TRANSLITER_REDIS_URL` | none |
| `TRANSLITER_REDIS_PREFIX` | `transliter` |
| `TRANSLITER_POSTGRES_URL` | none |
| `TRANSLITER_MYSQL_DSN` | none |
| `TRANSLITER_SQLITE_PATH` | `transliter-jobs.db` |
| `TRANSLITER_NATS_URL` | NATS client default |
| `TRANSLITER_NATS_STORE_DIR` | NATS temporary/default directory |
| `TRANSLITER_NATS_EMBEDDED_MEMORY` | `true` |

Backend connection strings may contain credentials and are environment-only.
Command-line flags override non-secret environment defaults.

PostgreSQL, MySQL, and SQLite constructors create the required tables and
indexes. Production deployments should still manage schema changes through their
normal migration workflow; exported `Schema` constants make the initial DDL
explicit.

## Embedded JetStream

`--queue-backend nats-embedded` starts NATS JetStream inside the transliter
process without opening a NATS network listener.

- Set `TRANSLITER_NATS_EMBEDDED_MEMORY=true` for an ephemeral stream.
- Leave it false and set `TRANSLITER_NATS_STORE_DIR` for a file-backed stream.
- Embedded mode is appropriate for a self-contained single-process deployment.
- Use an external NATS cluster for multi-process workers and independent queue
  lifecycle.

Stopping the transliter process also stops its embedded NATS server.

## Delivery and failure semantics

Queue implementations are treated as at-least-once:

- PostgreSQL leases a queue row for twice the configured job timeout.
- Redis reclaims idle pending stream entries after twice the job timeout.
- JetStream uses explicit acknowledgements with an ack wait of twice the job
  timeout.
- Completed or failed jobs are acknowledged without another model call.

If a worker dies while the model server is processing a `running` job, the
delivery may be retried and the model call may execute again. Applications must
not assume exactly-once inference.

The server does not automatically retry model errors. A successfully processed
job is persisted before its queue delivery is acknowledged.

If job storage succeeds but enqueueing fails, the API marks the stored job
failed and returns `503`.

## Retention

The default retention is 30 days. In-memory, PostgreSQL, MySQL, and SQLite
stores expose explicit expiration cleanup. Redis uses key TTLs and expiring
owner indexes.

The server runs expiration cleanup hourly. Increase
`TRANSLITER_JOB_RETENTION` if clients need a longer history window.

In-memory history is lost on restart regardless of retention.
