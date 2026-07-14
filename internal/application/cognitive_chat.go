package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Lotargo/Sonata/internal/cognition"
	"github.com/Lotargo/Sonata/internal/database"
	"github.com/Lotargo/Sonata/internal/database/dbgen"
	"github.com/Lotargo/Sonata/internal/httpapi"
	"github.com/Lotargo/Sonata/internal/protected"
	"github.com/Lotargo/Sonata/internal/provider"
	"github.com/jackc/pgx/v5/pgtype"
)

type CognitiveChatServiceImpl struct {
	runners      *RunnerAdapter
	runRepo      *database.RunRepository
	manifestRepo *database.ManifestRepository
	bundle       *protected.Bundle
	resolver     *protected.ManifestResolver
}

func NewCognitiveChatServiceImpl(
	runners *RunnerAdapter,
	runRepo *database.RunRepository,
	manifestRepo *database.ManifestRepository,
	bundle *protected.Bundle,
	resolver *protected.ManifestResolver,
) (*CognitiveChatServiceImpl, error) {
	if runners == nil {
		return nil, errors.New("runners are required")
	}
	if runRepo == nil {
		return nil, errors.New("run repository is required")
	}
	if manifestRepo == nil {
		return nil, errors.New("manifest repository is required")
	}
	if bundle == nil {
		return nil, errors.New("bundle is required")
	}
	if resolver == nil {
		return nil, errors.New("resolver is required")
	}

	return &CognitiveChatServiceImpl{
		runners:      runners,
		runRepo:      runRepo,
		manifestRepo: manifestRepo,
		bundle:       bundle,
		resolver:     resolver,
	}, nil
}

