# Sonata mini MVP checklist

> Статус: основной рабочий трекер mini MVP  
> Канонические требования: [`ARCHITECTURE.md`](./ARCHITECTURE.md), [`TECH_STACK.md`](./TECH_STACK.md)  
> Правило: пункт отмечается выполненным только после реализации и проверки его критерия приёмки.

## Обозначения

- `[x]` — реализовано и зафиксировано.
- `[ ]` — не реализовано или не прошло обязательную проверку.
- Родительский пункт закрывается только после закрытия всех обязательных подпунктов.

## 00. Граница и документация

- [x] Документация mini MVP вынесена в отдельную директорию `docs/mini-mvp/`.
- [x] Зафиксирована продуктовая архитектура mini MVP.
- [x] Зафиксирован технологический стек.
- [x] Зафиксировано разделение protected instructions и manifests.
- [x] Зафиксирована политика OpenCode Zen и маршрутизация моделей.
- [x] Зафиксирована единая система конфигурации и секретов.
- [x] Зафиксирован детерминированный эмоциональный модуль.
- [x] Корневой `AGENTS.md` направляет mini MVP задачи в `docs/mini-mvp/`.
- [ ] Все новые архитектурные решения сопровождаются обновлением этого checklist.

## 01. Go foundation и configuration

Документы:

- [`TECH_STACK.md`](./TECH_STACK.md)
- [`decisions/CONFIG_AND_SECRETS.md`](./decisions/CONFIG_AND_SECRETS.md)

### Каркас

- [x] Создан один Go module.
- [x] Создан основной binary `cmd/sonata`.
- [x] Добавлены команды `sonata config validate` и `sonata config print --redacted`.
- [x] Создан единый entrypoint `internal/config`.
- [x] Конфигурация разделена на domain YAML fragments.
- [x] `config/index.yaml` явно задаёт порядок загрузки.
- [x] Добавлены profiles `local`, `staging`, `production`.
- [x] Реализован deterministic deep merge.
- [x] Реализован strict decode с запретом unknown fields.
- [x] Реализованы typed Go config structs.
- [x] Реализована semantic validation.
- [x] Реализованы YAML anchors внутри одного fragment.
- [x] Cross-file anchors не используются.

### Секреты

- [x] `config/secrets.yaml` хранит только logical references.
- [x] Поддержаны secret sources `env` и `file`.
- [x] Реализован `SecretValue` с redacted formatting и serialization.
- [x] Добавлен `.env.example` без реальных значений.
- [x] Проверено, что ни один secret не попадает в logs, YAML snapshot и ошибки startup.
- [x] Добавлен автоматический secret-pattern scan в CI.

### Проверка

- [x] Добавлены unit tests config loader, merge, missing secrets и redaction.
- [x] `go mod tidy` выполнен в окружении с доступом к registry.
- [x] `go test ./...` проходит на целевой версии Go.
- [x] `go test -race ./...` проходит.
- [x] Команда `sonata config validate --profile production` проходит с тестовыми secrets.
- [x] Команда `sonata config print --redacted` не содержит raw secret values.

**Критерий этапа:** configuration загружается одинаково в API и Worker, invalid production config останавливает startup.

## 02. CI и качество кода

- [x] Добавлен GitHub Actions workflow для Go.
- [x] В CI закреплена выбранная версия Go.
- [x] CI запускает `go mod tidy` и проверяет чистый git diff.
- [x] CI запускает `go test ./...`.
- [x] CI запускает `go test -race ./...`.
- [x] CI запускает `go vet ./...`.
- [x] CI запускает `staticcheck ./...`.
- [x] CI запускает `govulncheck ./...`.
- [x] Добавлена проверка YAML configuration всех profiles.
- [x] Добавлена проверка отсутствия секретов в repository.
- [x] Добавлены минимальные coverage thresholds для критических domain modules.

**Критерий этапа:** merge невозможен при ошибке config, tests, race detector или secret scan.

