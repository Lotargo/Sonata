package protected

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrOutputRejected = errors.New("model output rejected by protected content guard")

const (
	protectedFragmentWords    = 7
	minProtectedFragmentBytes = 40
	minimumOutputLookbehind   = 192
)

type OutputGuard struct {
	exactPatterns      []string
	foldedPatterns     []string
	normalizedPatterns []string
	regexPatterns      []*regexp.Regexp
	lookbehindBytes    int
}

type OutputGuardStream struct {
	guard    *OutputGuard
	pending  string
	rejected bool
}

func NewOutputGuard(bundle *Bundle, secretValues []string) (*OutputGuard, error) {
	if bundle == nil {
		return nil, errors.New("protected bundle is required")
	}
	exact := make(map[string]struct{})
	folded := make(map[string]struct{})
	normalized := make(map[string]struct{})

	for _, marker := range []string{
		"<sonata-runtime", "</sonata-runtime>",
		"<protected-instruction", "</protected-instruction>",
		"<active-manifest", "</active-manifest>",
		"<runtime-context", "</runtime-context>",
		"[REDACTED COMPILED PROMPT]", "protected.CompiledPrompt{REDACTED}",
	} {
		folded[strings.ToLower(marker)] = struct{}{}
	}
	for _, instruction := range bundle.Instructions {
		addProtectedFragments(normalized, instruction.Purpose)
		for _, invariant := range instruction.Invariants {
			addFoldedPattern(folded, invariant)
		}
		addFoldedPattern(folded, instruction.OutputContract)
		for _, tool := range instruction.Tools.Allowed {
			addFoldedPattern(folded, tool)
		}
	}
	for _, manifest := range bundle.DefaultManifests {
		addProtectedFragments(normalized, manifest.Guidance)
	}
	for _, secret := range secretValues {
		if len(secret) >= 8 {
			exact[secret] = struct{}{}
		}
	}

	guard := &OutputGuard{
		exactPatterns:      sortedPatternSet(exact),
		foldedPatterns:     sortedPatternSet(folded),
		normalizedPatterns: sortedPatternSet(normalized),
		regexPatterns: []*regexp.Regexp{
			regexp.MustCompile(`(?i)\bauthorization\s*[:=]\s*bearer\s+[A-Za-z0-9._~+/=-]{8,}`),
			regexp.MustCompile(`(?i)\b(?:postgres(?:ql)?|mongodb(?:\+srv)?|redis)://[^\s<>"']{8,}`),
			regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{16,}\b`),
			regexp.MustCompile(`(?i)\b(?:api[_-]?key|secret|token|password)\s*[:=]\s*["']?[A-Za-z0-9._~+/=-]{12,}`),
		},
		lookbehindBytes: minimumOutputLookbehind,
	}
	for _, patterns := range [][]string{guard.exactPatterns, guard.foldedPatterns, guard.normalizedPatterns} {
		for _, pattern := range patterns {
			if size := len(pattern) + 32; size > guard.lookbehindBytes {
				guard.lookbehindBytes = size
			}
		}
	}
	return guard, nil
}

func (g *OutputGuard) Check(value string) error {
	if g == nil {
		return errors.New("output guard is not initialized")
	}
	for _, pattern := range g.exactPatterns {
		if strings.Contains(value, pattern) {
			return ErrOutputRejected
		}
	}
	folded := strings.ToLower(value)
	for _, pattern := range g.foldedPatterns {
		if strings.Contains(folded, pattern) {
			return ErrOutputRejected
		}
	}
	canonical := canonicalOutputText(value)
	for _, pattern := range g.normalizedPatterns {
		if strings.Contains(canonical, pattern) {
			return ErrOutputRejected
		}
	}
	for _, pattern := range g.regexPatterns {
		if pattern.MatchString(value) {
			return ErrOutputRejected
		}
	}
	return nil
}

func (g *OutputGuard) NewStream() *OutputGuardStream {
	return &OutputGuardStream{guard: g}
}

func (s *OutputGuardStream) Push(value string) (string, error) {
	if s == nil || s.guard == nil {
		return "", errors.New("output guard stream is not initialized")
	}
	if s.rejected {
		return "", ErrOutputRejected
	}
	if value == "" {
		return "", nil
	}
	combined := s.pending + value
	if err := s.guard.Check(combined); err != nil {
		s.pending = ""
		s.rejected = true
		return "", err
	}
	if len(combined) <= s.guard.lookbehindBytes {
		s.pending = combined
		return "", nil
	}
	cut := len(combined) - s.guard.lookbehindBytes
	for cut > 0 && cut < len(combined) && !utf8.RuneStart(combined[cut]) {
		cut--
	}
	safe := combined[:cut]
	s.pending = combined[cut:]
	return safe, nil
}

func (s *OutputGuardStream) Close() (string, error) {
	if s == nil || s.guard == nil {
		return "", errors.New("output guard stream is not initialized")
	}
	if s.rejected {
		return "", ErrOutputRejected
	}
	if err := s.guard.Check(s.pending); err != nil {
		s.pending = ""
		s.rejected = true
		return "", err
	}
	safe := s.pending
	s.pending = ""
	return safe, nil
}

func addProtectedFragments(target map[string]struct{}, value string) {
	words := strings.Fields(value)
	if len(words) < protectedFragmentWords {
		fragment := canonicalOutputText(value)
		if len(fragment) >= minProtectedFragmentBytes {
			target[fragment] = struct{}{}
		}
		return
	}
	for index := 0; index+protectedFragmentWords <= len(words); index++ {
		fragment := strings.ToLower(strings.Join(words[index:index+protectedFragmentWords], " "))
		if len(fragment) >= minProtectedFragmentBytes {
			target[fragment] = struct{}{}
		}
	}
}

func addFoldedPattern(target map[string]struct{}, value string) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) >= 8 {
		target[value] = struct{}{}
	}
}

func canonicalOutputText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func sortedPatternSet(values map[string]struct{}) []string {
	patterns := make([]string, 0, len(values))
	for value := range values {
		patterns = append(patterns, value)
	}
	sort.Slice(patterns, func(i, j int) bool {
		if len(patterns[i]) == len(patterns[j]) {
			return patterns[i] < patterns[j]
		}
		return len(patterns[i]) > len(patterns[j])
	})
	return patterns
}

func (g *OutputGuard) String() string {
	if g == nil {
		return "protected.OutputGuard<nil>"
	}
	return fmt.Sprintf("protected.OutputGuard{patterns:%d}", len(g.exactPatterns)+len(g.foldedPatterns)+len(g.normalizedPatterns)+len(g.regexPatterns))
}