func (s *CognitiveChatServiceImpl) Complete(
	ctx context.Context,
	request CognitiveChatRequest,
	emit func(httpapi.ChatDelta) error,
) (httpapi.ChatResult, error) {
	if s == nil {
		return httpapi.ChatResult{}, errors.New("cognitive chat service is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return httpapi.ChatResult{}, err
	}

	userID := strings.TrimSpace(request.Identity.UserID)
	chatID := strings.TrimSpace(request.Identity.ChatID)
	messageID := strings.TrimSpace(request.Identity.MessageID)

	if userID == "" || chatID == "" || messageID == "" {
		return httpapi.ChatResult{}, errors.New("request identity parameters (UserID, ChatID, MessageID) are required")
	}

	// 1. Parse history and user input
	history := make([]cognition.ConversationMessage, len(request.Messages))
	for i, msg := range request.Messages {
		var contentStr string
		if err := json.Unmarshal(msg.Content, &contentStr); err != nil {
			contentStr = string(msg.Content)
		}
		history[i] = cognition.ConversationMessage{
			Role:    msg.Role,
			Content: contentStr,
		}
	}

	userInput := latestUserText(request.Messages)
	if userInput == "" {
		return httpapi.ChatResult{}, errors.New("failed to extract user input from request messages")
	}

	// 2. Resolve manifest for each runtime role
	globalManifest, err := s.manifestRepo.Get(ctx, userID, protected.ManifestScopeGlobal, "")
	if err != nil && !errors.Is(err, database.ErrUserManifestNotFound) {
		return httpapi.ChatResult{}, err
	}
	var globalPtr *protected.UserManifest
	if err == nil {
		globalPtr = &globalManifest
	}

	chatManifest, err := s.manifestRepo.Get(ctx, userID, protected.ManifestScopeChat, chatID)
	if err != nil && !errors.Is(err, database.ErrUserManifestNotFound) {
		return httpapi.ChatResult{}, err
	}
	var chatPtr *protected.UserManifest
	if err == nil {
		chatPtr = &chatManifest
	}

	roleArtifacts := make(map[cognition.RuntimeRole]cognition.RoleArtifacts)
	userManifestContent := make(map[string]string)

	for _, spec := range cognition.RuntimeRoleSpecs() {
		resolved, err := s.resolver.Resolve(protected.ResolveManifestInput{
			InstructionID: spec.InstructionID,
			OwnerID:       userID,
			ChatID:        chatID,
			Chat:          chatPtr,
			Global:        globalPtr,
		})
		if err != nil {
			return httpapi.ChatResult{}, fmt.Errorf("resolve manifest for role %s: %w", spec.Role, err)
		}

		if resolved.Source == protected.ManifestSourceUserGlobal || resolved.Source == protected.ManifestSourceUserChat {
			userManifestContent[resolved.Metadata.ID] = resolved.UserText
		}

		instruction := s.bundle.Instructions[spec.InstructionID]
		roleArtifacts[spec.Role] = cognition.RoleArtifacts{
			Instruction: cognition.ArtifactRef{
				ID:      spec.InstructionID,
				Version: instruction.Version,
				Hash:    instruction.Hash,
			},
			Manifest: cognition.ManifestRef{
				ArtifactRef: cognition.ArtifactRef{
					ID:      resolved.Metadata.ID,
					Version: resolved.Metadata.Version,
					Hash:    resolved.Metadata.Hash,
				},
				Source: string(resolved.Source),
			},
		}
	}

	// Put user manifest content into request context
	ctxWithManifests := ContextWithUserManifests(ctx, userManifestContent)

	// 3. Run Router first to determine the route
	routerInput := cognition.RouterInput{
		UserInput: userInput,
		History:   history,
	}

	collector := &UsageCollector{}
	ctxWithManifests = context.WithValue(ctxWithManifests, usageCollectorKey{}, collector)

	routerResult, routerErr := s.runners.RunRouter(ctxWithManifests, routerInput)

	var lastUserMessageContent json.RawMessage
	for i := len(request.Messages) - 1; i >= 0; i-- {
		if strings.EqualFold(request.Messages[i].Role, "user") {
			lastUserMessageContent = request.Messages[i].Content
			break
		}
	}
	if len(lastUserMessageContent) == 0 {
		lastUserMessageContent = json.RawMessage(`""`)
	}

	if routerErr != nil {
		// Log failed run to DB
		startedAt := time.Now()
		rolesToStart := []database.RoleRunStart{
			{
				Role:        cognition.RoleRouter,
				ModelID:     "nemotron-3-ultra-free",
				Instruction: roleArtifacts[cognition.RoleRouter].Instruction,
				Manifest:    roleArtifacts[cognition.RoleRouter].Manifest,
			},
		}
		begunRun, bErr := s.runRepo.BeginCognitiveRun(ctxWithManifests, database.BeginCognitiveRunInput{
			OwnerID:           userID,
			ConversationID:    chatID,
			ConversationTitle: "Chat Conversation",
			MessageID:         messageID,
			MessageContent:    lastUserMessageContent,
			Route:             cognition.RouteDirect,
			StartedAt:         startedAt,
			Roles:             rolesToStart,
		})
		if bErr == nil {
			var errorCode string
			if pErr, ok := provider.ErrorCodeOf(routerErr); ok {
				errorCode = string(pErr)
			} else {
				errorCode = "UNKNOWN"
			}
			_, _ = s.runRepo.CompleteRoleRun(ctxWithManifests, database.CompleteRoleRunInput{
				OwnerID:        userID,
				CognitiveRunID: begunRun.Run.ID,
				RoleRunID:      begunRun.Roles[0].ID,
				Metadata: cognition.RoleMetadata{
					Role:        cognition.RoleRouter,
					Status:      cognition.RoleStatusFailed,
					Instruction: roleArtifacts[cognition.RoleRouter].Instruction,
					Manifest:    roleArtifacts[cognition.RoleRouter].Manifest,
				},
				ErrorCode: errorCode,
			})
			var cognitiveStatus database.CognitiveRunFinalStatus = database.CognitiveRunStatusFailedRouting
			if pErr, ok := provider.ErrorCodeOf(routerErr); ok && pErr == provider.CodeProviderExhausted {
				cognitiveStatus = database.CognitiveRunStatusProviderExhausted
			}
			_, _ = s.runRepo.CompleteCognitiveRun(ctxWithManifests, database.CompleteCognitiveRunInput{
				OwnerID:        userID,
				CognitiveRunID: begunRun.Run.ID,
				Status:         cognitiveStatus,
				CompletedAt:    time.Now(),
			})
		}
		return httpapi.ChatResult{}, fmt.Errorf("router failure: %w", routerErr)
	}

	// 4. Begin Cognitive Run in DB
	route := routerResult.Output.Route
	var rolesToStart []database.RoleRunStart
	if route == cognition.RouteDirect {
		rolesToStart = []database.RoleRunStart{
			{
				Role:        cognition.RoleRouter,
				ModelID:     routerResult.Metadata.ModelID,
				Instruction: roleArtifacts[cognition.RoleRouter].Instruction,
				Manifest:    roleArtifacts[cognition.RoleRouter].Manifest,
			},
			{
				Role:        cognition.RoleSynthesisFinal,
				ModelID:     "big-pickle",
				Instruction: roleArtifacts[cognition.RoleSynthesisFinal].Instruction,
				Manifest:    roleArtifacts[cognition.RoleSynthesisFinal].Manifest,
			},
		}
	} else {
		// Full route starts all roles defined in spec
		rolesToStart = make([]database.RoleRunStart, 0, len(roleArtifacts))
		for _, spec := range cognition.RuntimeRoleSpecs() {
			rolesToStart = append(rolesToStart, database.RoleRunStart{
				Role:        spec.Role,
				ModelID:     defaultModelForRole(spec.Role),
				Instruction: roleArtifacts[spec.Role].Instruction,
				Manifest:    roleArtifacts[spec.Role].Manifest,
			})
		}
	}

	startedAt := time.Now()
	begunRun, err := s.runRepo.BeginCognitiveRun(ctxWithManifests, database.BeginCognitiveRunInput{
		OwnerID:           userID,
		ConversationID:    chatID,
		ConversationTitle: "Chat Conversation",
		MessageID:         messageID,
		MessageContent:    lastUserMessageContent,
		Route:             route,
		StartedAt:         startedAt,
		Roles:             rolesToStart,
	})
	if err != nil {
		return httpapi.ChatResult{}, fmt.Errorf("begin cognitive run: %w", err)
	}

	roleRunIDs := make(map[cognition.RuntimeRole]pgtype.UUID)
	for _, rr := range begunRun.Roles {
		if role, ok := roleFromPhaseAndPerspective(rr.Phase, rr.Perspective); ok {
			roleRunIDs[role] = rr.ID
		}
	}

	// Complete the router role run immediately
	_, _ = s.runRepo.CompleteRoleRun(ctxWithManifests, database.CompleteRoleRunInput{
		OwnerID:        userID,
		CognitiveRunID: begunRun.Run.ID,
		RoleRunID:      roleRunIDs[cognition.RoleRouter],
		Metadata:       routerResult.Metadata,
	})

	// Log router's provider usage if collected
	if s.runners.usageRepo != nil && collector.Model != "" {
		_, _ = s.runners.usageRepo.Insert(ctxWithManifests, database.InsertProviderUsageInput{
			OwnerID:        userID,
			CognitiveRunID: begunRun.Run.ID,
			RoleRunID:      roleRunIDs[cognition.RoleRouter],
			Provider:       "open_code_zen",
			ModelID:        collector.Model,
			InputTokens:    int64(collector.Usage.InputTokens),
			OutputTokens:   int64(collector.Usage.OutputTokens),
			CachedTokens:   int64(collector.Usage.CachedTokens),
			CreatedAt:      time.Now(),
		})
	}

	// Put Run context info in request context
	runInfo := RunContextInfo{
		OwnerID:        userID,
		CognitiveRunID: begunRun.Run.ID,
		RoleRunIDs:     roleRunIDs,
	}
	ctxWithManifests = ContextWithRunInfo(ctxWithManifests, runInfo)

	// Decorate runners to log stats to DB
	decRaw := &dbRawRunner{runner: s.runners, runRepo: s.runRepo, ownerID: userID, cognitiveRunID: begunRun.Run.ID, roleRunIDs: roleRunIDs}
	decCritical := &dbCriticalRunner{runner: s.runners, runRepo: s.runRepo, ownerID: userID, cognitiveRunID: begunRun.Run.ID, roleRunIDs: roleRunIDs}
	decSummary := &dbSummaryRunner{runner: s.runners, runRepo: s.runRepo, ownerID: userID, cognitiveRunID: begunRun.Run.ID, roleRunIDs: roleRunIDs}
	decTooling := &dbSynthesisToolingRunner{runner: s.runners, runRepo: s.runRepo, ownerID: userID, cognitiveRunID: begunRun.Run.ID, roleRunIDs: roleRunIDs}
	decFinal := &dbSynthesisFinalRunner{runner: s.runners, runRepo: s.runRepo, ownerID: userID, cognitiveRunID: begunRun.Run.ID, roleRunIDs: roleRunIDs}

	var decExecutor cognition.ToolExecutor
	if requestExecutor, ok := ctx.Value("tool_executor").(cognition.ToolExecutor); ok {
		toolRepo, _ := database.NewToolCallRepository(s.runRepo.Pool())
		decExecutor = &dbToolExecutor{
			executor:       requestExecutor,
			toolRepo:       toolRepo,
			ownerID:        userID,
			cognitiveRunID: begunRun.Run.ID,
			roleRunID:      roleRunIDs[cognition.RoleSynthesisTooling],
		}
	}

	// 5. Construct a pipeline using the decorated runners
	pipeline, err := cognition.NewPipeline(
		s.runners, // router is already executed
		decRaw,
		decCritical,
		decSummary,
		decTooling,
		decFinal,
		decExecutor,
		cognition.FullPipelineOptions{
			PhaseTimeout:         2 * time.Minute,
			MaxConcurrentPrisms:  5,
			MinimumHealthyPrisms: 4,
		},
	)
	if err != nil {
		return httpapi.ChatResult{}, fmt.Errorf("create run pipeline: %w", err)
	}

	// 6. Execute the pipeline
	pipelineInput := cognition.PipelineInput{
		UserInput: userInput,
		History:   history,
		Emotion:   request.Emotion,
		Context:   cognition.ContextPack{Text: ""},
		Artifacts: roleArtifacts,
	}

	pipelineResult, runErr := pipeline.Run(ctxWithManifests, pipelineInput)

	// Complete Cognitive Run in DB
	var finalStatus database.CognitiveRunFinalStatus = database.CognitiveRunStatusOK
	if runErr != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(runErr); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}

		if errorCode == string(provider.CodeProviderExhausted) {
			finalStatus = database.CognitiveRunStatusProviderExhausted
		} else {
			if errors.Is(runErr, cognition.ErrInsufficientPrisms) {
				finalStatus = database.CognitiveRunStatusFailedContext
			} else if strings.Contains(runErr.Error(), "synthesis tooling") {
				finalStatus = database.CognitiveRunStatusFailedTooling
			} else if strings.Contains(runErr.Error(), "synthesis final") {
				finalStatus = database.CognitiveRunStatusFailedSynthesis
			} else {
				finalStatus = database.CognitiveRunStatusFailedSynthesis
			}
		}
	} else if pipelineResult.Status == cognition.RoleStatusDegraded {
		finalStatus = database.CognitiveRunStatusDegraded
	}

	_, _ = s.runRepo.CompleteCognitiveRun(ctxWithManifests, database.CompleteCognitiveRunInput{
		OwnerID:        userID,
		CognitiveRunID: begunRun.Run.ID,
		Status:         finalStatus,
		CompletedAt:    time.Now(),
	})

	if runErr != nil {
		return httpapi.ChatResult{}, fmt.Errorf("run pipeline: %w", runErr)
	}

	// 7. Stream the final answer back using the emit callback in chunks
	var content string
	var finishReason string
	if route == cognition.RouteDirect {
		content = pipelineResult.Direct.Final.Content
		finishReason = "stop"
	} else {
		content = pipelineResult.Full.Final.Content
		finishReason = "stop"
	}

	chunkSize := 16
	for i := 0; i < len(content); i += chunkSize {
		end := i + chunkSize
		if end > len(content) {
			end = len(content)
		}
		if err := emit(httpapi.ChatDelta{Content: content[i:end]}); err != nil {
			return httpapi.ChatResult{}, err
		}
	}

	return httpapi.ChatResult{
		FinishReason: finishReason,
	}, nil
}

