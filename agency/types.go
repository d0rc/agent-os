package agency

type Agent interface {
	GeneratePrompt() string
}

type AgentConfig struct {
	Name        string                  `yaml:"name"`
	InputSink   string                  `yaml:"input-sink"`
	PromptBased *PromptBasedAgentConfig `yaml:"prompt-based"`
}

type PromptBasedAgentConfig struct {
	Prompt          string        `yaml:"prompt"`
	ResponseFormat  interface{}   `yaml:"response-format"`
	LifeCycleType   LifeCycleType `yaml:"life-cycle-type"`
	LifeCycleLength int           `yaml:"life-cycle-length"`
}

type LifeCycleType string

const LifeCycleSingleShot LifeCycleType = "single-shot"
