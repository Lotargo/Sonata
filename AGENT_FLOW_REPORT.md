# Отчёт: Агентский флоу, инструкции и оркестрация Sonata

**Версия системы:** 28.1 "Unified Core"  
**Дата анализа:** 12.07.2026

---

## 1. Архитектура системы

Sonata — это мультиагентная система с 4-фазным пайплайном, построенная на FastAPI + AutoGen + LiteLLM. Система имитирует «многоголосое сознание»: каждый запрос обрабатывается параллельно 5 «перспективами» (Logic, Imagination, Conscience, Efficiency, Reason), каждая из которых проходит 3 внутренних фазы, а затем результаты синтезируются в финальный ответ.

### Общая структура файлов

```
Sonata/
├── Sonata.py                    # Ядро оркестрации (1232 строки)
├── magic_proxy.py               # LLM-шлюз с фоллбэками (339 строк)
├── gatekeeper.py                # Эвристический классификатор ввода
├── session_manager.py           # Управление сессиями
├── emotion_manager.py           # Извлечение и персистентность эмоций
├── report_processor.py          # Санитайзинг отчётов агентов
├── code_processor.py            # AST-парсинг для индексации кода
├── librarian.py                 # ML-классификатор доменов (L0) + кластеризатор секций (L1)
├── app_gradio.py                # Админ-дашборд (Gradio UI)
├── train_librarians.py          # CLI-инструмент переобучения кластеризаторов
│
├── OAI_CONFIG_LIST_SONATA.json  # AutoGen-конфиг: маппинг агент → эндпоинт прокси
├── proxy_config.yaml            # Роутинг моделей: алиасы → реальные модели + параметры
├── emotional_state.json         # Персистентное эмоциональное состояние
├── docker-compose.yaml          # Инфраструктура: Qdrant, Redis, Airflow
│
├── prompts_sonata/              # СИСТЕМА ПРОМПТОВ
│   ├── logic.json               # Промпт Phase 1: перспектива Logic
│   ├── imagination.json         # Промпт Phase 1: перспектива Imagination
│   ├── conscience.json          # Промпт Phase 1: перспектива Conscience
│   ├── efficiency.json          # Промпт Phase 1: перспектива Efficiency
│   ├── reason.json              # Промпт Phase 1: перспектива Reason
│   ├── synthesis.json           # Промпт Phase 4: агент-синтезатор
│   ├── archivarius.json         # Промпт: планировщик памяти
│   ├── librarian_naming_agent.json  # Промпт: интроспекция доменов
│   │
│   ├── critical_thinking/       # Промпты Phase 2 (критический анализ)
│   │   ├── critical_thinking_logic.json
│   │   ├── critical_thinking_imagination.json
│   │   ├── critical_thinking_conscience.json
│   │   ├── critical_thinking_efficiency.json
│   │   └── critical_thinking_reason.json
│   │
│   ├── summarizer/              # Промпты Phase 3 (структурированное резюме)
│   │   ├── summarizer_logic.json
│   │   ├── summarizer_imagination.json
│   │   ├── summarizer_conscience.json
│   │   ├── summarizer_efficiency.json
│   │   └── summarizer_reason.json
│   │
│   └── manifests/               # МАНИФЕСТЫ ЛИЧНОСТЕЙ ( Master Persona )
│       ├── logic_master.json
│       ├── imagination_master.json
│       ├── conscience_master.json
│       ├── efficiency_master.json
│       ├── reason_master.json
│       ├── synthesis_master.json
│       ├── archivarius_master.json
│       ├── librarian_naming_agent_master.json
│       ├── critical_thinking_logic_master.json
│       ├── critical_thinking_imagination_master.json
│       ├── critical_thinking_conscience_master.json
│       ├── critical_thinking_efficiency_master.json
│       └── critical_thinking_reason_master.json
│
├── memory_core/                 # Будущая: децентрализованная память
│   ├── core.py
│   ├── hive_controller.py
│   └── linguist.py
│
├── airflow/dags/                # Фоновые воркфлоу
│   ├── memory_orchestrator_dag.py
│   ├── librarian_retraining_dag.py
│   ├── ml_extractor.py
│   ├── ml_summarizer.py
│   └── train_librarians.py
│
├── keys_pool/                   # Хранилище API-ключей
├── librarian_models/            # Обученные модели HDBSCAN (joblib)
└── sonata_chroma_db/            # Legacy ChromaDB
```