func defaultModelForRole(role cognition.RuntimeRole) string {
	switch {
	case role == cognition.RoleRouter:
		return "nemotron-3-ultra-free"
	case strings.HasSuffix(string(role), "_raw"):
		return "deepseek-v4-flash-free"
	case strings.HasSuffix(string(role), "_critical"):
		return "deepseek-v4-flash-free"
	case strings.HasSuffix(string(role), "_summary"):
		return "nemotron-3-ultra-free"
	case role == cognition.RoleSynthesisTooling:
		return "big-pickle"
	case role == cognition.RoleSynthesisFinal:
		return "big-pickle"
	default:
		return "mimo-v2.5-free"
	}
}

func roleFromPhaseAndPerspective(phase string, perspective string) (cognition.RuntimeRole, bool) {
	for _, spec := range cognition.RuntimeRoleSpecs() {
		if string(spec.Phase) == phase && spec.Perspective == perspective {
			return spec.Role, true
		}
	}
	return "", false
}

// Decorated runners to intercept pipeline execution and log status to DB

type dbRawRunner struct {
	runner         cognition.RawRunner
	runRepo        *database.RunRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func (d *dbRawRunner) RunRaw(ctx context.Context, input cognition.RawInput) (cognition.PrismReport, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_raw")
	report, err := d.runner.RunRaw(ctx, input)
	roleRunID := d.roleRunIDs[role]
	if err != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(err); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}
		_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			RoleRunID:      roleRunID,
			Metadata: cognition.RoleMetadata{
				Role:        role,
				Status:      cognition.RoleStatusFailed,
				Instruction: input.Instruction,
				Manifest:    input.Manifest,
			},
			ErrorCode: errorCode,
		})
		return report, err
	}
	_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
		OwnerID:        d.ownerID,
		CognitiveRunID: d.cognitiveRunID,
		RoleRunID:      roleRunID,
		Metadata:       report.Metadata,
	})
	return report, nil
}

