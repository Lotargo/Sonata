# Relationship response

> Статус: accepted contract для mini MVP stage 07B  
> Rule ID: `relationship-response-v1`  
> Profile: `sonata-affective-v1.0.0`  
> Основной design contract: [`AFFECTIVE_DYNAMICS.md`](./AFFECTIVE_DYNAMICS.md)

## 1. Назначение

Relationship state должна менять восприятие следующего пользовательского стимула, а не только обновляться после уже рассчитанной эмоциональной реакции.

Одинаковый typed stimulus при одинаковых personality, physiology, drives и complex states должен давать разный emotional response для двух состояний с разными отношениями к пользователю.

Правило остаётся:

- детерминированным;
- bounded;
- versioned;
- независимым от LLM;
- изолированным по `identity_id + user_id`;
- неспособным менять security policy, protected instructions или memory facts.

## 2. Граница применения

`relationship-response-v1` применяется только к direct emotion effects текущего `StimulusDefinition`.

Порядок внутри обработки одного stimulus:

```text
1. read relationship snapshot before the stimulus
2. calculate personality response
3. calculate physiology response
4. calculate relationship response
5. calculate drive and active complex-state modifiers
6. apply bounded direct emotion delta
7. apply cross-emotion interactions
8. update relationship from the stimulus
```

Relationship effects текущего stimulus не могут усилить или ослабить этот же stimulus. Они влияют только на последующие события.

Cross-emotion interactions не получают отдельный relationship multiplier. Они работают с уже изменённым bounded emotion vector.

## 3. Производные сигналы

Из текущего `RelationshipState` вычисляются два bounded сигнала.

```text
support = mean(
  attachment,
  openness,
  confidence_in_user,
  perceived_safety
)

strain = 0.60 * tension + 0.40 * unresolved_hurt
```

Оба значения ограничиваются диапазоном `[0, 1]`.

Нейтральная точка support равна `0.50`. Нулевая strain считается отсутствием накопленного relational threat.

## 4. Per-emotion rule

Для эмоционального канала `e`:

```text
relationship_response(e) = clamp(
    1
  + support_weight(e) * (support - 0.50)
  + strain_weight(e) * strain,
  0.50,
  1.50
)
```

Коэффициенты `relationship-response-v1`:

| Emotion | support_weight | strain_weight | Смысл |
|---|---:|---:|---|
| joy | `+0.30` | `-0.20` | поддерживающие отношения усиливают положительную реакцию |
| trust | `+0.45` | `-0.45` | доверие наиболее чувствительно к качеству отношений |
| anticipation | `+0.15` | `-0.10` | поддержка умеренно усиливает положительное ожидание |
| surprise | `+0.05` | `0.00` | surprise почти нейтральна к отношениям |
| fear | `-0.25` | `+0.40` | безопасность ослабляет, strain усиливает threat response |
| anger | `-0.20` | `+0.45` | накопленное напряжение сильнее усиливает anger response |
| disgust | `-0.15` | `+0.35` | strain усиливает защитное отторжение |
| sadness | `-0.20` | `+0.30` | поддержка смягчает, unresolved strain усиливает sadness |

Таблица является частью versioned profile behavior. Изменение коэффициентов, clamp bounds, neutral point, порядка операторов или формулы требует нового rule ID и осознанного profile version bump.

## 5. Инварианты

- Modifier всегда конечен и находится в `[0.50, 1.50]`.
- Relationship state не меняет знак stimulus effect.
- Нулевой direct effect остаётся нулевым.
- Relationship state не обходит per-emotion max delta и ceiling.
- Высокий support не может полностью обнулить negative response.
- Высокий strain не может вывести response за bounded delta.
- Один пользователь не может влиять на relationship response другого пользователя.
- Одинаковые inputs дают одинаковый modifier и transition result.

## 6. Обязательные tests

До закрытия relationship increment нужны:

1. Один `user_warmth` stimulus создаёт больший joy/trust delta при высоком support, чем при низком support.
2. Один `user_hostility` stimulus создаёт меньший fear/anger delta при высоком support и низком strain, чем при низком support и высоком strain.
3. Relationship effects текущего stimulus не меняют его собственный response.
4. Modifier соблюдает bounds на крайних значениях всех relationship fields.
5. Повтор transition с одинаковыми inputs даёт идентичный state и `TransitionLog`.
6. Golden trajectory `same stimulus from trusted and untrusted user` привязана к `relationship-response-v1` и `sonata-affective-v1.0.0`.

## 7. Acceptance

Relationship-response increment считается готовым, когда:

- rule реализован отдельным typed domain component;
- `Transition` использует relationship snapshot до обновления relationship;
- startup/runtime validation отвергает неизвестную или несовместимую rule version;
- semantic golden trajectory проходит;
- property и replay tests продолжают проходить;
- изменение коэффициентов невозможно провести незаметно без rule/profile version change.
