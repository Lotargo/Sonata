# Neon PostgreSQL canonical storage

> Статус: first vertical slice implemented, ожидает полного CI  
> Candidate implementation head: `e6fb6649eaa04a1f0553a2d87f7106ac2de6e032`  
> Database schema: `sonata`  
> Migration format: goose SQL

## 1. Назначение

Neon PostgreSQL является canonical source of truth Sonata mini MVP.

Первый storage increment создаёт стабильную границу для следующих этапов:

```text
logical secret refs
-> pooled runtime URL / direct migration URL
-> pgxpool runtime connection
-> versioned goose migrations
-> owner-scoped relational schema
-> transactional affective CAS repository
```

Qdrant, River и последующие API работают поверх этой canonical schema и не становятся владельцами исходных данных.

## 2. Разделение подключений

Configuration уже содержит два независимых logical refs:

```text
storage.database.url_ref
storage.database.direct_url_ref
```

Назначение:

- runtime API и Worker используют `DATABASE_URL` через `pgxpool`;
- migrations и maintenance используют `DATABASE_DIRECT_URL`;
- local/CI могут временно использовать одну и ту же direct PostgreSQL строку в обеих переменных;
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

GitHub Actions поднимает PostgreSQL 16, применяет обе migrations через pinned `goose`, затем запускает unit, race и database integration tests.

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

Поэтому ошибочный или скомпрометированный repository method не может привязать child row к parent другого владельца простой подменой ID.

## 7. Affective persistence

`PostgresAffectiveStateStore` реализует существующий `emotion.AffectiveStateStore`.

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
- timestamps нормализуются до PostgreSQL microsecond precision до serialization;
- raw message text не записывается в affective state/event payload.

## 8. Verification

Database integration tests требуют `SONATA_TEST_DATABASE_URL` и проверяют:

- database migrations применены;
- cross-owner message insert блокируется PostgreSQL foreign key;
- affective state version 1 создаётся через CAS;
- stale CAS отклоняется;
- version 2 атомарно заменяет state;
- каждой подтверждённой версии соответствует affective event.

В GitHub Actions переменная направлена на изолированный PostgreSQL service текущего CI job.

## 9. Следующий increment

Этот срез не закрывает stage 08 целиком. Далее требуются:

- `sqlc` configuration и generated query layer;
- repositories для conversations, messages, cognitive/role runs и manifests;
- transaction boundary cognitive run + role runs;
- запуск migrations через deployment/pre-deploy command;
- wiring runtime API к `pgxpool` и Postgres affective store;
- дополнительные integration tests ownership и rollback;
- проверка на реальной Neon branch перед staging.

Checkboxes stage 08 отмечаются только после зелёного полного CI соответствующего implementation head.
