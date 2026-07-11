# Отчёт: Система памяти и Airflow в Sonata

**Версия системы:** 28.1 "Unified Core"  
**Дата анализа:** 12.07.2026

---

## 1. Архитектура памяти: три уровня

```
┌─────────────────────────────────────────────────────────┐
│                    USTM (Redis)                          │
│         Ультракраткосрочная рабочая память               │
│   • Последние 2 запроса + ответа (сырые)                 │
│   • Последние 10 саммари с ключевыми словами             │
│   • Без TTL — управление через LTRIM                     │
└────────────────────────┬────────────────────────────────┘
                         │ Airflow (каждую минуту)
                         ▼
┌─────────────────────────────────────────────────────────┐
│                   LTM (Qdrant)                           │
│              Долгосрочная векторная память                │
│   • ~40 коллекций (text/code пары для каждого агента)    │
│   • Гибридный поиск: семантический + по ключевым словам  │
│   • Два embedding-модели: текст + код                    │
│   • Доменная классификация (L0) + кластеризация (L1)     │
└────────────────────────┬────────────────────────────────┘
                         │ Airflow (каждый час)
                         ▼
┌─────────────────────────────────────────────────────────┐
│              Living Library (Qdrant + HDBSCAN)           │
│            Организованное хранилище документов            │
│   • Автоматическая классификация доменов                 │
│   • Самообучающиеся кластеризаторы секций                │
│   • PDF/DOCX индексация через populate_library.py        │
└─────────────────────────────────────────────────────────┘
```

---

## 2. USTM — Ультракраткосрочная память (Redis)

### 2.1 Ключи Redis

| Ключ | Тип | Макс. размер | Назначение |
|------|-----|-------------|------------|
| `ustm:raw_user_queries` | List | 2 элемента | Последние 2 сырых запроса пользователя |
| `ustm:raw_agent_responses` | List | 2 элемента | Последних 2 ответа Сонаты |
| `ustm:summaries` | List | 10 элементов | Скользящее окно саммари ходов диалога |

### 2.2 Запись (реальное время, в `Sonata.run()`)

После завершения синтеза (`Sonata.py:800-812`):

```python
pipe = redis_client.pipeline()         # Атомарный pipeline
pipe.lpush("ustm:raw_user_queries", current_task)
pipe.ltrim("ustm:raw_user_queries", 0, 1)   # Оставить только 2
pipe.lpush("ustm:raw_agent_responses", final_clean_response)
pipe.ltrim("ustm:raw_agent_responses", 0, 1)
pipe.execute()
```

Элементы пушатся слева (новые первые), обрезаются до 2.

### 2.3 Чтение (реальное время, в `_format_chat_history()`)

```python
summaries = redis_client.lrange("ustm:summaries", 0, 9)
summaries.reverse()  # LIFO → хронологический порядок
```

Саммари инжектятся в промпт как `--- Краткое содержание предыдущих ходов этого диалога ---`.

### 2.4 Формат саммари

Каждый элемент `ustm:summaries` — JSON-строка:

```json
{
    "summary": "первые 150 символов ответа агента...",
    "keywords": ["ключевое_слово_1", "ключевое_слово_2"]
}
```

### 2.5 TTL

Отсутствует. Управление — через `LTRIM` (обрезка по количеству). Данные вытесняются новыми.

---

## 3. LTM — Долгосрочная память (Qdrant)

### 3.1 Embedding-модели

| Модель | Переменная | Размер вектора | Используется для |
|--------|-----------|----------------|------------------|
| `sentence-transformers/all-MiniLM-L6-v2` | `text_embedder` | 384 | Весь естественный язык |
| `jinaai/jina-embeddings-v2-base-code` | `code_embedder` | динамический | Исходный код, AST-узлы |

Обе модели загружаются на GPU (`device="cuda"`), с фоллбэком на CPU.

### 3.2 Организация коллекций

Каждая логическая коллекция существует как **пара**: `{base_name}_text` + `{base_name}_code`.

**Коллекции агентов** (15 штук × 2 = 30):

