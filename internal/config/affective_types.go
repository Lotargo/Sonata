package config

// AffectiveConfig is the versioned configuration contract for the v1
// deterministic affective dynamics engine. The legacy emotion fields remain in
// EmotionConfig until the v0 transition engine is retired.
type AffectiveConfig struct {
	ProfileVersion          string                                    `yaml:"profile_version"`
	IntegrationStep         Duration                                  `yaml:"integration_step"`
	MaxSubsteps             int                                       `yaml:"max_substeps"`
	Personality             AffectivePersonalityConfig                `yaml:"personality"`
	InitialPhysiology       AffectivePhysiologyConfig                 `yaml:"initial_physiology"`
	PhysiologyRecoveryRates map[string]float64                        `yaml:"physiology_recovery_rates"`
	Dynamics                map[string]AffectiveEmotionDynamicsConfig `yaml:"dynamics"`
	PersonalityInfluences   map[string]PersonalityInfluenceConfig     `yaml:"personality_influences"`
	PhysiologyInfluences    map[string]PhysiologyInfluenceConfig      `yaml:"physiology_influences"`
	Interactions            []EmotionInteractionConfig                `yaml:"interactions"`
	Stimuli                 map[string]StimulusDefinitionConfig       `yaml:"stimuli"`
	Drives                  map[string]DriveDefinitionConfig          `yaml:"drives"`
	ComplexStates           map[string]ComplexStateDefinitionConfig   `yaml:"complex_states"`
}

type AffectivePersonalityConfig struct {
	Openness          float64 `yaml:"openness"`
	Conscientiousness float64 `yaml:"conscientiousness"`
	Extraversion      float64 `yaml:"extraversion"`
	Agreeableness     float64 `yaml:"agreeableness"`
	Neuroticism       float64 `yaml:"neuroticism"`
	Sensitivity       float64 `yaml:"sensitivity"`
	EmotionalInertia  float64 `yaml:"emotional_inertia"`
	RecoveryCapacity  float64 `yaml:"recovery_capacity"`
}

type AffectivePhysiologyConfig struct {
	Fatigue    float64 `yaml:"fatigue"`
	Arousal    float64 `yaml:"arousal"`
	Energy     float64 `yaml:"energy"`
	StressLoad float64 `yaml:"stress_load"`
	Stability  float64 `yaml:"stability"`
}

type AffectiveEmotionDynamicsConfig struct {
	ExcitationGain   float64 `yaml:"excitation_gain"`
	Persistence      float64 `yaml:"persistence"`
	Ceiling          float64 `yaml:"ceiling"`
	MaxPositiveDelta float64 `yaml:"max_positive_delta"`
	MaxNegativeDelta float64 `yaml:"max_negative_delta"`
}

type PersonalityInfluenceConfig struct {
	Openness          float64 `yaml:"openness"`
	Conscientiousness float64 `yaml:"conscientiousness"`
	Extraversion      float64 `yaml:"extraversion"`
	Agreeableness     float64 `yaml:"agreeableness"`
	Neuroticism       float64 `yaml:"neuroticism"`
}

type PhysiologyInfluenceConfig struct {
	Fatigue    float64 `yaml:"fatigue"`
	Arousal    float64 `yaml:"arousal"`
	Energy     float64 `yaml:"energy"`
	StressLoad float64 `yaml:"stress_load"`
	Stability  float64 `yaml:"stability"`
}

type EmotionInteractionConfig struct {
	From   string  `yaml:"from"`
	To     string  `yaml:"to"`
	Mode   string  `yaml:"mode"`
	Weight float64 `yaml:"weight"`
}

type StimulusDefinitionConfig struct {
	DefinitionID        string             `yaml:"definition_id"`
	EmotionEffects      map[string]float64 `yaml:"emotion_effects"`
	RelationshipEffects map[string]float64 `yaml:"relationship_effects"`
	PhysiologyEffects   map[string]float64 `yaml:"physiology_effects"`
}

type DriveDefinitionConfig struct {
	DefinitionID        string             `yaml:"definition_id"`
	Baseline            float64            `yaml:"baseline"`
	InitialSatisfaction float64            `yaml:"initial_satisfaction"`
	GrowthRate          float64            `yaml:"growth_rate"`
	SatisfactionMap     map[string]float64 `yaml:"satisfaction_map"`
	EmotionEffects      map[string]float64 `yaml:"emotion_effects"`
}

type ComplexStateDefinitionConfig struct {
	DefinitionID     string             `yaml:"definition_id"`
	EntryConditions  []ConditionConfig  `yaml:"entry_conditions"`
	ExitConditions   []ConditionConfig  `yaml:"exit_conditions"`
	MinEntryDuration Duration           `yaml:"min_entry_duration"`
	MinExitDuration  Duration           `yaml:"min_exit_duration"`
	EntryThreshold   float64            `yaml:"entry_threshold"`
	ExitThreshold    float64            `yaml:"exit_threshold"`
	Effects          StateEffectsConfig `yaml:"effects"`
}

type ConditionConfig struct {
	Signal    string  `yaml:"signal"`
	Operator  string  `yaml:"operator"`
	Threshold float64 `yaml:"threshold"`
	Weight    float64 `yaml:"weight"`
}

type StateEffectsConfig struct {
	EmotionGainModifiers          map[string]float64 `yaml:"emotion_gain_modifiers"`
	EmotionRecoveryModifiers      map[string]float64 `yaml:"emotion_recovery_modifiers"`
	EmotionPersistenceModifiers   map[string]float64 `yaml:"emotion_persistence_modifiers"`
	EmotionCeilingModifiers       map[string]float64 `yaml:"emotion_ceiling_modifiers"`
	EmotionInhibitionModifiers    map[string]float64 `yaml:"emotion_inhibition_modifiers"`
	EmotionTargetShifts           map[string]float64 `yaml:"emotion_target_shifts"`
	PhysiologyRecoveryModifiers   map[string]float64 `yaml:"physiology_recovery_modifiers"`
	RelationshipRecoveryModifiers map[string]float64 `yaml:"relationship_recovery_modifiers"`
	DriveUrgencyModifiers         map[string]float64 `yaml:"drive_urgency_modifiers"`
	ReportBiases                  []string           `yaml:"report_biases"`
}