## 03. HTTP API и OpenWebUI boundary

- [x] Добавлены `net/http` и `chi` application server.
- [x] Реализован graceful shutdown.
- [x] Реализованы request ID, panic recovery, timeout и structured logging middleware.
- [x] Реализован `GET /internal/health/live`.
- [x] Реализован `GET /internal/health/ready`.
- [x] Реализован `GET /v1/models` с моделью `sonata`.
- [x] Реализован каркас `POST /v1/chat/completions`.
- [ ] Реализован OpenAI-compatible SSE streaming.
- [ ] Отмена клиентского запроса отменяет весь cognitive pipeline.
- [ ] Sonata API развёртывается как private service.
- [ ] Проверяется внутренний credential OpenWebUI → Sonata.
- [ ] Forwarded OpenWebUI user headers принимаются только после проверки service credential.
- [ ] Стабильно связываются OpenWebUI user, chat и message IDs.
- [ ] Встроенная memory OpenWebUI отключена.
- [ ] Direct provider models визуально и логически отделены от модели Sonata.

**Критерий этапа:** OpenWebUI видит модель `sonata`, получает streaming-ответ и не может подделать другого пользователя прямым HTTP-запросом.

## 04. OpenCode Zen provider

Документ: [`decisions/OPENCODE_ZEN_PROVIDER.md`](./decisions/OPENCODE_ZEN_PROVIDER.md)

- [ ] Реализован `ModelProvider` interface.
- [ ] Реализован `OpenCodeZenProvider` на `net/http` и `encoding/json`.
- [ ] Используется один shared `http.Client` и Transport.
- [ ] Endpoint берётся только из typed config.
- [ ] Master key передаётся adapter через `SecretValue`.
- [ ] Реализован allowlist моделей.
- [ ] Реализована role-to-model routing policy:
  - [ ] Router → `nemotron-3-ultra-free`.
  - [ ] Raw/Critical → `deepseek-v4-flash-free`.
  - [ ] Summary → `nemotron-3-ultra-free`.
  - [ ] Synthesis → `big-pickle`.
- [ ] Реализован fallback на `mimo-v2.5-free` по принятой схеме.
- [ ] `north-mini-code-free` не используется обычным pipeline.
- [ ] Model failure отделён от `PROVIDER_EXHAUSTED`.
- [ ] Реализованы timeout, retry budget и circuit breaker.
- [ ] Raw provider errors проходят redaction.
- [ ] Добавлен mock Zen server для integration tests.
- [ ] Streaming и non-streaming ответы покрыты тестами.

**Критерий этапа:** каждая runtime role использует заданную модель, fallback работает только при model-level failure, master key не виден вне adapter.

## 05. Protected instructions и manifests

Документ: [`contracts/INSTRUCTION_AND_MANIFEST.md`](./contracts/INSTRUCTION_AND_MANIFEST.md)

- [ ] Определена директория protected instructions.
- [ ] Определена директория protected default manifests.
- [ ] Перенесены и смыслово адаптированы инструкции из vanilla Sonata.
- [ ] Удалён язык отдельных личностей и независимых агентов.
- [ ] Каждая instruction имеет stable ID, version и hash.
- [ ] Каждый default manifest имеет stable ID, version и hash.
- [ ] Реализован strict XML loader protected artifacts.
- [ ] Реализован typed prompt compiler.
- [ ] Реализован manifest resolver:
  - [ ] chat user manifest;
  - [ ] global user manifest;
  - [ ] protected default manifest.
- [ ] При наличии user manifest default manifest отключается, но не удаляется.
- [ ] При удалении user manifest default manifest автоматически возвращается.
- [ ] User manifest принимается как free-form text, а не как XML/system instruction.
- [ ] User manifest ограничен по размеру и нормализуется.
- [ ] Пользователь не может отключить protected instruction через prompt injection.
- [ ] Compiled prompt не сохраняется в обычных logs и traces.
- [ ] Реализован output guard против утечки protected fragments.
- [ ] Добавлены tests на disclosure attempts, manifest fallback и cross-user access.