type dbCriticalRunner struct {
	runner         cognition.CriticalRunner
	runRepo        *database.RunRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func (d *dbCriticalRunner) RunCritical(ctx context.Context, input cognition.CriticalInput) (cognition.CriticalReport, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_critical")
	report, err := d.runner.RunCritical(ctx, input)
	roleRunID := d.roleRunIDs[role]
	if err != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(err); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}
		_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			RoleRunID:      roleRunID,
			Metadata: cognition.RoleMetadata{
				Role:        role,
				Status:      cognition.RoleStatusFailed,
				Instruction: input.Instruction,
				Manifest:    input.Manifest,
			},
			ErrorCode: errorCode,
		})
		return report, err
	}
	_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
		OwnerID:        d.ownerID,
		CognitiveRunID: d.cognitiveRunID,
		RoleRunID:      roleRunID,
		Metadata:       report.Metadata,
	})
	return report, nil
}

type dbSummaryRunner struct {
	runner         cognition.SummaryRunner
	runRepo        *database.RunRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func (d *dbSummaryRunner) RunSummary(ctx context.Context, input cognition.SummaryInput) (cognition.PrismSummary, error) {
	role := cognition.RuntimeRole(string(input.Prism) + "_summary")
	report, err := d.runner.RunSummary(ctx, input)
	roleRunID := d.roleRunIDs[role]
	if err != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(err); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}
		_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			RoleRunID:      roleRunID,
			Metadata: cognition.RoleMetadata{
				Role:        role,
				Status:      cognition.RoleStatusFailed,
				Instruction: input.Instruction,
				Manifest:    input.Manifest,
			},
			ErrorCode: errorCode,
		})
		return report, err
	}
	_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
		OwnerID:        d.ownerID,
		CognitiveRunID: d.cognitiveRunID,
		RoleRunID:      roleRunID,
		Metadata:       report.Metadata,
	})
	return report, nil
}

