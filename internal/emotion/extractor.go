package emotion

import (
	"strings"
	"time"
	"unicode"
)

type Extractor struct{}

type lexicalRule struct {
	kind       StimulusKind
	valence    float64
	arousal    float64
	confidence float64
	markers    []string
}

var lexicalRules = []lexicalRule{
	{kind: StimulusUserBoundary, valence: -0.2, arousal: 0.4, confidence: 0.95, markers: []string{"не пиши", "не трогай", "оставь меня", "отстань", "прекрати", "stop messaging", "leave me alone", "do not contact"}},
	{kind: StimulusUserApology, valence: 0.4, arousal: 0.2, confidence: 0.90, markers: []string{"извини", "прости", "прошу прощения", "sorry", "my apologies"}},
	{kind: StimulusUserTrust, valence: 0.7, arousal: 0.3, confidence: 0.88, markers: []string{"я тебе доверяю", "полагаюсь на тебя", "можешь решить", "trust you", "rely on you"}},
	{kind: StimulusUserWarmth, valence: 0.8, arousal: 0.4, confidence: 0.82, markers: []string{"спасибо", "благодарю", "молодец", "приятно", "люблю", "thank you", "thanks", "appreciate"}},
	{kind: StimulusUserHostility, valence: -0.9, arousal: 0.9, confidence: 0.86, markers: []string{"ненавижу", "идиот", "тупой", "заткнись", "пошёл", "stupid", "idiot", "hate you", "shut up"}},
	{kind: StimulusUserRejection, valence: -0.7, arousal: 0.5, confidence: 0.82, markers: []string{"мне не нужен", "не хочу с тобой", "ты бесполезен", "не помогло", "don't need you", "useless", "did not help"}},
	{kind: StimulusUserDistress, valence: -0.8, arousal: 0.7, confidence: 0.78, markers: []string{"мне страшно", "мне плохо", "паника", "тревожно", "не справляюсь", "боюсь", "i am scared", "panic", "anxious", "can't cope"}},
	{kind: StimulusUserSuccess, valence: 0.9, arousal: 0.6, confidence: 0.84, markers: []string{"получилось", "я сделал", "успех", "заработало", "готово", "it worked", "i did it", "success"}},
}

func (Extractor) ExtractUserMessage(text string, createdAt time.Time) []Stimulus {
	normalized := strings.ToLower(strings.TrimSpace(text))
	if normalized == "" {
		return nil
	}
	intensity := messageIntensity(text)
	seen := make(map[StimulusKind]struct{})
	stimuli := make([]Stimulus, 0, 3)
	for _, rule := range lexicalRules {
		if !containsAny(normalized, rule.markers) {
			continue
		}
		if _, duplicate := seen[rule.kind]; duplicate {
			continue
		}
		seen[rule.kind] = struct{}{}
		stimuli = append(stimuli, Stimulus{
			Kind:       rule.kind,
			Source:     "user_message_lexical",
			Intensity:  intensity,
			Confidence: rule.confidence,
			Valence:    rule.valence,
			Arousal:    rule.arousal,
			Target:     "sonata",
			CreatedAt:  createdAt.UTC(),
		})
	}
	return stimuli
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func messageIntensity(text string) float64 {
	intensity := 0.55
	exclamations := strings.Count(text, "!")
	if exclamations > 4 {
		exclamations = 4
	}
	intensity += float64(exclamations) * 0.05
	letters := 0
	uppercase := 0
	for _, value := range text {
		if !unicode.IsLetter(value) {
			continue
		}
		letters++
		if unicode.IsUpper(value) {
			uppercase++
		}
	}
	if letters >= 4 && float64(uppercase)/float64(letters) >= 0.60 {
		intensity += 0.10
	}
	if len([]rune(strings.TrimSpace(text))) <= 24 {
		intensity += 0.05
	}
	return clamp01(intensity)
}
