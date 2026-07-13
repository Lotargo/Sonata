package protected

type Kind string

const (
	KindInstruction     Kind = "instruction"
	KindDefaultManifest Kind = "default_manifest"
)

type Metadata struct {
	ID      string
	Version int
	Hash    string
}

type Identity struct {
	Entity        string
	Mode          string
	SeparateAgent bool
}

type ToolPolicy struct {
	Mode    string
	Allowed []string
}

type Instruction struct {
	Metadata
	Phase          string
	Perspective    string
	Identity       Identity
	Purpose        string
	Invariants     []string
	OutputContract string
	Tools          ToolPolicy
}

type DefaultManifest struct {
	Metadata
	Target    string
	Tone      string
	Focus     string
	Verbosity string
	Guidance  string
}

type Bundle struct {
	Instructions     map[string]Instruction
	DefaultManifests map[string]DefaultManifest
}
