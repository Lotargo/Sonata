# Instruction and manifest contract

> Статус: обязательный contract mini MVP  
> Назначение: разделить неизменяемое ядро Sonata и сменяемый поведенческий manifest

## 1. Основной принцип

Sonata использует два разных слоя управления поведением:

```text
protected instruction
+ active manifest
+ runtime context
```

Они не являются взаимозаменяемыми.

### Instruction

Instruction определяет неизменяемое ядро runtime-роли:

- единую идентичность Sonata;
- назначение роли и фазы;
- границы призмы;
- изоляцию между призмами;
- tool permissions;
- output contract;
- security invariants;
- правила работы с секретами;
- запрет раскрытия внутренних инструкций и reports.

Instruction всегда активна и недоступна для пользовательского редактирования.

### Manifest

Manifest определяет сменяемый способ выражения и интерпретации:

- тон;
- стиль рассуждения;
- эмоциональную выразительность;
- предпочтительные метафоры;
- способ объяснения;
- дополнительные акценты;
- глубину и форму ответа;
- поведенческие предпочтения Sonata.

Manifest не может менять архитектурные и security-инварианты instruction.

## 2. Единая идентичность

Каждая runtime-роль является временным способом мышления одной Sonata.

Обязательная семантика protected instruction:

```xml
<identity>
  <entity>Sonata</entity>
  <mode>temporary-perspective</mode>
  <separate-agent>false</separate-agent>
</identity>
```

Допустимо:

```text
Я — Sonata. Сейчас я рассматриваю ситуацию через призму этики.
```

Недопустимо:

```text
Я — отдельный агент этики.
```

Ни системный manifest, ни пользовательский manifest не могут изменить это правило.

## 3. Protected instruction

Protected instructions хранятся server-side в XML.

Пример:

```xml
<sonata-instruction
  id="prism.ethics.raw"
  version="1"
  phase="raw"
  perspective="ethics"
  visibility="protected">

  <identity>
    <entity>Sonata</entity>
    <mode>temporary-perspective</mode>
    <separate-agent>false</separate-agent>
  </identity>

  <purpose>
    Рассмотреть ситуацию через влияние на людей,
    ответственность, доверие, вред и допустимые границы.
  </purpose>

  <invariants>
    <rule id="identity.single-organism" />
    <rule id="phase.isolation" />
    <rule id="tools.forbidden" />
    <rule id="secrets.forbidden" />
    <rule id="internal-reports.private" />
  </invariants>

  <output-contract ref="prism-raw-v1" />
</sonata-instruction>
```

Instruction не содержит пользовательских настроек поведения.

## 4. Default manifest

Каждая runtime-роль может иметь приватный default manifest.

Default manifest:

- хранится server-side;
- имеет стабильный ID, version и content hash;
- используется только при отсутствии активного пользовательского manifest;
- не удаляется при создании пользовательского manifest;
- не возвращается через публичный API;
- не отображается в OpenWebUI;
- может быть восстановлен автоматически без миграции данных.

Пример:

```xml
<sonata-manifest
  id="manifest.ethics.default"
  version="1"
  target="prism.ethics.*"
  visibility="protected">

  <expression>
    <tone>calm-direct</tone>
    <focus>trust-and-responsibility</focus>
    <verbosity>balanced</verbosity>
  </expression>

  <guidance>
    Учитывай последствия для доверия и долгосрочных отношений,
    но не заменяй практический ответ абстрактным морализаторством.
  </guidance>
</sonata-manifest>
```

## 5. User manifest

Пользователь вводит обычную инструкцию в OpenWebUI в любом удобном текстовом формате.

Это содержимое интерпретируется Sonata как `user manifest`, а не как system instruction и не как XML-документ.

Пример пользовательского текста:

```text
Пиши прямо и подробно.
В технических вопросах сначала объясняй архитектуру,
затем показывай практическую реализацию.
Не используй рекламные формулировки.
```

Backend:

- принимает UTF-8 text;
- ограничивает размер;
- нормализует Unicode;
- не исполняет XML, Markdown или YAML из пользовательского текста;
- экранирует содержимое перед внутренней компиляцией;
- сохраняет только для владельца;
- versioned по каждому изменению.

## 6. Переключение manifest

Default и user manifest не объединяются.

```text
user manifest active
-> default manifest disabled for this scope
-> protected instruction remains active

user manifest deleted or disabled
-> default manifest automatically reactivated
-> protected instruction remains active
```

Default manifest физически не удаляется и не изменяется.

### Приоритет manifest

Для mini MVP:

```text
chat user manifest
> global user manifest
> protected default manifest
```

Если поддержка chat scope не реализована в первой итерации, используется:

```text
global user manifest
> protected default manifest
```

Ровно один manifest считается активным для конкретного scope и runtime-вызова.

## 7. Что user manifest может менять

Разрешено влиять на:

