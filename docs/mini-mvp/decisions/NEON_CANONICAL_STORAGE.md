# Neon PostgreSQL canonical storage

> Статус: third vertical slice implemented and verified by full CI; stage remains open for runtime wiring  
> Verified PR #6 code head: `33371442be2d2d352697d3da87f8786715eb9db0`; merged as `601f9795a65fae48426621aba12acb66dfa8dae9`  
> Database schema: `sonata`  
> Migration format: goose SQL  
> Query generation: sqlc `v1.31.1`, pgx/v5

## 1. Назначение

Neon PostgreSQL является canonical source of truth Sonata mini MVP.

Текущий storage boundary:

```text
logical secret refs
-> pooled runtime URL / direct migration URL
-> pgxpool runtime connection
-> versioned goose migrations
-> owner-scoped relational schema
-> sqlc typed queries
-> transactional repositories
```

Qdrant, River и последующие API работают поверх этой canonical schema и не становятся владельцами исходных данных.

## 2. Разделение подключений

Configuration содержит два независимых logical refs:

```text
storage.database.url_ref
storage.database.direct_url_ref
```

Назначение:

- runtime API и Worker используют `DATABASE_URL` через `pgxpool`;
- migrations и maintenance используют `DATABASE_DIRECT_URL`;
- local/CI могут временно использовать одну direct PostgreSQL строку в обеих переменных;
- staging/production должны использовать pooled Neon endpoint для `DATABASE_URL` и direct endpoint для `DATABASE_DIRECT_URL`.

Raw connection strings не сохраняются в logs, errors или documentation snapshots.

## 3. Изоляция schema

Все canonical entities Sonata размещаются в PostgreSQL schema:

```text
sonata
```

OpenWebUI не должен использовать эту schema. Для production требуется отдельная database либо отдельная schema и отдельные credentials OpenWebUI.

Runtime pool устанавливает:

```text
search_path=sonata,public
application_name=<service_name>
```

## 4. Migrations

Migration directory:

```text
internal/database/migrations/
```

Текущие migrations:

1. `00001_canonical_schema.sql` создаёт extension, schema, таблицы, indexes и базовые constraints.
2. `00002_owner_invariants.sql` усиливает составные owner boundaries для manifests, role runs, tool calls, provider usage и memory items.
3. `00003_role_manifest_source.sql` сохраняет полный `ManifestRef.Source` каждого role run.
4. `00004_run_completion_invariants.sql` запрещает дубли canonical role внутри одного cognitive run.

GitHub Actions `Go CI` поднимает PostgreSQL 16, применяет migrations через pinned `goose`, затем запускает unit, race и database integration tests.

Render Blueprint на build-этапе устанавливает отдельный pinned `goose v3.27.2` в `./bin`. `preDeployCommand` применяет migrations через `DATABASE_DIRECT_URL` до запуска новой версии `sonata-api`; pooled `DATABASE_URL` migration process не использует. Ненулевой exit status блокирует новый deployment до старта application process.

## 5. Canonical entities

Созданы таблицы:

- `users`;
- `conversations`;
- `messages`;
- `cognitive_runs`;
- `role_runs`;
- `tool_calls`;
- `instruction_versions`;
- `manifest_versions`;
- `user_manifests`;
- `affective_states`;
- `affective_events`;
- `memory_items`;
- `documents`;
- `provider_usage`;
- `outbox_events`.

UUID генерируются PostgreSQL через `pgcrypto`. User, conversation и forwarded message IDs остаются text, потому что их источником является trusted OpenWebUI boundary.

## 6. Owner boundary

User-owned parent/child связи используют составные keys и foreign keys с `owner_id`.

Примеры:

```text
message(owner_id, conversation_id)
-> conversation(owner_id, id)

role_run(owner_id, cognitive_run_id)
-> cognitive_run(owner_id, id)

tool_call(owner_id, cognitive_run_id, role_run_id)
-> role_run(owner_id, cognitive_run_id, id)
```

Поэтому repository method не может привязать child row к parent другого владельца простой подменой ID.

## 7. sqlc query layer

Configuration:

```text
sqlc.yaml
schema  -> internal/database/migrations
queries -> internal/database/queries
output  -> internal/database/dbgen
runtime -> pgx/v5
```

Query groups:

- `core.sql`: users, conversations и messages;
- `runs.sql`: cognitive runs и role runs;
- `manifests.sql`: versioned user manifests и scope locks.

Generated bindings хранятся в repository, поэтому обычная Go-сборка не зависит от установленного `sqlc` CLI.

Отдельный `SQLC CI`:

