# REST API and asynchronous jobs

`cmd/transliter-server` exposes an asynchronous translation API backed by the
same model packages and OpenAI-compatible inference client as the Go library.

The server does not start a model runtime. Ollama, LM Studio, llama-server,
vLLM, or another compatible endpoint must already be running.

## Lifecycle

1. Authenticate the inbound API key.
2. Resolve the key to a stable owner ID.
3. Validate the model, language pair, prompt kind, and generation profile.
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
| `POST` | `/v1/jobs` | required | Validate and enqueue a translation |
| `GET` | `/v1/jobs/{id}` | required | Read one job owned by the API key |
| `GET` | `/v1/jobs?limit=20&before=<time>` | required | List previous jobs for the API key |
| `GET` | `/healthz` | public | Process health |

Use either `Authorization: Bearer <key>` or `X-API-Key: <key>`. Authorization
headers take precedence.

Cross-owner reads return `404`, rather than revealing that another owner's job
exists.

## Create a job

```http
POST /v1/jobs HTTP/1.1
Authorization: Bearer client-secret
Content-Type: application/json

{
  "model": "hymt2-30b-a3b",
  "provider_model": "hy-mt2-local",
  "profile": "deterministic",
  "translation": {
    "source": "The service is ready.",
    "source_language": "English",
    "target_language": "Korean",
    "kind": "text"
  }
}
```

`provider_model` is optional. When omitted, `TRANSLITER_API_MODEL` supplies the
model name understood by the inference server.

Successful submission returns `202 Accepted` and a `Location` header:

```json
{
  "id": "52d14df7b8b5a7d231b2295790994332",
  "status": "queued",
  "request": {
    "model": "hymt2-30b-a3b",
    "provider_model": "hy-mt2-local",
    "profile": "deterministic",
    "translation": {
      "source": "The service is ready.",
      "source_language": "English",
      "target_language": "Korean",
      "kind": "text"
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

`TRANSLITER_SERVER_API_KEYS` must be a JSON object:

```text
{"alice":"client-secret-a","team-b":"client-secret-b"}
```

Both values are environment-only. There are no flags for credentials.

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

Examples:

```text
--queue-backend memory         --store-backend memory
--queue-backend redis          --store-backend redis
--queue-backend postgres       --store-backend postgres
--queue-backend nats           --store-backend mysql
--queue-backend nats-embedded  --store-backend postgres
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
| `TRANSLITER_QUEUE_BACKEND` | `memory` |
| `TRANSLITER_STORE_BACKEND` | `memory` |
| `TRANSLITER_WORKERS` | `1` |
| `TRANSLITER_JOB_TIMEOUT` | `10m` |
| `TRANSLITER_JOB_RETENTION` | `720h` |
| `TRANSLITER_REDIS_URL` | none |
| `TRANSLITER_REDIS_PREFIX` | `transliter` |
| `TRANSLITER_POSTGRES_URL` | none |
| `TRANSLITER_MYSQL_DSN` | none |
| `TRANSLITER_NATS_URL` | NATS client default |
| `TRANSLITER_NATS_STORE_DIR` | NATS temporary/default directory |
| `TRANSLITER_NATS_EMBEDDED_MEMORY` | `false` |

Backend connection strings may contain credentials and are environment-only.
Command-line flags override non-secret environment defaults.

PostgreSQL and MySQL constructors create the required tables and indexes.
Production deployments should still manage schema changes through their normal
migration workflow; exported `Schema` constants make the initial DDL explicit.

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

The default retention is 30 days. In-memory, PostgreSQL, and MySQL stores expose
explicit expiration cleanup. Redis uses key TTLs and expiring owner indexes.

The server runs expiration cleanup hourly. Increase
`TRANSLITER_JOB_RETENTION` if clients need a longer history window.

In-memory history is lost on restart regardless of retention.