```
agent_logic_rag_text / agent_logic_rag_code
agent_imagination_rag_text / agent_imagination_rag_code
agent_conscience_rag_text / agent_conscience_rag_code
agent_efficiency_rag_text / agent_efficiency_rag_code
agent_reason_rag_text / agent_reason_rag_code
agent_critical_thinking_logic_rag_text / ..._code
agent_critical_thinking_imagination_rag_text / ..._code
agent_critical_thinking_conscience_rag_text / ..._code
agent_critical_thinking_efficiency_rag_text / ..._code
agent_critical_thinking_reason_rag_text / ..._code
agent_summarizer_logic_rag_text / ..._code
agent_summarizer_imagination_rag_text / ..._code
agent_summarizer_conscience_rag_text / ..._code
agent_summarizer_efficiency_rag_text / ..._code
agent_summarizer_reason_rag_text / ..._code
```

**Системные коллекции** (4 × 2 = 8):

```
user_queries_rag_text / user_queries_rag_code
conversation_summaries_rag_text / conversation_summaries_rag_code
archived_generic_rag_text / archived_generic_rag_code
document_library_rag_text / document_library_rag_code
```

**Итого: ~38 коллекций** в Qdrant.

### 3.3 Payload каждой точки

```python
{
    "session_id": str,
    "timestamp": float,           # Unix timestamp
    "tags": List[str],            # Из context_plan
    "domain": str,                # L0-классификация ("engineering", "philosophy_and_psychology"...)
    "section_id": int,            # L1-кластер ID (-1 = нет модели, -2 = ошибка)
    "text": str,                  # Собственно контент
    "data_type": str,             # "natural_language_doc", "natural_language_chunk", "ast_node", "code_fragment"
    "original_doc_id": str,       # UUID или детерминированный UUID5
    "keywords": List[str],        # (для данных библиотеки)
    "chunk_number": int,          # (для чанков)
    "code_hash": str,             # (для кода)
    "node_info": {"type": str, "name": str}  # (для AST-узлов)
}
```

### 3.4 Путь записи (`_save_to_rag()`)

```
Вход: collection_name, doc_id, content, metadata, session_id, context_plan
  │
  ├── 1. Доменная классификация (L0)
  │      Dispatcher.classify(content, data_type)
  │      → "engineering" / "philosophy_and_psychology" / "project_meta" / "personal_dialogue"
  │
  ├── 2. Секционная классификация (L1)
  │      SectionSpecialist(domain).predict(vector)
  │      → section_id (кластер HDBSCAN)
  │
  ├── 3. Ветвление по типу данных:
  │
  │   ┌─── source_code ──────────────────────────────┐
  │   │  • AST-индексация (code_processor)            │
  │   │    →每个AST-узел → отдельная точка            │
  │   │    → data_type: "ast_node"                    │
  │   │  • Хеширование фрагментов (SHA256)            │
  │   │    →每个фрагмент → отдельная точка             │
  │   │    → data_type: "code_fragment"               │
  │   │    → вектор: нулевой (дамми)                  │
  │   └───────────────────────────────────────────────┘
  │
  │   ┌─── natural_language ─────────────────────────┐
  │   │  • Если > 500 символов:                       │
  │   │    → Чанкинг (450 символов, 50 overlap)       │
  │   │    →每个чанк → отдельная точка                 │
  │   │    → data_type: "natural_language_chunk"      │
  │   │  • Если ≤ 500 символов:                       │
  │   │    → Единая точка                             │
  │   │    → data_type: "natural_language_doc"        │
  │   └───────────────────────────────────────────────┘
  │
  └── 4. Upsert в Qdrant
```

### 3.5 Путь чтения (гибридный поиск)

