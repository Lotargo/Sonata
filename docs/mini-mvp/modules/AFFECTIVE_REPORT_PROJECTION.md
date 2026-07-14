# Canonical affective report projection

> Статус: implemented and verified; полный CI подтверждён на head `0e43a04a537c24c1bbd905c0751365071d530f16`  
> Contract: `sonata-emotion-report-v1`  
> Affective profile: `sonata-affective-v1.0.0`  
> Parent boundary: [`EMOTION_MODULE.md`](./EMOTION_MODULE.md)

## 1. Назначение

Один cognitive run использует один immutable `cognition.EmotionReport`.

Report создаётся до cognitive pipeline и не пересчитывается отдельно для разных ролей. Все разрешённые роли получают одинаковые `Text` и `StateVersion` под одним contract identifier.

```go
const EmotionReportContractVersion = "sonata-emotion-report-v1"

type EmotionReport struct {
    Text         string
    StateVersion int64
}
```

`ContractVersion()` возвращает идентификатор схемы, а `Validate()` отвергает пустой text и отрицательную state version.

## 2. Projection matrix

| Boundary | Получает report | Причина |
|---|---:|---|
| Router | нет | Маршрутизация не должна зависеть от affective state |
| Raw, 5 ролей | да | Affective context участвует в первичном рассмотрении |
| Critical, 5 ролей | да | Критическая проверка учитывает ту же state version |
| Summary, 5 ролей | нет | Summary обобщает Raw и Critical своей призмы и не получает отдельную affective инъекцию |
| Synthesis tooling | да | Выбор инструментов учитывает canonical report |
| Synthesis final | да | Финальный ответ использует тот же report в direct и full route |

Для full route report проецируется ровно в 12 role inputs: пять Raw, пять Critical и два Synthesis.

## 3. Invariants

- `RouterInput` и `SummaryInput` структурно не содержат `Emotion` или `EmotionReport`.
- `RawInput`, `CriticalInput`, `SynthesisToolingInput` и `SynthesisFinalInput` используют один тип `EmotionReport`.
- Direct route передаёт report только в `SynthesisFinalInput`; Router его не видит.
- Full route передаёт один и тот же value во все разрешённые роли.
- Pipeline и публичные direct/full entrypoints выполняют fail-fast validation до LLM role calls.
- Report не содержит raw message text, event history, secrets или protected instructions.

## 4. Verification

Regression tests проверяют:

- canonical contract identifier;
- invalid empty/negative reports;
- structural exclusion Router и Summary;
- точное равенство report во всех разрешённых full-route ролях;
- direct Synthesis получает report без full-route Context, Dialogue или ToolResults.

Полный repository CI подтверждён на head `0e43a04a537c24c1bbd905c0751365071d530f16`; increment принят.

## 5. Граница текущего increment

Этот increment фиксирует cognitive contract и projection topology. Он не извлекает stimuli из HTTP request, не загружает affective state и не сохраняет transition.

Подключение affective module к HTTP request flow остаётся отдельным пунктом stage 07B.