```text
sqlc generate
-> sqlc vet
-> go test ./internal/database/...
-> generated gofmt check
```

Он проверяет query/schema consistency независимо от PostgreSQL integration job.

## 8. Cognitive run transaction

`RunRepository.BeginCognitiveRun` фиксирует одной PostgreSQL transaction:

```text
ensure user
-> upsert conversation
-> insert request message
-> create cognitive run
-> create related role runs
-> commit
```

Инварианты:

- owner, conversation и message IDs обязательны;
- message и metadata должны быть valid JSON;
- route проходит typed validation;
- role IDs уникальны внутри input;
- model ID обязателен;
- instruction ID обязан совпадать с canonical `RuntimeRoleSpec`;
- instruction и manifest versions положительны;
- manifest source сохраняется как `system_default | user_global | user_chat`;
- phase и perspective выводятся repository из canonical role spec, а не принимаются от caller;
- request cancellation не мешает deferred rollback освободить transaction.

Integration test намеренно создаёт duplicate message conflict после conversation upsert и подтверждает, что conversation и cognitive run полностью откатываются.

## 9. Terminal run policy

`RunRepository.CompleteRoleRun` переводит role из `RUNNING` только если совпадают:

- owner ID;
- cognitive run ID;
- role run ID;
- phase и perspective canonical роли;
- instruction ID, version и hash;
- manifest ID, version, hash и source.

Публичная ошибка не различает отсутствующий row и identity mismatch.

`RunRepository.CompleteCognitiveRun` разрешает terminal status только когда:

- run ещё находится в `RUNNING`;
- не осталось role runs со статусом `RUNNING`;
- при status `OK` все связанные role runs также имеют `OK`.

Повторное завершение role или run отклоняется. Integration tests проверяют раннее завершение, manifest mismatch и повторный terminal update.

## 10. Versioned user manifests

`ManifestRepository` использует тот же normalizer, что runtime resolver:

```text
UTF-8 validation
-> CRLF normalization
-> Unicode NFC
-> trim
-> byte limit
-> SHA-256
```

PUT и DELETE одного owner/scope выполняются под transaction-level advisory lock.

Одна версия выполняет:

```text
ensure user
-> lock owner/scope
-> lock current manifest row
-> allocate stable manifest ID when absent
-> increment version exactly once
-> insert immutable manifest_versions metadata
-> update current user_manifests row
-> commit
```

Soft delete создаёт новую version со статусом `deleted`, очищает current content и немедленно позволяет resolver вернуться к следующему manifest по приоритету.

Integration tests проверяют:

- stable manifest ID;
- PUT versions `1 -> 2`;
- DELETE version `3`;
- normalized content hash;
- owner isolation;
- восемь конкурентных updates одного chat scope получают уникальные последовательные versions без orphaned current row.

## 11. Affective persistence

`PostgresAffectiveStateStore` реализует `emotion.AffectiveStateStore`.

Одна transaction выполняет:

```text
ensure canonical user
-> INSERT or version-checked UPDATE affective_states
-> INSERT state_transition affective_event
-> COMMIT
```

Правила:

- stale expected version возвращает `emotion.ErrVersionConflict`;
- state version увеличивается ровно на один;
- JSON state envelope проверяется против indexed columns при чтении;
- timestamps нормализуются до PostgreSQL microsecond precision;
- raw message text не записывается в affective state/event payload.

## 12. CI status

На PR #6 code head `33371442be2d2d352697d3da87f8786715eb9db0` подтверждены:

- `SQLC CI` run `29310652075`: `success`;
- `Go CI` run `29310652083`: `success`;
- `Secret scan`: `success`.

Исправление слито в `main` merge commit `601f9795a65fae48426621aba12acb66dfa8dae9`.

Полный Go CI выполнил `go mod tidy` с чистым diff, formatting check, canonical migrations на PostgreSQL 16, unit и database integration tests, coverage threshold, race detector, `go vet`, `staticcheck`, `govulncheck`, validation всех configuration profiles и проверку redacted output.

SQLC CI повторно сгенерировал bindings через pinned `sqlc v1.31.1`, выполнил `sqlc vet`, подтвердил отсутствие generated diff и успешную сборку `internal/database`.

## 13. Следующий increment

Stage 08 ещё требует:

- repositories для `tool_calls`, `provider_usage`, `outbox_events` и metadata protected artifacts;
- wiring runtime API к `pgxpool`, Postgres affective store и canonical repositories на всех обязательных путях;
- отдельной database или schema и credentials для OpenWebUI в развёрнутой среде;
- проверки storage slice на реальной Neon branch перед staging.
