# Affective HTTP request integration

> Статус: verified  
> Full CI подтверждён на head: `5edf3bdbd70539aef399d8062cea138d69cdda29`  
> Affective profile: `sonata-affective-v1.0.0`  
> Report contract: `sonata-emotion-report-v1`

## 1. Назначение

Этот increment подключает deterministic affective dynamics v1 к OpenAI-compatible HTTP chat boundary.

Поток одного запроса:

```text
trusted OpenWebUI identity
-> HTTP ChatRequest
-> latest plain-text user message extraction
-> owner-scoped affective v1 transition
-> optimistic CAS persistence
-> one safe canonical report
-> CognitiveChatRequest
-> cognitive chat implementation
```

`internal/application.AffectiveChatService` реализует `httpapi.ChatService`. HTTP transport не знает внутреннюю математику affective module, а downstream cognitive service получает явный typed `cognition.EmotionReport` вместе с trusted identity и сообщениями.

## 2. State ownership

Canonical key остаётся:

```text
sonata identity ID + trusted OpenWebUI user ID
```

`chat_id` и `message_id` не являются частью affective state key. Они остаются transport metadata и не позволяют разделить или подменить состояние одного пользователя.

Forwarded user ID используется только после проверки internal service credential существующим HTTP middleware.

## 3. Request processing

Для affective lexical extraction используется последнее сообщение с role `user`, если его content является JSON string.

- raw text передаётся extractor только в памяти текущего вызова;
- raw text не сохраняется в state, store, report или metadata;
- structured или multimodal content сохраняется для cognitive service, но не интерпретируется lexical extractor как affective signal;
- отсутствие lexical markers допустимо: runtime выполняет elapsed-time transition или возвращает текущую state version.

Один вызов `AffectiveChatService.Complete` создаёт ровно один report до вызова downstream cognitive service.

## 4. V1 runtime и store

`internal/emotion.AffectiveRuntime` использует:

- `AffectiveRuntimeProfile`;
- pure deterministic `Transition`;
- owner-scoped `AffectiveState`;
- `AffectiveStateStore` с optimistic compare-and-swap;
- bounded retry при version conflict;
- safe `AffectiveReport` без raw events, evidence accumulators, definition IDs и внутренних коэффициентов.

`MemoryAffectiveStateStore` является временной mini-MVP implementation до canonical Neon repository этапа 08. Он сохраняет ту же CAS boundary, которую должна реализовать PostgreSQL версия.

## 5. Graceful degradation

Storage или transition failure не превращается автоматически в отказ всего chat request.

```text
affective processing error
-> do not overwrite canonical state
-> build validated baseline report
-> status=DEGRADED
-> state_version=0
-> continue cognitive chat
```

Отмена request context является исключением: cancellation немедленно возвращается наверх и не маскируется degraded fallback.

## 6. Projection

Application adapter преобразует safe `emotion.AffectiveReport` в один:

```go
type cognition.EmotionReport struct {
    Text         string
    StateVersion int64
}
```

Дальнейшая projection topology остаётся неизменной:

- Router report не получает;
- Raw и Critical получают один и тот же report;
- Summary не получает отдельный report;
- direct и full Synthesis получают тот же canonical value.

## 7. Verification

Tests проверяют:

- HTTP request с trusted forwarded identity проходит affective transition до cognition;
- первое meaningful событие создаёт state version 1;
- следующий запрос того же пользователя создаёт version 2;
- другой пользователь начинает с независимой version 1;
- raw message text отсутствует в report;
- concurrent updates сохраняют CAS version sequence;
- store failure возвращает cognitive service validated `DEGRADED` report;
- request cancellation не вызывает downstream cognition;
- HTTP response остаётся доступным при affective degradation.

Полный CI для этого increment подтверждён на head `5edf3bdbd70539aef399d8062cea138d69cdda29`.

## 8. Не входит в increment

- canonical Neon persistence;
- запись raw message text в affective tables;
- LLM classification affective hints;
- самостоятельные фоновые transitions;
- response и tool outcome events, которые подключаются отдельными typed event producers.

Пункт 07B закрыт после зелёного полного CI на implementation head `5edf3bdbd70539aef399d8062cea138d69cdda29`.