```
Вход: context_plan (query, tags, data_type), session_id
  │
  ├── Коллекции для поиска:
  │   • Всегда: document_library_rag_text / _code
  │   • Если memory-теги: ВСЕ остальные коллекции
  │
  ├── Параллельно два поиска:
  │
  │   ┌── A. Семантический поиск ────────────────────┐
  │   │  • Эмбеддинг запроса (text или code модель)   │
  │   │  • client.search(collection, vector, limit=5) │
  │   │  • Только коллекции соответствующего суффикса  │
  │   └───────────────────────────────────────────────┘
  │
  │   ┌── B. Точный поиск ───────────────────────────┐
  │   │  • Разбиение ключевых слов                    │
  │   │  • Filter(should=[MatchAny(keywords)])        │
  │   │  • client.scroll(collection, filter, limit=5) │
  │   │  • Только _text коллекции                     │
  │   └───────────────────────────────────────────────┘
  │
  ├── Дедупликация по тексту
  │   • Точные совпадения получают буст +1.0 к скору
  │
  └── Top-5 результатов → форматированный контекст
```

### 3.6 Память агентов (внутри `_run_single_agent()`)

Каждый агент при вызове имеет доступ к двум коллекциям:

1. `user_queries_rag_text` — что пользователи спрашивали
2. `agent_{agent_name}_rag_text` — собственные прошлые отчёты

```python
query_vector = text_embedder.encode(task)
query_filter = Filter(must=[{"key": "session_id", "match": {"value": session_id}}])
results = client.search(collection, query_vector, query_filter, limit=5)
```

Фильтрация по `session_id` — агент видит только контекст текущей сессии. Результаты инжектятся как `retrieved_user_queries` и `retrieved_agent_memories`.

---

## 4. Система Лорибрана (L0 + L1)

### 4.1 L0: Dispatcher — Доменная классификация

**Алгоритм:** центроидный (косинусное сходство)

| Домен | Embedder | Ключевые слова (семантическое ядро) |
|-------|----------|-------------------------------------|
| `engineering` | code_embedder | код, программирование, python, docker, sql, api, backend... |
| `philosophy_and_psychology` | text_embedder | философия, сознание, этика, психология, эмоции... |
| `project_meta` | text_embedder | управление проектом, дорожная карта, планирование... |
| `personal_dialogue` | text_embedder | личный разговор, приветствие, как дела... (дефолт) |

**Правило:** домен может быть классифицирован только той моделью, которой был создан его центроид. Это гарантирует совпадение embedding-пространств.

### 4.2 L1: SectionSpecialist — Кластеризация секций

**Алгоритм:** HDBSCAN (`min_cluster_size=3, min_samples=1, metric='euclidean'`)

- Модели обучаются по векторам из Qdrant для каждого домена
- Сохраняются в `librarian_models/{domain}_l1_model.joblib`
- `predict()` использует `hdbscan.approximate_predict()` для быстрого инференса
- Результат: ID кластера или -1 (шум)

---

## 5. Airflow: Фоновая самоорганизация

### 5.1 memory_orchestrator_dag — Конвейер памяти

**Расписание:** `*/1 * * * *` (каждую минуту)  
**Единственная задача:** `process_and_archive_memory_turn`

```
┌──────────────────────────────────────────────────────────┐
│                 Каждую минуту                             │
│                                                          │
│  1. RPOP oldest query из ustm:raw_user_queries           │
│  2. RPOP oldest response из ustm:raw_agent_responses     │
│  3. Если пусто → выход (нечего обрабатывать)              │
│                                                          │
│  4. Склейка: "Пользователь: {query}\n\nСоната: {response}"│
│                                                          │
│  5. Извлечение ключевых слов (SpaCy Extractor)           │
│     → multi-language: ru_core_news_md / en_core_web_md   │
│     → NER + лемматизация существительных                  │
│                                                          │
│  6. Создание саммари: response[:150] + "..."             │
│                                                          │
│  7. LPUSH summary+keywords в ustm:summaries              │
│                                                          │
│  8. Если ustm:summaries > 10:                            │
│     → RPOP oldest summary                                │
│     → POST /api/internal/archive_text                    │
│       → Archivarius (LLM) анализирует контекст           │
│       → _save_to_rag() → Qdrant conversation_summaries   │
│                                                          │
│  9. При ошибке архивации:                                │
│     → summary возвращается обратно в Redis (rollback)    │
└──────────────────────────────────────────────────────────┘
```

