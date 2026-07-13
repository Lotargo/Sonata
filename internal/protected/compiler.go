package protected

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

const compiledPromptRedacted = "[REDACTED COMPILED PROMPT]"

type PromptRole string

const (
	PromptRoleSystem PromptRole = "system"
	PromptRoleUser   PromptRole = "user"
)

type PromptMessage struct {
	Role    PromptRole
	Content string
}

type RuntimeContext struct {
	UserInput        string
	RoleInput        string
	EmotionReport    string
	ContextPack      string
	InternalDialogue string
	ToolResults      string
}

type CompileInput struct {
	InstructionID string
	Manifest      ResolvedManifest
	Runtime       RuntimeContext
}

type CompilationMetadata struct {
	Instruction    Metadata
	ManifestSource ManifestSource
	Manifest       Metadata
	Phase          string
	Perspective    string
	OutputContract string
	ToolMode       string
	AllowedTools   []string
}

type CompiledPrompt struct {
	Messages []PromptMessage
	Metadata CompilationMetadata
}

func (CompiledPrompt) String() string   { return compiledPromptRedacted }
func (CompiledPrompt) GoString() string { return "protected.CompiledPrompt{REDACTED}" }
func (CompiledPrompt) LogValue() slog.Value {
	return slog.StringValue(compiledPromptRedacted)
}
func (CompiledPrompt) MarshalJSON() ([]byte, error) {
	return []byte(`"` + compiledPromptRedacted + `"`), nil
}
func (CompiledPrompt) MarshalText() ([]byte, error) {
	return []byte(compiledPromptRedacted), nil
}

type PromptCompiler struct {
	bundle *Bundle
}

func NewPromptCompiler(bundle *Bundle) (*PromptCompiler, error) {
	if bundle == nil {
		return nil, errors.New("protected bundle is required")
	}
	return &PromptCompiler{bundle: bundle}, nil
}

func (c *PromptCompiler) Compile(input CompileInput) (CompiledPrompt, error) {
	if c == nil || c.bundle == nil {
		return CompiledPrompt{}, errors.New("prompt compiler is not initialized")
	}
	instruction, exists := c.bundle.Instructions[input.InstructionID]
	if !exists {
		return CompiledPrompt{}, fmt.Errorf("unknown protected instruction %q", input.InstructionID)
	}
	if err := validateResolvedManifest(input.Manifest, input.InstructionID); err != nil {
		return CompiledPrompt{}, err
	}
	if strings.TrimSpace(input.Runtime.UserInput) == "" && strings.TrimSpace(input.Runtime.RoleInput) == "" {
		return CompiledPrompt{}, errors.New("runtime user input or role input is required")
	}
	system, err := renderSystemPrompt(instruction, input.Manifest)
	if err != nil {
		return CompiledPrompt{}, err
	}
	user, err := renderRuntimePrompt(input.Runtime)
	if err != nil {
		return CompiledPrompt{}, err
	}
	allowed := append([]string(nil), instruction.Tools.Allowed...)
	return CompiledPrompt{
		Messages: []PromptMessage{
			{Role: PromptRoleSystem, Content: system},
			{Role: PromptRoleUser, Content: user},
		},
		Metadata: CompilationMetadata{
			Instruction:    instruction.Metadata,
			ManifestSource: input.Manifest.Source,
			Manifest:       input.Manifest.Metadata,
			Phase:          instruction.Phase,
			Perspective:    instruction.Perspective,
			OutputContract: instruction.OutputContract,
			ToolMode:       instruction.Tools.Mode,
			AllowedTools:   allowed,
		},
	}, nil
}

func validateResolvedManifest(manifest ResolvedManifest, instructionID string) error {
	if manifest.Metadata.ID == "" || manifest.Metadata.Version < 1 || manifest.Metadata.Hash == "" {
		return errors.New("resolved manifest metadata is incomplete")
	}
	switch manifest.Source {
	case ManifestSourceSystemDefault:
		if manifest.Default == nil || manifest.UserText != "" {
			return errors.New("system default manifest requires protected default content only")
		}
		if manifest.Default.Target != instructionID {
			return errors.New("resolved default manifest targets another instruction")
		}
		if manifest.Default.Metadata != manifest.Metadata {
			return errors.New("resolved default manifest metadata mismatch")
		}
	case ManifestSourceUserGlobal, ManifestSourceUserChat:
		if manifest.Default != nil || strings.TrimSpace(manifest.UserText) == "" {
			return errors.New("user manifest requires text content only")
		}
	default:
		return fmt.Errorf("unsupported manifest source %q", manifest.Source)
	}
	return nil
}