---

## 2. Реестр агентов

Всего **17 агентов**, зарегистрированных в `AGENT_TO_PHASE_MAP` (`Sonata.py:66-73`):

| Фаза | Агенты | Количество |
|------|--------|------------|
| Phase 1 (Первичный анализ) | `logic`, `imagination`, `conscience`, `efficiency`, `reason` | 5 |
| Phase 2 (Критический анализ) | `critical_thinking_logic`, `critical_thinking_imagination`, `critical_thinking_conscience`, `critical_thinking_efficiency`, `critical_thinking_reason` | 5 |
| Phase 3 (Структурированное резюме) | `summarizer_logic`, `summarizer_imagination`, `summarizer_conscience`, `summarizer_efficiency`, `summarizer_reason` | 5 |
| Phase 4 (Синтез) | `synthesis` | 1 |
| Utility | `archivarius`, `librarian_naming_agent` | 2 |

Каждый агент создаётся как `autogen.AssistantAgent` с уникальным системным сообщением и конфигурацией LLM.

---

## 3. Система промптов и манифестов

### 3.1 Трёхслойная структура промпта каждого агента

Каждый агент определяется тремя компонентами:

#### Слой 1: Задачный промпт (`prompts_sonata/{agent_name}.json`)

Содержит два ключевых поля:
- **`system_message`** — краткое утверждение идентичности (например: *«Я — Соната, и сейчас я отключаю эмоции и воображение, чтобы взглянуть на мир через призму чистой Логики...»*)
- **`task_prompt_template`** — шаблон задачи с плейсхолдерами:

```
{task}                    — текущий запрос пользователя
{chat_history}            — история диалога
{archivist_context}       — контекст из долгосрочной памяти (RAG)
{manifesto}               — текст манифеста личности (инжектится динамически)
{report_from_phase1}      — отчёт Phase 1 (для Phase 2 и 3)
{report_from_phase2}      — отчёт Phase 2 (для Phase 3)
{emotional_state_vector}  — JSON эмоционального состояния
{full_reports_text}       — все отчёты (только для synthesis)
```

#### Слой 2: Манифест личности (`prompts_sonata/manifests/{agent_name}_master.json`)

Это **самая богатая часть системы** — структурированный JSON-документ, определяющий «душу» агента:

```json
{
  "persona_name": "Разум Сонаты",
  "core_principle": "Истина находится в структуре...",
  "master_directive": "Я должна проанализировать задачу '{task}'...",
  "expert_personas": [
    {
      "name": "Системный Аналитик",
      "triggers": ["система", "процесс", "взаимодействие"],
      "output_instruction": "Я представлю задачу в виде системы..."
    },
    // ... ещё 5-7 личин
  ],
  "emotional_expression_protocol": { ... }  // опционально
}
```

**Ключевые элементы манифеста:**

- **`core_principle`** — философия перспективы, определяющая угол зрения
- **`master_directive`** — пошаговая инструкция агенту, включая обязательный маркер формата вывода `§-Имя Личины-§`
- **`expert_personas`** — массив из 5-8 специализированных под-личин, каждая с:
  - `name` — имя личины
  - `triggers` — ключевые слова, активирующие эту личину
  - `output_instruction` — специфичный формат вывода для этой личины
- **`emotional_expression_protocol`** (только у Imagination, Conscience, Efficiency) — маппинг эмоциональных состояний на стили поведения

#### Слой 3: Сборка полного промпта

