# Sonata mini MVP technology stack

> Статус: выбранный стек первой развёртываемой версии  
> Принцип: Go modular monolith без языкового и инфраструктурного зоопарка

## 1. Сводка

```text
Language              Go, pinned supported stable release
HTTP                  net/http + chi
Streaming             SSE + http.Flusher
Concurrency           context + errgroup + semaphore
API contracts         OpenAPI 3.1 + oapi-codegen
Database              Neon PostgreSQL
PostgreSQL client     pgx/v5 + pgxpool
SQL generation        sqlc
Migrations            goose
Background jobs       River
Vector retrieval      Qdrant Cloud + official Go client
Model provider        OpenCode Zen
Provider protocol     OpenAI-compatible Chat Completions
Web search            LangSearch
UI and auth           OpenWebUI
Instructions          protected XML
Manifests             protected XML default or free-form user manifest
Emotion               deterministic Go module
Configuration         split YAML + custom typed loader
Secrets               Render Environment Group + secret files
Observability         OpenTelemetry OTLP/HTTP + Grafana Cloud Free
Logging               log/slog JSON
Object storage        Cloudflare R2 when document upload is enabled
Deployment            Render Blueprint
CI                     GitHub Actions
Sandbox                disabled in mini MVP
```

## 2. Go application

Sonata использует один Go module и один основной binary с несколькими командами:

```text
sonata api
sonata worker
sonata migrate
sonata config validate
sonata config print --redacted
sonata doctor
```

Не используются:

- внутренний gRPC;
- dependency injection framework;
- несколько backend-языков;
- отдельные microservices когнитивного ядра;
- универсальный agent framework.

Dependencies соединяются явными constructors.

## 3. HTTP

```text
net/http
+ github.com/go-chi/chi/v5
```

Причины:

- совместимость со стандартным Go HTTP stack;
- корректная работа с context cancellation;
- удобные middleware и route groups;
- отсутствие собственного несовместимого runtime;
- простой SSE streaming.

Основные endpoints:

```text
GET  /v1/models
POST /v1/chat/completions

GET  /internal/health/live
GET  /internal/health/ready
GET  /internal/metrics

GET    /api/v1/manifest
PUT    /api/v1/manifest
DELETE /api/v1/manifest
```

OpenAI-compatible streaming реализуется вручную.

Остальные API описываются OpenAPI 3.1 и при необходимости генерируются через `oapi-codegen`.

## 4. Concurrency

```text
context.Context
golang.org/x/sync/errgroup
semaphore
```

Пять ролей внутри одной фазы исполняются параллельно.

```text
raw phase
-> five parallel model calls

critical phase
-> five parallel model calls

summary phase
-> five parallel model calls
```

Request context отменяет весь pipeline при отключении клиента или timeout.

Основной интерактивный pipeline не отправляется в background queue.

## 5. PostgreSQL

```text
Neon PostgreSQL
pgx/v5
pgxpool
sqlc
goose
```

Принципы:

- SQL пишется явно;
- `sqlc` генерирует typed Go methods;
- ORM не используется;
- migrations хранятся как SQL;
- pooled connection используется приложением;
- direct connection используется migrations и maintenance tasks.

Канонические данные Sonata и данные OpenWebUI должны быть разделены отдельными databases или минимум отдельными schemas.

## 6. Background jobs

```text
River
```

River использует PostgreSQL и не требует Redis.

Jobs:

- индексация memory items в Qdrant;
- document processing;
- повторная индексация;
- conversation summaries;
- удаление orphaned projections;
- model registry synchronization;
- deferred usage aggregation;
- cleanup jobs.

LLM-ответ пользователю не выполняется через River.

## 7. Qdrant Cloud

```text
github.com/qdrant/go-client
```

Collections:

```text
sonata_memory
sonata_documents
```

Retrieval:

```text
Intfloat Multilingual E5 Small
+ BM25
-> fusion
-> optional Answer.AI ColBERT Small V1
```

ColBERT выключен по умолчанию и включается только после измеримого выигрыша.

Neon остаётся canonical source of truth. Qdrant является rebuildable projection.

## 8. OpenCode Zen

Endpoint:

```text
https://opencode.ai/zen/v1/chat/completions
```

Все выбранные модели используют OpenAI-compatible Chat Completions. Для mini MVP реализуется один transport.

### Model assignment

| Runtime role | Primary model | Fallback |
|---|---|---|
| Router | `nemotron-3-ultra-free` | `mimo-v2.5-free` |
| Raw prisms | `deepseek-v4-flash-free` | `mimo-v2.5-free` |
| Critical prisms | `deepseek-v4-flash-free` | `mimo-v2.5-free` |
| Summary | `nemotron-3-ultra-free` | `mimo-v2.5-free` |
| Synthesis tooling | `big-pickle` | `deepseek-v4-flash-free`, затем `mimo-v2.5-free` |
| Synthesis final | `big-pickle` | `deepseek-v4-flash-free`, затем `mimo-v2.5-free` |

### Reserved code model

```text
north-mini-code-free
```

Он зарезервирован для будущего code analysis, IDE и sandbox integration и не участвует в обычном pipeline mini MVP.

### Shared provider policy

