# OpenWebUI → Sonata boundary contract

> Статус: accepted contract mini MVP  
> Назначение: зафиксировать service authentication, forwarded identity и streaming boundary между public OpenWebUI и private Sonata API

## 1. Сетевая граница

- OpenWebUI развёртывается как public service и выполняет user authentication.
- Sonata API развёртывается как private service.
- Наличие private network не заменяет application-level service credential.
- Health endpoints `/internal/health/live` и `/internal/health/ready` не требуют OpenWebUI credential.
- Все endpoints под `/v1/*` требуют OpenWebUI service credential.

## 2. Service credential

OpenWebUI передаёт credential стандартным OpenAI-compatible способом:

```http
Authorization: Bearer <OPENWEBUI_INTERNAL_SECRET>
```

Sonata:

1. загружает credential только через logical secret reference `openwebui_internal_secret`;
2. останавливает startup, если credential не разрешён;
3. сравнивает credential без обычного string comparison;
4. не пишет credential в logs, traces, errors или responses;
5. возвращает `401` до чтения forwarded identity при неверном credential.

Пустой credential не включает режим без authentication.

## 3. Forwarded identity

После успешной service authentication Sonata принимает стандартные OpenWebUI headers:

| Header | Значение |
|---|---|
| `X-OpenWebUI-User-Id` | стабильный OpenWebUI user ID |
| `X-OpenWebUI-Chat-Id` | стабильный chat/conversation ID |
| `X-OpenWebUI-Message-Id` | стабильный ID текущего сообщения |

Для `POST /v1/chat/completions` обязательны все три ID.

Правила:

- forwarded headers никогда не читаются как trusted metadata до проверки Bearer credential;
- ID не могут быть пустыми, длиннее 256 bytes или содержать whitespace/control characters;
- user name, email и role не используются как authorization source;
- прямой HTTP-клиент не может выдать себя за другого пользователя одной подстановкой `X-OpenWebUI-*` headers;
- validated identity помещается в request context и передаётся в `ChatService` как typed `RequestIdentity`.

Signed OpenWebUI user JWT может быть добавлен позднее как дополнительная защита identity. Он не отменяет обязательный service credential mini MVP.

## 4. Chat transport

`POST /v1/chat/completions` поддерживает:

- OpenAI-compatible JSON request;
- non-streaming `chat.completion` response;
- SSE `chat.completion.chunk` response при `stream: true`;
- первый chunk с `assistant` role;
- content delta chunks;
- финальный chunk с `finish_reason`;
- терминатор `data: [DONE]`.

SSE response использует:

```http
Content-Type: text/event-stream
Cache-Control: no-cache
X-Accel-Buffering: no
```

Каждый event немедленно flush-ится через `http.Flusher`.

## 5. Cancellation

Request context передаётся в `ChatService` без замены на background context.

Отмена соединения или timeout:

1. отменяет context transport layer;
2. должна быть передана будущему cognitive pipeline и provider requests;
3. прекращает дальнейшую запись SSE chunks.

Полный checklist-пункт cancellation закрывается только после подключения и проверки cognitive pipeline, а не только HTTP adapter.

## 6. OpenWebUI configuration

Для соединения с Sonata должны быть настроены:

- OpenAI-compatible base URL private Sonata API;
- API key со значением `OPENWEBUI_INTERNAL_SECRET`;
- forwarding user information headers;
- forwarding session chat/message IDs;
- модель `sonata` как отдельное подключение от direct provider models.

Отключение встроенной memory OpenWebUI и deployment private service проверяются отдельно при Render/OpenWebUI integration acceptance.