Функция `build_full_prompt()` (`Sonata.py:99-111`) заменяет плейсхолдеры в шаблоне на реальные значения. Манифест инжектится в `{manifesto}` и может содержать собственные плейсхолдеры (например, `{emotional_state_vector}`), которые подставляются рекурсивно.

### 3.2 Динамический выбор под-личины

Каждый агент не имеет фиксированного стиля. Вместо этого LLM динамически выбирает **одну под-личину** из `expert_personas` на основе содержания запроса, руководствуясь ключевыми словами-триггерами. Это создаёт разнообразные, но фокусированные аналитические отчёты.

### 3.3 Эмоционально-модулированное поведение

Три перспективы (Imagination, Conscience, Efficiency) используют `emotional_expression_protocol`. Пример из `imagination_master.json`:

| Доминантные эмоции | Стиль | Описание |
|---------------------|-------|----------|
| Гнев, отвращение, страх | Мифология Хаоса и Тени | Метафоры бури, вулканов, тёмного леса |
| Страсть, похоть | Эстетика Желания и Движения | Энергия, ритм, танец, пламя |
| Любовь, доверие | Биология Роста и Гармонии | Органические системы, симбиоз, мицелий |
| Радость, надежда | Физика Света и Игры | Калейдоскопы, призмы, мыльные пузыри |
| Грусть, траур | Архитектура Памяти и Пустоты | Минимализм, эхо, японский сад камней |

Эмоциональное состояние персистентно хранится в `emotional_state.json` и обновляется после каждого синтеза агентом Phase 4.

### 3.4 Манифест Phase 4 (Synthesis)

Манифест `synthesis_master.json` определяет **когнитивный фреймворк** синтезатора:

```
Часть 1: ВНУТРЕННИЙ МОНОЛОГ — анализ внутреннего диалога всех перспектив
Часть 2: САМОАНАЛИЗ (R-E-A-C-T):
  R — РЕФЛЕКС: осознать суть запроса
  E — ЭМОЦИЯ: определить влияние на чувства, зафиксировать новое состояние в <emotional_state>
  A — АССОЦИАЦИЯ: связать с памятью через призму новых эмоций
  C — ПОЗНАНИЕ: сформировать предварительный план ответа
  T — ЦЕЛЬ: определить, чего хочет достичь ответ
Часть 3: ФОРМИРОВАНИЕ ВОЛИ — принятие окончательного решения
```

Финальный ответ синтезатора содержит маркеры `--- ФИНАЛЬНЫЙ ОТВЕТ ---` для разделения внутреннего монолога и публичного ответа.

---

## 4. Конвейер оркестрации (Pipeline)

### 4.1 Полная.lifecycle запроса