### 5.2 librarian_retraining_dag — Самообучение

**Расписание:** `0 * * * *` (каждый час, на 0-й минуте)

```
┌──────────────────────────────────────────────────────────┐
│                 Каждый час                                │
│                                                          │
│  1. check_for_noisy_domains()                            │
│     → GET /api/library/noise_stats                       │
│     → Фильтр: шум >= 50 точек                            │
│                                                          │
│  2. generate_commands(domains)                           │
│     → Для каждого домена:                                │
│       "python train_librarians.py --domain {domain}"     │
│                                                          │
│  3. retrain_domain (BashOperator, .expand())             │
│     → SCROLL все Qdrant-векторы домена                   │
│     → HDBSCAN кластеризация (min_cluster_size=3)         │
│     → Сохранение модели в .joblib                         │
└──────────────────────────────────────────────────────────┘
```

### 5.3 Вспомогательные ML-модули

| Модуль | Модель | Назначение | Статус |
|--------|--------|------------|--------|
| `ml_extractor.py` | SpaCy `ru_core_news_md` / `en_core_web_md` | Извлечение ключевых слов, NER | **Активен** (используется DAG) |
| `ml_summarizer.py` | `cointegrated/rut5-base-absum` (T5) | Абстрактивная суммаризация | **Не используется** — DAG обрезает текст до 150 символов вместо вызова T5 |

---

## 6. docker-compose: инфраструктура

| Сервис | Образ | Порт | Назначение |
|--------|-------|------|------------|
| `qdrant` | `qdrant/qdrant:latest` | 6333 (HTTP), 6334 (gRPC) | Векторная БД. Том: `./qdrant_data` |
| `redis` | `redis:latest` | 6379 | USTM — рабочая память |
| `airflow-standalone` | Кастомный `Dockerfile.airflow` | 8080 | Оркестратор фоновых задач. GPU-доступ. |

Все сервисы в сети `sonata_agi_network`.

---

## 7. Жизненный цикл данных: полная картина

### Запись (за один ход диалога):

```
Запрос пользователя
  → Gatekeeper: классификация (код / текст)
  → Archivarius (LLM): план контекста (нужна ли память, теги, запрос)
  → Если memory_required: retrieve_data_from_rag() [ЧТЕНИЕ]
  → Sonata.run():
      → Phase 1-3 (5 перспектив × 3 фазы, параллельно)
      → Phase 4: Синтез
      → Redis USTM: запись запроса + ответа (pipeline, max 2)
      → Фоновая задача: _save_to_rag() для:
          • user_queries_rag
          • agent_{perspective}_rag (Phase 1)
          • agent_critical_thinking_{perspective}_rag (Phase 2)
          • agent_summarizer_{perspective}_rag (Phase 3)
          → L0 классификация → домен
          → L1 предсказание → section_id
          → Эмбеддинг подходящей моделью
          → Чанкинг (если > 500 символов)
          → Upsert в Qdrant
```

### Фоновая обработка (каждую минуту):

```
Airflow memory_orchestrator_dag
  → RPOP oldest query/response из Redis
  → Извлечение ключевых слов (SpaCy)
  → Саммари: response[:150]
  → LPUSH в ustm:summaries (max 10)
  → При переполнении: архивация oldest summary → Qdrant
```

### Техническое обслуживание (каждый час):

```
Airflow librarian_retraining_dag
  → Мониторинг шума в кластерах
  → Переобучение HDBSCAN для проблемных доменов
```

---

## 8. Проблемы и «спорные решения»

### 8.1 Почему эксперимент с Airflow был неудачным

**Проблема 1: Неправильная абстракция уровня**
Airflow задумывался как «подсознание» — фоновый процесс, обрабатывающий память. Но Airflow заточен под DAG-пайплайны с тяжёлыми задачами (ETL, батчинг). Для задачи «каждую минуту RPOP из Redis и архивация в Qdrant» это тяжёлая артиллерия:
- Docker-контейнер Airflow занимает >1GB
- Cold start Airflow-воркера ~30 секунд
- На каждую минутную задачу поднимается entire Airflow scheduler + worker