**Критерий этапа:** пользователь меняет только manifest; identity, phase isolation, tools и security invariants всегда задаются protected instruction.

## 06. Cognitive state machine

- [ ] Определены typed input/output contracts всех runtime roles.
- [ ] Определён JSON contract Router: только `direct | full`.
- [ ] Router не имеет tools, RAG, EmotionReport и права менять модели.
- [ ] Реализован direct route.
- [ ] Реализован full route из 18 LLM-вызовов.
- [ ] Raw-фаза запускает пять призм параллельно.
- [ ] Raw-призмы не видят ответы друг друга.
- [ ] Каждый critical run видит только raw своей призмы.
- [ ] Каждый summary run видит только raw и critical своей призмы.
- [ ] `synthesis_tooling` получает полный внутренний диалог.
- [ ] `synthesis_final` получает внутренний диалог и результаты tools.
- [ ] Два прохода Synthesis сохраняют единую identity Sonata.
- [ ] Падение одной призмы может дать `DEGRADED`, а не обязательный полный отказ.
- [ ] Установлены phase timeouts и concurrency limits.
- [ ] Сохраняются role status, latency, model ID, instruction и manifest metadata.
- [ ] Добавлены deterministic orchestration tests без реальных LLM.

**Критерий этапа:** изоляция пяти призм доказана тестами, а сложный запрос стабильно проходит весь pipeline.

## 07. Emotional state module

Документ: [`modules/EMOTION_MODULE.md`](./modules/EMOTION_MODULE.md)

- [ ] Реализованы typed emotion и relationship state.
- [ ] Реализован baseline profile из config.
- [ ] Реализован deterministic stimulus extractor.
- [ ] Реализованы bounded transitions.
- [ ] Реализованы opposition/dominance rules.
- [ ] Реализован lazy decay.
- [ ] Реализован versioned state update.
- [ ] Реализован компактный `EmotionReport`.
- [ ] State изолирован по `user_id`.
- [ ] Модуль не использует LLM.
- [ ] Модуль не имеет tools и provider credentials.
- [ ] Emotional state не может менять security policy и memory facts.
- [ ] Добавлены tests на decay, bounds, conflict rules и concurrent updates.

**Критерий этапа:** одинаковая последовательность событий даёт воспроизводимое bounded состояние без LLM.

## 08. Neon PostgreSQL и canonical storage

- [ ] Создана структура migrations.
- [ ] Подключены `pgx/v5` и `pgxpool`.
- [ ] Подключён `sqlc`.
- [ ] Подключён `goose`.
- [ ] Разделены pooled runtime URL и direct migration URL.
- [ ] Созданы таблицы:
  - [ ] users;
  - [ ] conversations;
  - [ ] messages;
  - [ ] cognitive_runs;
  - [ ] role_runs;
  - [ ] tool_calls;
  - [ ] instruction_versions;
  - [ ] manifest_versions;
  - [ ] user_manifests;
  - [ ] emotional_states;
  - [ ] emotional_events;
  - [ ] memory_items;
  - [ ] documents;
  - [ ] provider_usage;
  - [ ] outbox_events.
- [ ] Все user-owned сущности имеют строгий owner boundary.
- [ ] Реализованы transactions для cognitive run и связанных role runs.
- [ ] Реализован optimistic/version lock emotional state.
- [ ] OpenWebUI и Sonata используют отдельные databases или schemas.
- [ ] Добавлены integration tests PostgreSQL.

**Критерий этапа:** Neon является единственным canonical source of truth, cross-user чтение невозможно на repository/service boundary.

## 09. Background jobs

