# Implementation Plan - Sonata Storage Layer & Runtime Wiring (Iterative)

To ensure high quality and prevent errors, we divide the work into four distinct phases. We will execute them sequentially, obtaining user approval at each gate.

---

## Phase 1: Database Repositories & Integration Tests (COMPLETED)
*Goal: Implement and fully test the missing repositories. No API or runtime changes yet.*

### Checklists
- [x] Create [tool_call_repository.go](file:///f:/projects/Sonata/internal/database/tool_call_repository.go)
- [x] Create [provider_usage_repository.go](file:///f:/projects/Sonata/internal/database/provider_usage_repository.go)
- [x] Create [outbox_repository.go](file:///f:/projects/Sonata/internal/database/outbox_repository.go)
- [x] Create [instruction_repository.go](file:///f:/projects/Sonata/internal/database/instruction_repository.go)
- [x] Create [repositories_integration_test.go](file:///f:/projects/Sonata/internal/database/repositories_integration_test.go)
- [x] Run `go test -v ./internal/database/...` (requires local/mock Postgres)

---

## Phase 2: Runtime Runner Adapters (COMPLETED)
*Goal: Implement the runners (`RouterRunner`, `RawRunner`, `CriticalRunner`, `SummaryRunner`, `SynthesisToolingRunner`, `SynthesisFinalRunner`) translating prompt XMLs and parsing provider completions.*

### Checklists
- [x] Create [runners.go](file:///f:/projects/Sonata/internal/application/runners.go)
- [x] Implement robust unmarshaling/decoding for JSON outputs (`RouterOutput`, `PrismReport`, `CriticalReport`, `PrismSummary`, `SynthesisToolingOutput`) with markdown code block stripping.
- [x] Add unit tests for runner adapters using mocked provider completions.

---

## Phase 3: Runtime Wiring & Orchestration (COMPLETED)
*Goal: Connect the Chi HTTP handler, AffectiveChatService, the new Runners, and Database Pool in `main.go`.*

### Checklists
- [x] Create `CognitiveChatServiceImpl` in [cognitive_chat.go](file:///f:/projects/Sonata/internal/application/cognitive_chat.go) to orchestrate runs in the database transaction.
- [x] Modify [main.go](file:///f:/projects/Sonata/cmd/sonata/main.go) to construct `pgxpool`, sync loaded instructions on startup, initialize repositories, and build the Chi router.
- [x] Run a full local execution test via `go test ./...` with mock provider and real DB.

---

## Phase 4: Docker Compilation & Smoke Testing
*Goal: Containerize the monolith, verify schema migrations and run a smoke test.*

### Checklists
- [ ] Verify Dockerfile and compose setup.
- [ ] Run migrations and API server inside Docker.
- [ ] Trigger a request via curl to confirm model registration and pipeline completion.