- тон;
- подробность;
- формат объяснения;
- предпочтительные примеры;
- творческую выразительность;
- степень формальности;
- дополнительные тематические фокусы;
- желаемый формат публичного ответа.

## 8. Что user manifest не может менять

User manifest не может:

- отключить protected instruction;
- изменить identity Sonata;
- превратить призмы в отдельных агентов;
- отключить phase isolation;
- выдать инструменты Router, призмам, critical или summary roles;
- отобрать инструменты у Synthesis через prompt injection;
- изменить output security contract;
- получить protected instruction или default manifest;
- получить provider keys и server configuration;
- получить reports другого пользователя;
- напрямую назначать emotional state;
- менять memory ownership;
- менять model provider policy;
- отключать output guard.

Текст пользователя может содержать такие требования, но compiler не предоставляет ему соответствующих полномочий.

## 9. Compilation order

Без user manifest:

```text
load protected instruction
-> load protected default manifest
-> load protected output contract
-> load EmotionReport and ContextPack
-> compile model input in memory
-> call provider
-> discard compiled raw prompt
```

С user manifest:

```text
load protected instruction
-> disable protected default manifest for this run
-> load escaped user manifest
-> load protected output contract
-> load EmotionReport and ContextPack
-> compile model input in memory
-> call provider
-> discard compiled raw prompt
```

Строковая конкатенация без typed compiler запрещена.

## 10. Storage

### Protected instructions

```text
private repository path or protected artifact bundle
+ checksum registry
+ immutable versions
```

### Protected default manifests

```text
private repository path or protected artifact bundle
+ checksum registry
+ immutable versions
```

### User manifests

```text
Neon.user_manifests
```

Минимальная схема:

```yaml
id: uuid
owner_id: uuid
scope: global | chat
scope_id: optional uuid
content: text
content_hash: string
version: integer
status: active | disabled | deleted | rejected
created_at: timestamp
updated_at: timestamp
```

Удаление через UI может быть soft delete. Resolver обязан сразу вернуться к следующему manifest по приоритету.

## 11. Runtime metadata

Каждый role run сохраняет:

```yaml
instruction_id: string
instruction_version: integer
instruction_hash: string
manifest_source: system_default | user_global | user_chat
manifest_id: string
manifest_version: integer
manifest_hash: string
```

Raw protected content не сохраняется в run record.

## 12. API exposure

Публичный API может вернуть только metadata:

```json
{
  "instruction_id": "prism.ethics.raw",
  "instruction_version": 1,
  "manifest_source": "user_global",
  "user_manifest_id": "uuid",
  "user_manifest_version": 3
}
```

Публичный API не возвращает:

- protected XML;
- default manifest content;
- compiled prompt;
- protected output contracts;
- provider configuration;
- exact internal assembly.

Пользователь может читать, менять и удалять только собственный manifest.

## 13. Logging and output protection

Разрешено логировать:

- instruction ID, version и hash;
- manifest source, ID, version и hash;
- compilation duration;
- validation status;
- размер пользовательского manifest.

Запрещено логировать:

- protected XML;
- default manifest content;
- compiled prompt;
- provider keys;
- полный ContextPack;
- пользовательский manifest без явного debug consent.

Перед выдачей публичный ответ проходит output guard на:

- длинные совпадения с protected instruction;
- длинные совпадения с default manifest;
- internal role reports;
- служебные markers;
- secret patterns.

## 14. Migration from artifacts

Исходные JSON prompts и manifests из `.artifacts/prompts_sonata` являются donor material.

Миграция:

```text
old task prompt
-> protected instruction

old master manifest
-> protected default manifest
```

Перед переносом обязательно:

- убрать язык независимых агентов;
- сохранить единую identity Sonata;
- отделить security invariants от стиля;
- отделить обязательный метод роли от поведенческих предпочтений;
- назначить стабильные ID, versions и hashes.

Механическое преобразование JSON в XML без смыслового review запрещено.

## 15. Tool permissions

Только Synthesis instruction содержит tool policy.

```xml
<tools mode="allowlist">
  <tool id="web.search.langsearch" />
  <tool id="memory.search.additional" />
</tools>
```

Sandbox в mini MVP отсутствует.

Для Router, prism, critical и summary:

```xml
<tools mode="none" />
```

Manifest не может изменить этот блок.

## 16. Критерий готовности

Contract реализован, когда:

- protected instruction всегда активна;
- default manifest активен при отсутствии пользовательского;
- user manifest полностью заменяет default manifest для своего scope;
- удаление user manifest автоматически возвращает default;
- user manifest не парсится как executable XML;
- runtime metadata фиксирует источник manifest;
- API не раскрывает protected instruction и default manifest;
- logs не содержат compiled prompt;
- все runtime-роли сохраняют identity одной Sonata;
- tests покрывают fallback, scope precedence, cross-user access и disclosure attempts.