- [ ] Подключён River без Redis.
- [ ] Один binary поддерживает режим `sonata worker`.
- [ ] Реализован transactional enqueue через outbox/transaction boundary.
- [ ] Добавлены jobs для Qdrant indexing.
- [ ] Добавлены jobs для conversation summary.
- [ ] Добавлены retry и dead-letter правила.
- [ ] Добавлены cleanup jobs.
- [ ] Interactive LLM pipeline не выполняется через River.
- [ ] Worker использует тот же validated RuntimeConfig.
- [ ] Добавлены integration tests job idempotency.

**Критерий этапа:** повтор job безопасен, а сбой Qdrant не откатывает canonical данные Neon.

## 10. Qdrant и memory retrieval

- [ ] Подключён официальный Go client Qdrant.
- [ ] Созданы collections `sonata_memory` и `sonata_documents`.
- [ ] Каждый point ссылается на canonical ID и owner ID.
- [ ] Реализован dense retrieval.
- [ ] Реализован BM25 sparse retrieval.
- [ ] Реализована fusion strategy.
- [ ] ColBERT остаётся выключенным feature flag.
- [ ] Реализована пересборка projection из Neon.
- [ ] Реализованы delete/update jobs.
- [ ] Context assembler соблюдает token budget.
- [ ] Retrieval строго фильтруется по user ownership.
- [ ] Добавлены relevance и isolation tests.

**Критерий этапа:** удаление Qdrant collection не уничтожает данные, projection восстанавливается из Neon.

## 11. Synthesis tools и LangSearch

- [ ] Определён typed `ToolPlan` contract.
- [ ] Реализован Tool Executor с allowlist.
- [ ] Только `synthesis_tooling` может создавать tool calls.
- [ ] Реализован LangSearch Go client.
- [ ] LangSearch key загружается через secret reference.
- [ ] Реализованы timeout, max calls и max result size.
- [ ] Tool results нормализуются перед передачей модели.
- [ ] Tool errors не раскрывают secrets и internal network.
- [ ] Дополнительный memory search доступен только Synthesis.
- [x] Sandbox выключена в mini MVP config.
- [ ] В production отсутствует рабочая implementation code execution.
- [ ] Добавлены tests на запрещённые tools и bounded loop.

**Критерий этапа:** ни Router, ни призмы, ни critical/summary роли не могут вызвать внешний инструмент.

## 12. User manifest persistence и API

- [ ] Реализован `GET /api/v1/manifest`.
- [ ] Реализован `PUT /api/v1/manifest`.
- [ ] Реализован `DELETE /api/v1/manifest`.
- [ ] Поддержан global scope.
- [ ] Поддержан chat scope или явно отложен feature flag.
- [ ] Manifest versioned при каждом изменении.
- [ ] Delete/disable немедленно возвращает default manifest.
- [ ] Пользователь может читать и менять только свой manifest.
- [ ] Protected default manifest никогда не возвращается API.
- [ ] OpenWebUI instruction field корректно преобразуется в user manifest.
- [ ] Добавлены API tests auth, ownership и fallback.

**Критерий этапа:** пользовательская инструкция из OpenWebUI заменяет только default manifest своего scope.

## 13. Observability

- [ ] Подключён `log/slog` JSON handler.
- [ ] Подключён OpenTelemetry Go SDK.
- [ ] Настроен OTLP/HTTP export в Grafana Cloud.
- [ ] API и Worker имеют разные `service.name`.
- [ ] Один user request создаёт один root trace.
- [ ] Созданы spans Router, phases, roles, Synthesis, tools, DB и Qdrant.
- [ ] Созданы metrics latency, failures, active pipelines, tokens и provider exhaustion.
- [ ] High-cardinality IDs не используются как metric labels.
- [ ] Prompts, manifests, role reports, RAG chunks и secrets не отправляются в telemetry.
- [ ] Реализована central redaction policy.
- [ ] Создан минимальный Grafana dashboard.
- [ ] Созданы alerts provider exhaustion и elevated failure rate.

**Критерий этапа:** полный запрос прослеживается по trace без раскрытия пользовательского содержимого и внутренних instructions.

## 14. Security и abuse protection

