package agency

type SimplePromptBasedGeneratingAgent struct {
	Prompt          string        `yaml:"prompt"`
	LifeCycleType   LifeCycleType `yaml:"life-cycle-type"`
	LifeCycleLength int           `yaml:"life-cycle-length"`
	ResponseFormat  interface{}   `yaml:"response-format"`
}

func NewSimplePromptBasedGeneratingAgent(config *AgentConfig) (*SimplePromptBasedGeneratingAgent, error) {
	return &SimplePromptBasedGeneratingAgent{
		Prompt:          config.PromptBased.Prompt,
		LifeCycleType:   config.PromptBased.LifeCycleType,
		LifeCycleLength: config.PromptBased.LifeCycleLength,
		ResponseFormat:  config.PromptBased.ResponseFormat,
	}, nil
}

func (agent *SimplePromptBasedGeneratingAgent) GeneratePrompt() string {
	return ""
}