**Проблема 2: HTTP-звёзды между контейнерами**
Airflow обращается к Sonata через `http://host.docker.internal:8000/api/internal/archive_text`. Это:
- Ненадёжно при старте (Sonata ещё не готова)
- Медленно (HTTP overhead на каждую минуту)
- Хрупко (если Sonata перезапускается — Airflow теряет данные)

**Проблема 3: Архитектурное дублирование**
Сам Airflow DAG дублирует логику, которая уже есть в Sonata: работа с Redis, форматирование, ошибка rollback. Два центра управления одним состоянием.

**Проблема 4: T5-summarizer не используется**
`ml_summarizer.py` написан и загружен, но DAG использует примитивную обрезку `response[:150]` вместо полноценной суммаризации. Инфраструктура есть, интеграции нет.

### 8.2 Что было правильной идеей (но неправильной реализацией)

| Идея | Почему верная | Что пошло не так |
|------|---------------|-----------------|
| Фоновая обработка памяти | Диалог не должен блокироваться на суммаризации | Airflow — избыточный оркестратор для простой cron-задачи |
| Самообучение кластеризаторов | Автоматическая адаптация к новым доменам | /api/library/noise_stats не реализован; нет обратной связи |
| Ключевые слова через SpaCy | Определяют релевантность без LLM-вызова | Работает только для ru/en; нет fallback для других языков |
| USTM→LTM конвейер | Плавная деградация краткосрочной → долгосрочной памяти | Два независимых центра управления (Sonata + Airflow) |

---

## 9. Вспомогательные скрипты

| Скрипт | Назначение |
|--------|------------|
| `populate_library.py` | Пакетная индексация PDF/DOCX из `my_documents/` в `document_library_rag_text` |
| `reindex_sections.py` | Пересчёт L1 `section_id` для существующих точек в指定ленных доменах |
| `export_memory.py` | Экспорт `conversation_summaries_rag_text` в JSONL |
| `import_memory.py` | Импорт из JSONL в `conversation_summaries_rag_text` |
| `train_librarians.py` | Обучение HDBSCAN L1-моделей по доменам |

---

## 10. Извлекаемые уроки для нового проекта

### Что сохранить

1. **Концепция трёх уровней памяти** (USTM → LTM → Library) — правильная абстракция
2. **Дуальные embedding-модели** (текст + код) — осознанный выбор
3. **Гибридный поиск** (семантический + точный) — даёт лучшие результаты, чем чистый векторный
4. **L0-домены с центроидами** — простая и эффективная классификация
5. **L1-кластеризация HDBSCAN** — адаптивная, не требует заранее заданного числа кластеров
6. **Память агентов** — каждый агент помнит свои прошлые отчёты (но видит только текущую сессию)

### Что переделать

1. **Заменить Airflow на встроенный планировщик** — `asyncio`-based, без Docker, без HTTP
2. **Единая точка управления памятью** — один модуль `MemoryManager`, а не два центра (Sonata + Airflow)
3. **T5-суммаризацию подключить** — она написана, но не используется; заменить обрезку 150 символов
4. **Добавить TTL** в Redis — сейчас данные живут вечно, вытесняясь только LTRIM
5. **Реализовать `/api/library/noise_stats`** — без этого самообучение не работает
6. **Сократить количество коллекций** — 38 коллекций Qdrant это overhead; можно выделить один векторный индекс с payload-фильтрацией
7. **memory_core/** — зачаточная абстракция, которая так и не была интегрирована; в новом проекте это должен быть первый модуль

### Архитектурный принцип для нового проекта

> **Память — это сервис, не процесс.**  
> В Sonata память была размазана между Sonata.py, Airflow DAGs и вспомогательными скриптами.  
> В новом проекте память должна быть одним модулем с чётким API: `store()`, `retrieve()`, `summarize()`, `reorganize()`.  
> Фоновая обработка — внутренняя деталь этого модуля, не отдельная инфраструктура.