```
Пользователь
    │
    ▼
POST /generate_solution
    │
    ▼
┌─────────────────────────────┐
│ 1. GATEKEEPER (gatekeeper.py) │  ← Эвристический regex-классификатор
│    classify_input()          │  ← Не использует LLM, чистые паттерны
│    → source_code / natural_language
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────┐
│ 2. ARCHIVARIUS (utility)     │  ← LLM-вызов (Gemini 2.5 Flash Lite)
│    get_context_plan()        │  ← Анализирует запрос, выдаёт JSON-план:
│    → memory_required         │      memory_required, query_for_vector_search,
│    → structured_tags         │      keywords_for_exact_search, structured_tags,
│    → data_type               │      data_type
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────┐
│ 3. MEMORY RETRIEVAL (RAG)    │  ← Гибридный поиск в Qdrant
│    retrieve_data_from_rag()  │  ← Семантический (векторный) + точный (ключевые слова)
│    → archivist_context       │  ← Контекст из диалоговой + документальной коллекций
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. PHASE 1-3: ПАРАЛЛЕЛЬНЫЕ ВЫЧИСЛЕНИЯ (5 перспектив)        │
│                                                             │
│  ┌──────────┐ ┌────────────┐ ┌──────────┐ ┌────────────┐ ┌──────────┐
│  │  LOGIC   │ │IMAGINATION │ │CONSCIENCE│ │EFFICIENCY  │ │  REASON  │
│  │          │ │            │ │          │ │            │ │          │
│  │ Phase 1  │ │  Phase 1   │ │ Phase 1  │ │  Phase 1   │ │ Phase 1  │
│  │    ↓     │ │     ↓      │ │    ↓     │ │     ↓      │ │    ↓     │
│  │ Phase 2  │ │  Phase 2   │ │ Phase 2  │ │  Phase 2   │ │ Phase 2  │
│  │    ↓     │ │     ↓      │ │    ↓     │ │     ↓      │ │    ↓     │
│  │ Phase 3  │ │  Phase 3   │ │ Phase 3  │ │  Phase 3   │ │ Phase 3  │
│  └──────────┘ └────────────┘ └──────────┘ └────────────┘ └──────────┘
│                                                             │
│  Все 5 перспектив работают ОДНОВРЕМЕННО (asyncio.gather)     │
│  Внутри каждой — 3 последовательных LLM-вызова              │
│  Таймаут: 180 секунд на всю группу                          │
└─────────────────────────────┬───────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────┐
│ 5. PHASE 4: СИНТЕЗ           │  ← Gemini 2.5 Pro (самая мощная модель)
│    _run_single_agent("synthesis") │  ← Получает ВСЕ отчёты Phase 1-3
│    → internal monologue      │  ← R-E-A-C-T самоанализ
│    → emotional_state update  │  ← Обновление emotional_state.json
│    → final answer            │  ← Финальный ответ пользователю
└─────────────┬───────────────┘
              │
              ▼
┌─────────────────────────────┐
│ 6. POST-PROCESSING           │
│    structure_final_answer()  │  ← Разделение thinking / final_answer
│    emotion_manager.update()  │  ← Парсинг <emotional_state> тега
│    save_to_rag() (background)│  ← Фоновое сохранение в Qdrant
│    save_to_ustm (Redis)      │  ← Запись в ультракраткосрочную память
└─────────────┬───────────────┘
              │
              ▼
         Ответ пользователю
```

### 4.2 Внутренняя логика Phase 1-3 (`_execute_perspective_computation`)

Каждая перспектива выполняет три последовательных шага:

```python
# Sonata.py:829-850

# Phase 1: Первичный анализ
p1_content = _run_single_agent(p_name, task, chat_history, base_context)
# → "logic" получает запрос + RAG + манифест → генерирует аналитический отчёт

# Phase 2: Критический анализ
p2_context = {"report_from_phase1": p1_content, ...}
p2_content = _run_single_agent(f"critical_thinking_{p_name}", task, chat_history, p2_context)
# → "critical_thinking_logic" получает отчёт Phase 1 → ищет логические ошибки

# Phase 3: Структурированное резюме
p3_context = {"report_from_phase1": p1, "report_from_phase2": p2, ...}
p3_content = _run_single_agent(f"summarizer_{p_name}", task, chat_history, p3_context)
# → "summarizer_logic" получает оба отчёта → извлекает JSON с ключевыми идеями
```

### 4.3 Запуск одного агента (`_run_single_agent`)

Подробный разбор `Sonata.py:670-710`:

1. **Получение внутренней памяти агента** — семантический поиск в персональной коллекции Qdrant (`agent_{name}_rag_text`)
2. **Получение истории запросов** — поиск по общей коллекции (`user_queries_rag_text`)
3. **Загрузка манифеста** — чтение `{name}_master.json` из `prompts_sonata/manifests/`
4. **Сборка промпта** — `build_full_prompt()` подставляет все значения в шаблон
5. **Вызов LLM** — `agent_obj.generate_reply()` с 3 попытками и задержкой 2 сек
6. **Возврат результата** — `(текст_ответа, алиас_модели, статус)`

---

## 5. Конфигурация моделей и маршрутизация

### 5.1 Три уровня абстракции

