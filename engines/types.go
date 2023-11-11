package engines

import "github.com/d0rc/agent-os/vectors"

type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleSystem    ChatRole = "system"
	ChatRoleAssistant ChatRole = "assistant"
)

type Message struct {
	Role    ChatRole `json:"role"`
	Content string   `json:"content"`
}

type GenerationSettings struct {
	Messages           []Message                  `json:"messages"`
	AfterJoinPrefix    string                     `json:"after_join_prefix"`
	RawPrompt          string                     `json:"raw_prompt"`
	NoCache            bool                       `json:"no_cache"`
	Temperature        float32                    `json:"temperature"`
	StopTokens         []string                   `json:"stop_tokens"`
	BestOf             int                        `json:"best_of"`
	StatisticsCallback func(info *StatisticsInfo) `json:"statistics_callback"`
	MaxRetries         int                        `json:"max_retries"`
}

type StatisticsInfo struct {
	TokensProcessed int
	TokensGenerated int
	PromptTokens    int
}

type JobQueueTask struct {
	Req           *GenerationSettings
	Res           chan *Message
	ResEmbeddings chan *vectors.Vector
}