- один server-side OpenCode Zen master key;
- нет индивидуальных финансовых quotas;
- есть только технические concurrency и anti-abuse limits;
- exhaustion общего key останавливает default Sonata provider для всех пользователей;
- direct user providers в OpenWebUI не получают внутренний pipeline Sonata.

## 9. Provider adapter

```go
type ModelProvider interface {
    Generate(ctx context.Context, req GenerateRequest) (GenerateResult, error)
    Stream(ctx context.Context, req GenerateRequest) (<-chan StreamEvent, error)
    ListModels(ctx context.Context) ([]ModelDescriptor, error)
}
```

Для mini MVP существует:

```text
OpenCodeZenProvider
-> OpenAI-compatible Chat Completions transport
```

Provider SDK не используется. Клиент строится на `net/http` и `encoding/json`.

Один shared `http.Client` и Transport переиспользуются всеми calls.

## 10. Instructions and manifests

```text
protected XML instruction
+ one active manifest
+ runtime context
```

Default manifest хранится как protected XML.

Пользователь вводит free-form text в OpenWebUI. Этот текст становится user manifest и временно отключает default manifest для соответствующего scope.

При удалении user manifest default manifest автоматически возвращается.

Подробный contract:

```text
contracts/INSTRUCTION_AND_MANIFEST.md
```

## 11. Emotion module

Эмоциональное состояние реализуется собственным deterministic Go module.

Он:

- не использует LLM;
- не является microservice;
- хранит versioned state в Neon;
- применяет lazy decay;
- формирует компактный EmotionReport;
- не имеет инструментов и provider credentials.

## 12. OpenWebUI and auth

OpenWebUI предоставляет:

- регистрацию;
- password auth;
- OAuth/OIDC при необходимости;
- users и roles;
- admin interface;
- chat UI;
- поле пользовательской инструкции.

OpenWebUI является public service.

Sonata API развёртывается как private service и доверяет forwarded user headers только после проверки внутреннего service credential.

Supabase не является частью mini MVP.

## 13. User manifest input

Пользовательская инструкция из OpenWebUI:

- принимается как free-form UTF-8 text;
- не парсится как XML;
- не считается system instruction;
- не получает доступ к protected core;
- хранится в Neon;
- имеет global или chat scope;
- заменяет default manifest, но не protected instruction.

## 14. Web search

```text
LangSearch
+ custom Go HTTP client
```

LangSearch принадлежит только Synthesis.

Router, raw, critical и summary roles не имеют tool access.

## 15. Sandbox

```text
ENABLE_CODE_SANDBOX=false
```

В mini MVP нет выполнения пользовательского кода.

Можно сохранить интерфейс `CodeSandbox`, но production implementation отсутствует.

Будущая cloud sandbox должна иметь:

- отдельное isolated environment;
- CPU, RAM, disk и timeout limits;
- ephemeral filesystem;
- no access to Sonata secrets;
- network disabled by default;
- preinstalled toolchains;
- автоматическое уничтожение runtime.

## 16. Object storage

Cloudflare R2 добавляется только при включении document uploads.

```text
R2      raw files and generated artifacts
Neon    metadata and ownership
Qdrant  searchable chunks
```

Local Render filesystem не является permanent storage.

## 17. Configuration and secrets

```text
go.yaml.in/yaml/v3
+ custom typed loader
+ config/index.yaml
+ split domain YAML
+ Render Environment Group
+ Render secret files
```

Подробный ADR:

```text
decisions/CONFIG_AND_SECRETS.md
```

## 18. Observability

```text
log/slog JSON
OpenTelemetry Go
OTLP/HTTP
Grafana Cloud Free
```

Mini MVP отправляет telemetry напрямую в Grafana Cloud без отдельного Collector.

Один user request соответствует одному trace.

Основные spans:

```text
chat.request
router
context.load
emotion.update
phase.raw
phase.critical
phase.summary
synthesis.tooling
tool.langsearch
synthesis.final
persistence
```

Запрещено отправлять:

- provider keys;
- protected instructions;
- default manifests;
- compiled prompts;
- полные role reports;
- RAG chunks;
- user documents;
- полный user manifest по умолчанию.

## 19. Deployment

```text
Render public web service
-> OpenWebUI

Render private service
-> Sonata API

Render background worker
-> Sonata Worker

External
-> Neon
-> Qdrant Cloud
-> OpenCode Zen
-> LangSearch
-> Grafana Cloud
-> Cloudflare R2 when enabled
```

Infrastructure описывается в `render.yaml`.

## 20. CI and quality

```text
GitHub Actions
go test ./...
go test -race ./...
staticcheck
govulncheck
go vet
```

Integration tests используют:

- `httptest`;
- test PostgreSQL container;
- Qdrant test container или mock boundary;
- mock OpenCode Zen server;
- mock LangSearch server.

Fuzz targets:

- YAML config loader;
- user manifest input;
- provider response parser;
- SSE stream parser;
- structured model outputs;
- tool plan validation.

## 21. Решения, отложенные после mini MVP

- code sandbox provider;
- BYOK bridge, который питает внутренний pipeline Sonata;
- browser IDE или VS Code workspace;
- external public Sonata API keys;
- optional OpenTelemetry Collector;
- ColBERT activation;
- targeted per-prism user manifests;
- Supabase integration.