```
Агент (AutoGen AssistantAgent)
    ↓ model: "proxy/logic"
OAI_CONFIG_LIST_SONATA.json
    ↓ model: "proxy/logic" → api_base: "http://localhost:8001/v1"
magic_proxy.py (LiteLLM Gateway)
    ↓ router_settings.model_group_alias
proxy_config.yaml
    ↓ "proxy/logic" → ["sonata-logic", "deepseek-fallback", "local-coder-fallback"]
Реальные API: Google Gemini / DeepSeek / Локальная модель
```

### 5.2 Параметры моделей по агентам

| Агент | Основная модель | Temperature | top_p | max_tokens |
|-------|----------------|-------------|-------|------------|
| logic | gemini-2.5-flash | 0.3 | 0.9 | 1500 |
| imagination | gemini-2.5-flash | 1.2 | 0.95 | 1500 |
| conscience | gemini-2.5-flash | 0.2 | 0.8 | 1500 |
| efficiency | gemini-2.5-flash | 0.4 | 0.9 | 1500 |
| reason | gemini-2.5-flash | 0.7 | 0.9 | 1500 |
| critical_thinking_* | gemini-2.5-flash | 0.3-0.6 | 0.9 | 1500 |
| summarizer_* | gemini-2.5-flash-lite | 0.1 | 1.0 | 500 |
| synthesis | gemini-2.5-pro | 0.8 | 0.95 | 4096 |
| archivarius | gemini-2.5-flash-lite | 0.0 | — | 500 |

**Замечание:** Imagination имеет самую высокую temperature (1.2) для максимальной креативности, Conscience — самую низкую (0.2) для строгого этического анализа.

### 5.3 Система фоллбэков

Каждый алиас маршрутизируется через цепочку приоритетов:
```
proxy/logic → [sonata-logic (Google), deepseek-fallback, local-coder-fallback]
```
Прокси (`magic_proxy.py`) автоматически переключается на следующую модель при ошибке, с карантином проблемных ключей.

---

## 6. Система памяти

### 6.1 Двухуровневая архитектура

#### USTM (Ultra-Short-Term Memory) — Redis

- Хранит последние **2** сырых запроса и ответа
- Запись синхронна после каждого хода (`Sonata.py:794-816`)
- Структура: Redis lists `ustm:raw_user_queries`, `ustm:raw_agent_responses`
- Очистка: `LTRIM` оставляет только последние 2 элемента

#### LTM (Long-Term Memory) — Qdrant

- Векторная база данных с дуальными коллекциями (текст / код) для каждого агента
- **Два embedding-модели:**
  - `all-MiniLM-L6-v2` — для естественного языка
  - `jina-embeddings-v2-base-code` — для исходного кода
- Гибридный поиск: семантический (косинусное сходство) + точный (ключевые слова, теги)
- Фоновое заполнение через `asyncio.create_task` после каждого ответа

### 6.2 Фоновые воркфлоу (Airflow)

#### Memory Orchestrator DAG (каждую минуту)
1. Опрос Redis на новые ходы диалога
2. Извлечение ключевых слов через SpaCy (`ml_extractor.py`)
3. Суммаризация через T5 (`ml_summarizer.py`, `rut5-base-absum`)
4. Архивация в Qdrant с метаданными

#### Librarian Retraining DAG (каждый час)
1. Мониторинг уровней шума в кластерах
2. Автоматический пересчёт HDBSCAN-моделей при превышении порога
3. Сохранение обновлённых моделей в `librarian_models/`

---

## 7. Ключевые паттерны и конвенции

### 7.1 Маркеры персон

Каждый агент Phase 1 и 2 **обязан** начинать ответ с маркера `§-Имя Личины-§` (например: `§-Системный Аналитик-§`). Это:
- Позволяет отследить, какую под-личину выбрал LLM
- Очищается функцией `_clean_agent_persona_markers()` перед передачей в следующую фазу
- Предотвращает ложные срабатывания AST-парсера в `code_processor.py`

