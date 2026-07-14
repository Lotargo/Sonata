# Render deployment: Sonata API и OpenWebUI

> Статус: конфигурация deployment подготовлена; live acceptance ещё не выполнен  
> Scope: Render Blueprint, private network boundary и настройки OpenWebUI для mini MVP

## 1. Топология

`render.yaml` создаёт два сервиса в одном регионе:

```text
Internet
-> public sonata-web (OpenWebUI)
-> Render private network
-> private sonata-api
```

`sonata-api` имеет тип Render `pserv` и не должен получать публичный `onrender.com` URL. OpenWebUI является единственной публичной точкой входа пользователя.

## 2. Sonata API

Blueprint:

- собирает `cmd/sonata` как один Go binary;
- во время build устанавливает отдельный pinned `goose v3.27.2` в `./bin`;
- перед каждым start применяет canonical migrations из `internal/database/migrations`;
- запускает `sonata api` с profile `production` только после успешных migrations;
- использует `/internal/health/ready` как health check;
- даёт приложению время на graceful shutdown;
- подключает одну environment group `sonata-runtime-secrets`.

Migration boundary:

```text
buildCommand
-> build ./bin/sonata
-> install ./bin/goose@v3.27.2

preDeployCommand
-> ./bin/goose
-> internal/database/migrations
-> postgres "$DATABASE_DIRECT_URL" up

startCommand
-> ./bin/sonata api --profile production
```

Migrations используют только direct connection `DATABASE_DIRECT_URL`. Pooled runtime connection `DATABASE_URL` предназначен для API и будущего Worker и не должен использоваться migration process.

Ошибка migration завершает pre-deploy command с ненулевым status и блокирует запуск новой версии Sonata. Приложение не должно стартовать поверх частично или неуспешно обновлённой schema.

При первом создании Blueprint оператор вводит значения, помеченные `sync: false`. `OPENWEBUI_INTERNAL_SECRET` генерируется Render и не хранится в repository.

До live deployment также нужно создать secret file:

```text
/etc/secrets/grafana-otlp-headers
```

Он должен содержать OTLP headers для Grafana Cloud. Без него production configuration обязана остановить startup, потому что OTLP включён в production profile.

## 3. OpenWebUI

OpenWebUI разворачивается из закреплённого image:

```text
ghcr.io/open-webui/open-webui:v0.10.2
```

Его единственное OpenAI-compatible соединение формируется из private address `sonata-api`:

```text
http://${SONATA_INTERNAL_ADDRESS}/v1
```

Credential передаётся через `OPENAI_API_KEY`, полученный из `OPENWEBUI_INTERNAL_SECRET` private-сервиса. Provider credentials Sonata не передаются в OpenWebUI.

Обязательные значения:

```text
ENABLE_OPENAI_API=true
ENABLE_OLLAMA_API=false
ENABLE_MEMORIES=false
ENABLE_FORWARD_USER_INFO_HEADERS=true
```

В Blueprint отсутствуют direct provider connections. Поэтому список моделей OpenWebUI должен поступать только из `GET /v1/models` Sonata и содержать модель `sonata`.

## 4. Автоматическая проверка

`internal/deployment/render_blueprint_test.go` защищает следующие invariants:

- Sonata остаётся private Go service;
- OpenWebUI остаётся public service;
- image OpenWebUI закреплён по версии;
- Sonata build создаёт application binary и pinned `goose` binary;
- pre-deploy command применяет canonical migrations через `DATABASE_DIRECT_URL`;
- pooled `DATABASE_URL` не используется migration process;
- OpenWebUI подключён только к private Sonata address;
- internal credential не записан в repository;
- встроенная memory и Ollama API отключены;
- forwarding OpenWebUI identity headers включён;
- direct provider environment variables отсутствуют.

## 5. Live acceptance

Пункты deployment в `CHECKLIST.md` закрываются только после реального Render deployment.

Нужно проверить:

1. У `sonata-api` отсутствует публичный URL.
2. Pre-deploy logs показывают успешное применение или отсутствие pending migrations.
3. При намеренно invalid migration новая версия не запускается, а предыдущая продолжает обслуживать requests.
4. `/internal/health/live` и `/internal/health/ready` доступны OpenWebUI внутри private network.
5. `/v1/models` без service credential возвращает `401`.
6. OpenWebUI показывает только модель `sonata`.
7. Streaming chat заканчивается `data: [DONE]`.
8. Подмена `X-OpenWebUI-*` без service credential не проходит.
9. Memory OpenWebUI отсутствует в пользовательском интерфейсе и не сохраняет параллельные воспоминания.
10. В OpenWebUI отсутствуют direct OpenCode Zen или другие provider models.