type dbSynthesisToolingRunner struct {
	runner         cognition.SynthesisToolingRunner
	runRepo        *database.RunRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func (d *dbSynthesisToolingRunner) RunSynthesisTooling(ctx context.Context, input cognition.SynthesisToolingInput) (cognition.SynthesisToolingOutput, error) {
	role := cognition.RoleSynthesisTooling
	report, err := d.runner.RunSynthesisTooling(ctx, input)
	roleRunID := d.roleRunIDs[role]
	if err != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(err); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}
		_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			RoleRunID:      roleRunID,
			Metadata: cognition.RoleMetadata{
				Role:        role,
				Status:      cognition.RoleStatusFailed,
				Instruction: input.Instruction,
				Manifest:    input.Manifest,
			},
			ErrorCode: errorCode,
		})
		return report, err
	}
	_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
		OwnerID:        d.ownerID,
		CognitiveRunID: d.cognitiveRunID,
		RoleRunID:      roleRunID,
		Metadata:       report.Metadata,
	})
	return report, nil
}

type dbSynthesisFinalRunner struct {
	runner         cognition.SynthesisFinalRunner
	runRepo        *database.RunRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunIDs     map[cognition.RuntimeRole]pgtype.UUID
}

func (d *dbSynthesisFinalRunner) RunSynthesisFinal(ctx context.Context, input cognition.SynthesisFinalInput) (cognition.SynthesisFinalOutput, error) {
	role := cognition.RoleSynthesisFinal
	report, err := d.runner.RunSynthesisFinal(ctx, input)
	roleRunID := d.roleRunIDs[role]
	if err != nil {
		var errorCode string
		if pErr, ok := provider.ErrorCodeOf(err); ok {
			errorCode = string(pErr)
		} else {
			errorCode = "UNKNOWN"
		}
		_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			RoleRunID:      roleRunID,
			Metadata: cognition.RoleMetadata{
				Role:        role,
				Status:      cognition.RoleStatusFailed,
				Instruction: input.Instruction,
				Manifest:    input.Manifest,
			},
			ErrorCode: errorCode,
		})
		return report, err
	}
	_, _ = d.runRepo.CompleteRoleRun(ctx, database.CompleteRoleRunInput{
		OwnerID:        d.ownerID,
		CognitiveRunID: d.cognitiveRunID,
		RoleRunID:      roleRunID,
		Metadata:       report.Metadata,
	})
	return report, nil
}