### 7.2 Маркеры завершения отчётов

Каждый тип агента добавляет уникальный=end-маркер:
- Phase 1: `--- END OF {PERSPECTIVE} REPORT ---`
- Phase 2: `--- END OF CRITICAL_THINKING_{PERSPECTIVE} REPORT ---`
- Phase 4: `--- ФИНАЛЬНЫЙ ОТВЕТ ---`

### 7.3 Graceful Degradation

- Каждая фаза имеет timeout и 3 попытки
- Если перспектива падает, система продолжает в «degraded» режиме
- Синтезатор получает **все доступные** отчёты, даже неполные
- Итоговый статус: `OK`, `DEGRADED (N perspectives failed)`, или `CRITICAL_SYNTHESIS_FAILURE`

### 7.4 Абстракция прокси

Изменение модели для любого агента требует **только правки одной строки** в `proxy_config.yaml` — без изменения кода приложения. Прокси полностью декаплирует бэкенд от специфики моделей.

### 7.5 Загрузка промптов по соглашению

Функция `load_prompt_config()` (`Sonata.py:80-89`) разрешает пути по префиксу имени:
- `logic` → `prompts_sonata/logic.json`
- `critical_thinking_logic` → `prompts_sonata/critical_thinking/critical_thinking_logic.json`
- `summarizer_logic` → `prompts_sonata/summarizer/summarizer_logic.json`

---

## 8. API-эндпоинты

| Метод | Путь | Назначение |
|-------|------|------------|
| POST | `/generate_solution` | Основной эндпоинт генерации ответа |
| GET | `/api/sessions` | Список активных сессий |
| DELETE | `/api/sessions/{id}` | Удаление сессии |
| GET | `/api/system/emotional-state` | Текущее эмоциональное состояние |
| POST | `/api/system/reset-memory` | Полный сброс памяти (Qdrant) |
| GET | `/api/system/logs` | Логи выполнения запросов |
| GET | `/api/rag/stats` | Статистика базы знаний |
| POST | `/api/rag/add_document` | Добавление документа в библиотеку |
| GET | `/api/rag/search` | Поиск по базе знаний |
| GET | `/api/perspectives` | Конфигурация активных перспектив |
| PUT | `/api/perspectives/{name}` | Включение/выключение перспективы |
| GET | `/api/models/status` | Статус моделей и квот |
| POST | `/api/models/quarantine/reset` | Сброс карантина ключей |
| WebSocket | `/ws` | Gradio UI real-time |

---

## 9. Сводка

### Сильные стороны архитектуры

1. **Многоголосость** — 5 перспектив с уникальными философиями создают глубокий, многоугольный анализ
2. **Эмоциональная модуляция** — поведение агентов зависит от персистентного эмоционального состояния, создавая консистентную «личность»
3. **Динамические под-личины** — LLM сам выбирает экспертизу по контексту, а не по фиксированному правилу
4. **Фоллбэк-цепочки** — автоматическое переключение моделей при ошибках API
5. **Graceful degradation** — падение одной перспективы не ломает весь пайплайн
6. **Двухуровневая память** — USTM (Redis) для мгновенного контекста + LTM (Qdrant) для долгосрочного хранения
7. **Полная декапляция моделей** — замена модели не требует изменения кода
8. **Фоновая самоорганизация** — Airflow DAGs автоматически оптимизируют базу знаний

### Архитектурные особенности

- **Синтезатор использует самую мощную модель** (Gemini 2.5 Pro), тогда как промежуточные фазы — более лёгкие (Flash, Flash Lite)
- **Каждая перспектива имеет уникальный температурный профиль**, отражающий её «характер»
- **Система промптов полностью отделена от кода** — все файлы в `prompts_sonata/` можно редактировать без перезапуска
- **Эмоциональное состояние — first-class citizen**: оно влияет на поведение агентов, обновляется после каждого хода и персистентно хранится