- [ ] Sonata API недоступна напрямую из public internet.
- [ ] OpenWebUI service credential ротируется без изменения кода.
- [ ] Добавлены request body и manifest size limits.
- [ ] Добавлен global concurrency limit full pipelines.
- [ ] Добавлен basic anti-abuse rate control.
- [ ] Добавлена защита от prompt disclosure.
- [ ] Добавлена защита от cross-user memory retrieval.
- [ ] Добавлена защита от cross-user manifest access.
- [ ] Provider и tool error bodies проходят redaction.
- [ ] Никакие secrets не попадают в model context.
- [ ] XML protected artifacts защищены от DTD/XXE и path traversal.
- [ ] Добавлены security regression tests.
- [ ] Проведён ручной adversarial smoke test перед релизом.

**Критерий этапа:** тесты подтверждают isolation пользователей, protected core и provider credentials.

## 15. Deployment на Render

- [ ] Создан `render.yaml`.
- [ ] Добавлен public OpenWebUI service.
- [ ] Добавлен private Sonata API service.
- [ ] Добавлен Sonata background worker.
- [ ] Создан единый Render Environment Group для Sonata secrets.
- [ ] Business configuration не размазана по Render variables.
- [ ] Настроена Neon database.
- [ ] Настроен Qdrant Cloud.
- [ ] Настроен OpenCode Zen master key.
- [ ] Настроен LangSearch key.
- [ ] Настроен Grafana Cloud OTLP.
- [ ] Выполняются migrations перед запуском новой версии.
- [ ] Readiness учитывает database и обязательные providers.
- [ ] Проверен graceful deploy без потери активных requests.
- [ ] Документирован rollback.

**Критерий этапа:** новая среда поднимается декларативно из repository и secret values без ручного редактирования application config.

## 16. End-to-end acceptance

- [ ] Пользователь регистрируется и входит через OpenWebUI.
- [ ] OpenWebUI показывает модель `sonata`.
- [ ] Приветствие проходит direct route.
- [ ] Сложный запрос проходит full route из 18 LLM calls.
- [ ] Пять raw-призм выполняются изолированно.
- [ ] Critical и Summary видят только собственную призму.
- [ ] Synthesis единственный владелец tools.
- [ ] LangSearch вызывается и возвращает нормализованный результат.
- [ ] EmotionReport влияет на ответ, но не нарушает security rules.
- [ ] User manifest отключает default manifest.
- [ ] Удаление user manifest возвращает default manifest.
- [ ] Memory сохраняется в Neon и индексируется в Qdrant.
- [ ] Повторный запрос использует релевантную память только текущего пользователя.
- [ ] Падение одной призмы возвращает `DEGRADED`, если Synthesis может продолжить.
- [ ] Исчерпание общего Zen key возвращает `PROVIDER_EXHAUSTED`.
- [ ] В logs, traces и API отсутствуют keys и protected content.
- [ ] Система развёрнута в production profile.

## 17. Отложено после mini MVP

Эти пункты не блокируют первый релиз:

- [ ] Free cloud code sandbox.
- [ ] Browser IDE или VS Code workspace.
- [ ] `north-mini-code-free` code workflow.
- [ ] BYOK bridge для внутреннего pipeline Sonata.
- [ ] Внешние Sonata API keys и proxy mode.
- [ ] Supabase integration.
- [ ] OpenTelemetry Collector.
- [ ] ColBERT activation после benchmark.
- [ ] Cloudflare R2 и document uploads.
- [ ] Глобальный emotional state общей Sonata.
- [ ] Targeted per-prism user manifests.

## Release gate

Mini MVP считается готовым только когда:

- [ ] Закрыты разделы 01–16.
- [ ] Все CI checks зелёные.
- [ ] Production config validation проходит.
- [ ] End-to-end acceptance выполнен минимум на двух пользователях для проверки isolation.
- [ ] Проведена проверка отсутствия secrets и protected instructions в logs, traces, database и API responses.
- [ ] Зафиксирован release commit и deployment rollback point.