func renderSystemPrompt(instruction Instruction, manifest ResolvedManifest) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString(`<sonata-runtime version="1">`)
	buffer.WriteString(`<protected-instruction id="`)
	writeEscaped(&buffer, instruction.ID)
	buffer.WriteString(`" version="`)
	buffer.WriteString(fmt.Sprintf("%d", instruction.Version))
	buffer.WriteString(`" hash="`)
	writeEscaped(&buffer, instruction.Hash)
	buffer.WriteString(`" phase="`)
	writeEscaped(&buffer, instruction.Phase)
	buffer.WriteString(`" perspective="`)
	writeEscaped(&buffer, instruction.Perspective)
	buffer.WriteString(`">`)
	writeElement(&buffer, "identity", instruction.Identity.Entity)
	writeElement(&buffer, "identity-mode", instruction.Identity.Mode)
	writeElement(&buffer, "separate-agent", fmt.Sprintf("%t", instruction.Identity.SeparateAgent))
	writeElement(&buffer, "purpose", instruction.Purpose)
	buffer.WriteString(`<invariants>`)
	for _, invariant := range instruction.Invariants {
		writeElement(&buffer, "rule", invariant)
	}
	buffer.WriteString(`</invariants>`)
	buffer.WriteString(`<tools mode="`)
	writeEscaped(&buffer, instruction.Tools.Mode)
	buffer.WriteString(`">`)
	for _, tool := range instruction.Tools.Allowed {
		writeElement(&buffer, "tool", tool)
	}
	buffer.WriteString(`</tools>`)
	writeElement(&buffer, "output-contract", instruction.OutputContract)
	buffer.WriteString(`</protected-instruction>`)

	buffer.WriteString(`<active-manifest source="`)
	writeEscaped(&buffer, string(manifest.Source))
	buffer.WriteString(`" id="`)
	writeEscaped(&buffer, manifest.Metadata.ID)
	buffer.WriteString(`" version="`)
	buffer.WriteString(fmt.Sprintf("%d", manifest.Metadata.Version))
	buffer.WriteString(`" hash="`)
	writeEscaped(&buffer, manifest.Metadata.Hash)
	buffer.WriteString(`">`)
	if manifest.Source == ManifestSourceSystemDefault {
		writeElement(&buffer, "tone", manifest.Default.Tone)
		writeElement(&buffer, "focus", manifest.Default.Focus)
		writeElement(&buffer, "verbosity", manifest.Default.Verbosity)
		writeElement(&buffer, "guidance", manifest.Default.Guidance)
	} else {
		buffer.WriteString(`<user-text encoding="escaped-text">`)
		writeEscaped(&buffer, manifest.UserText)
		buffer.WriteString(`</user-text>`)
	}
	buffer.WriteString(`</active-manifest>`)
	buffer.WriteString(`</sonata-runtime>`)
	return buffer.String(), nil
}

func renderRuntimePrompt(runtime RuntimeContext) (string, error) {
	var buffer bytes.Buffer
	buffer.WriteString(`<runtime-context version="1">`)
	writeOptionalElement(&buffer, "user-input", runtime.UserInput)
	writeOptionalElement(&buffer, "role-input", runtime.RoleInput)
	writeOptionalElement(&buffer, "emotion-report", runtime.EmotionReport)
	writeOptionalElement(&buffer, "context-pack", runtime.ContextPack)
	writeOptionalElement(&buffer, "internal-dialogue", runtime.InternalDialogue)
	writeOptionalElement(&buffer, "tool-results", runtime.ToolResults)
	buffer.WriteString(`</runtime-context>`)
	return buffer.String(), nil
}

func writeOptionalElement(buffer *bytes.Buffer, name, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	writeElement(buffer, name, value)
}

func writeElement(buffer *bytes.Buffer, name, value string) {
	buffer.WriteByte('<')
	buffer.WriteString(name)
	buffer.WriteByte('>')
	writeEscaped(buffer, value)
	buffer.WriteString(`</`)
	buffer.WriteString(name)
	buffer.WriteByte('>')
}

func writeEscaped(buffer *bytes.Buffer, value string) {
	_ = xml.EscapeText(buffer, []byte(value))
}