type dbToolExecutor struct {
	executor       cognition.ToolExecutor
	toolRepo       *database.ToolCallRepository
	ownerID        string
	cognitiveRunID pgtype.UUID
	roleRunID      pgtype.UUID
}

func (d *dbToolExecutor) ExecuteTools(ctx context.Context, calls []cognition.ToolCall) ([]cognition.ToolResult, error) {
	dbToolCalls := make([]dbgen.ToolCall, len(calls))
	for i, call := range calls {
		reqMeta, _ := json.Marshal(struct {
			Arguments string `json:"arguments"`
		}{Arguments: call.Arguments})
		inserted, err := d.toolRepo.Insert(ctx, database.InsertToolCallInput{
			OwnerID:         d.ownerID,
			CognitiveRunID:  d.cognitiveRunID,
			RoleRunID:       d.roleRunID,
			ToolName:        call.Name,
			Status:          "RUNNING",
			RequestMetadata: reqMeta,
			CreatedAt:       time.Now(),
		})
		if err != nil {
			return nil, fmt.Errorf("insert tool call in DB: %w", err)
		}
		dbToolCalls[i] = inserted
	}

	results, err := d.executor.ExecuteTools(ctx, calls)
	if err != nil {
		for _, tc := range dbToolCalls {
			resMeta, _ := json.Marshal(struct {
				Error string `json:"error"`
			}{Error: err.Error()})
			_, _ = d.toolRepo.Complete(ctx, database.CompleteToolCallInput{
				OwnerID:        d.ownerID,
				CognitiveRunID: d.cognitiveRunID,
				ToolCallID:     tc.ID,
				Status:         "FAILED",
				ResultMetadata: resMeta,
				CompletedAt:    time.Now(),
			})
		}
		return nil, err
	}

	for i, res := range results {
		tc := dbToolCalls[i]
		status := "OK"
		var resMeta []byte
		if res.ErrorCode != "" {
			status = "FAILED"
			resMeta, _ = json.Marshal(struct {
				ErrorCode string `json:"error_code"`
				Content   string `json:"content"`
			}{ErrorCode: res.ErrorCode, Content: res.Content})
		} else {
			resMeta, _ = json.Marshal(struct {
				Content string `json:"content"`
			}{Content: res.Content})
		}
		_, _ = d.toolRepo.Complete(ctx, database.CompleteToolCallInput{
			OwnerID:        d.ownerID,
			CognitiveRunID: d.cognitiveRunID,
			ToolCallID:     tc.ID,
			Status:         status,
			ResultMetadata: resMeta,
			CompletedAt:    time.Now(),
		})
	}

	return results, nil
}
