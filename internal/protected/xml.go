package protected

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

func preflightXML(data []byte) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	seenRoot := false
	stack := make([]string, 0, 8)
	singletons := make(map[string]struct{})
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}
		switch typed := token.(type) {
		case xml.Directive:
			return errors.New("XML directives and DTD are forbidden")
		case xml.ProcInst:
			if seenRoot || typed.Target != "xml" {
				return errors.New("XML processing instructions are forbidden")
			}
		case xml.StartElement:
			seenRoot = true
			stack = append(stack, typed.Name.Local)
			currentPath := strings.Join(stack, "/")
			if singletonXMLPath(currentPath) {
				if _, duplicate := singletons[currentPath]; duplicate {
					return fmt.Errorf("duplicate XML element %s", currentPath)
				}
				singletons[currentPath] = struct{}{}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	return nil
}

func singletonXMLPath(value string) bool {
	switch value {
	case "sonata-instruction/identity",
		"sonata-instruction/purpose",
		"sonata-instruction/invariants",
		"sonata-instruction/output-contract",
		"sonata-instruction/tools",
		"sonata-instruction/identity/entity",
		"sonata-instruction/identity/mode",
		"sonata-instruction/identity/separate-agent",
		"sonata-manifest/expression",
		"sonata-manifest/guidance",
		"sonata-manifest/expression/tone",
		"sonata-manifest/expression/focus",
		"sonata-manifest/expression/verbosity":
		return true
	default:
		return false
	}
}

func decodeInstruction(data []byte, entry registryEntry) (Instruction, error) {
	if err := preflightXML(data); err != nil {
		return Instruction{}, err
	}
	var document instructionXML
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	if err := decoder.Decode(&document); err != nil {
		return Instruction{}, err
	}
	if document.XMLName.Local != "sonata-instruction" {
		return Instruction{}, fmt.Errorf("unexpected root element %q", document.XMLName.Local)
	}
	if err := rejectUnknown(document.UnknownAttrs, document.Unknown, "sonata-instruction"); err != nil {
		return Instruction{}, err
	}
	if err := rejectUnknown(document.Identity.UnknownAttrs, document.Identity.Unknown, "identity"); err != nil {
		return Instruction{}, err
	}
	if err := rejectUnknown(document.Invariants.UnknownAttrs, document.Invariants.Unknown, "invariants"); err != nil {
		return Instruction{}, err
	}
	if err := rejectUnknown(document.Output.UnknownAttrs, document.Output.Unknown, "output-contract"); err != nil {
		return Instruction{}, err
	}
	if err := rejectUnknown(document.Tools.UnknownAttrs, document.Tools.Unknown, "tools"); err != nil {
		return Instruction{}, err
	}
	if document.ID != entry.ID || document.Version != entry.Version {
		return Instruction{}, errors.New("registry metadata does not match instruction XML")
	}
	if document.Visibility != "protected" {
		return Instruction{}, errors.New("instruction visibility must be protected")
	}
	if document.Phase == "" || document.Perspective == "" || normalizeText(document.Purpose) == "" {
		return Instruction{}, errors.New("instruction phase, perspective and purpose are required")
	}
	identity := Identity{
		Entity:        normalizeText(document.Identity.Entity),
		Mode:          normalizeText(document.Identity.Mode),
		SeparateAgent: document.Identity.SeparateAgent,
	}
	if identity.Entity != "Sonata" || identity.Mode != "temporary-perspective" || identity.SeparateAgent {
		return Instruction{}, errors.New("instruction violates the single Sonata identity")
	}
	invariants := make([]string, 0, len(document.Invariants.Rules))
	seenRules := make(map[string]struct{}, len(document.Invariants.Rules))
	for _, rule := range document.Invariants.Rules {
		if err := rejectUnknown(rule.UnknownAttrs, rule.Unknown, "rule"); err != nil {
			return Instruction{}, err
		}
		if !stableIDPattern.MatchString(rule.ID) {
			return Instruction{}, fmt.Errorf("invalid invariant ID %q", rule.ID)
		}
		if _, duplicate := seenRules[rule.ID]; duplicate {
			return Instruction{}, fmt.Errorf("duplicate invariant %q", rule.ID)
		}
		seenRules[rule.ID] = struct{}{}
		invariants = append(invariants, rule.ID)
	}
	if len(invariants) == 0 {
		return Instruction{}, errors.New("instruction requires invariants")
	}
	if document.Output.Ref == "" {
		return Instruction{}, errors.New("instruction output contract is required")
	}
	tools, err := validateToolPolicy(document.Phase, document.Tools)
	if err != nil {
		return Instruction{}, err
	}
	return Instruction{
		Metadata:       Metadata{ID: entry.ID, Version: entry.Version, Hash: entry.SHA256},
		Phase:          document.Phase,
		Perspective:    document.Perspective,
		Identity:       identity,
		Purpose:        normalizeText(document.Purpose),
		Invariants:     invariants,
		OutputContract: document.Output.Ref,
		Tools:          tools,
	}, nil
}

func decodeDefaultManifest(data []byte, entry registryEntry) (DefaultManifest, error) {
	if err := preflightXML(data); err != nil {
		return DefaultManifest{}, err
	}
	var document manifestXML
	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = true
	if err := decoder.Decode(&document); err != nil {
		return DefaultManifest{}, err
	}
	if document.XMLName.Local != "sonata-manifest" {
		return DefaultManifest{}, fmt.Errorf("unexpected root element %q", document.XMLName.Local)
	}
	if err := rejectUnknown(document.UnknownAttrs, document.Unknown, "sonata-manifest"); err != nil {
		return DefaultManifest{}, err
	}
	if err := rejectUnknown(document.Expression.UnknownAttrs, document.Expression.Unknown, "expression"); err != nil {
		return DefaultManifest{}, err
	}
	if document.ID != entry.ID || document.Version != entry.Version {
		return DefaultManifest{}, errors.New("registry metadata does not match manifest XML")
	}
	if document.Visibility != "protected" {
		return DefaultManifest{}, errors.New("default manifest visibility must be protected")
	}
	manifest := DefaultManifest{
		Metadata:  Metadata{ID: entry.ID, Version: entry.Version, Hash: entry.SHA256},
		Target:    document.Target,
		Tone:      normalizeText(document.Expression.Tone),
		Focus:     normalizeText(document.Expression.Focus),
		Verbosity: normalizeText(document.Expression.Verbosity),
		Guidance:  normalizeText(document.Guidance),
	}
	if !stableIDPattern.MatchString(manifest.Target) || manifest.Tone == "" || manifest.Focus == "" || manifest.Verbosity == "" || manifest.Guidance == "" {
		return DefaultManifest{}, errors.New("default manifest target and expression fields are required")
	}
	return manifest, nil
}

func validateToolPolicy(phase string, document toolsXML) (ToolPolicy, error) {
	if document.Mode != "none" && document.Mode != "allowlist" {
		return ToolPolicy{}, fmt.Errorf("unsupported tool policy %q", document.Mode)
	}
	allowed := make([]string, 0, len(document.Tools))
	seen := make(map[string]struct{}, len(document.Tools))
	for _, tool := range document.Tools {
		if err := rejectUnknown(tool.UnknownAttrs, tool.Unknown, "tool"); err != nil {
			return ToolPolicy{}, err
		}
		if !stableIDPattern.MatchString(tool.ID) {
			return ToolPolicy{}, fmt.Errorf("invalid tool ID %q", tool.ID)
		}
		if _, duplicate := seen[tool.ID]; duplicate {
			return ToolPolicy{}, fmt.Errorf("duplicate tool ID %q", tool.ID)
		}
		seen[tool.ID] = struct{}{}
		allowed = append(allowed, tool.ID)
	}
	if phase == "synthesis_tooling" {
		if document.Mode != "allowlist" || len(allowed) == 0 {
			return ToolPolicy{}, errors.New("synthesis_tooling requires a non-empty tool allowlist")
		}
	} else if document.Mode != "none" || len(allowed) != 0 {
		return ToolPolicy{}, fmt.Errorf("phase %s cannot use tools", phase)
	}
	return ToolPolicy{Mode: document.Mode, Allowed: allowed}, nil
}

func rejectUnknown(attributes []xml.Attr, elements []unknownXML, context string) error {
	if len(attributes) > 0 {
		return fmt.Errorf("unknown attribute %s on %s", attributes[0].Name.Local, context)
	}
	if len(elements) > 0 {
		return fmt.Errorf("unknown element %s in %s", elements[0].XMLName.Local, context)
	}
	return nil
}

func normalizeText(value string) string { return strings.Join(strings.Fields(value), " ") }

type unknownXML struct {
	XMLName xml.Name
}

type instructionXML struct {
	XMLName     xml.Name `xml:"sonata-instruction"`
	ID          string   `xml:"id,attr"`
	Version     int      `xml:"version,attr"`
	Phase       string   `xml:"phase,attr"`
	Perspective string   `xml:"perspective,attr"`
	Visibility  string   `xml:"visibility,attr"`

	Identity   identityXML   `xml:"identity"`
	Purpose    string        `xml:"purpose"`
	Invariants invariantsXML `xml:"invariants"`
	Output     outputXML     `xml:"output-contract"`
	Tools      toolsXML      `xml:"tools"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type identityXML struct {
	Entity        string `xml:"entity"`
	Mode          string `xml:"mode"`
	SeparateAgent bool   `xml:"separate-agent"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type invariantsXML struct {
	Rules []ruleXML `xml:"rule"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type ruleXML struct {
	ID string `xml:"id,attr"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type outputXML struct {
	Ref string `xml:"ref,attr"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type toolsXML struct {
	Mode  string    `xml:"mode,attr"`
	Tools []toolXML `xml:"tool"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type toolXML struct {
	ID string `xml:"id,attr"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type manifestXML struct {
	XMLName    xml.Name `xml:"sonata-manifest"`
	ID         string   `xml:"id,attr"`
	Version    int      `xml:"version,attr"`
	Target     string   `xml:"target,attr"`
	Visibility string   `xml:"visibility,attr"`

	Expression expressionXML `xml:"expression"`
	Guidance   string        `xml:"guidance"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}

type expressionXML struct {
	Tone      string `xml:"tone"`
	Focus     string `xml:"focus"`
	Verbosity string `xml:"verbosity"`

	UnknownAttrs []xml.Attr   `xml:",any,attr"`
	Unknown      []unknownXML `xml:",any"`
}
