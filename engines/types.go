package engines

import (
	"crypto/sha512"
	"github.com/d0rc/agent-os/vectors"
	"github.com/google/uuid"
	"sync"
)

type ChatRole string

const (
	ChatRoleUser      ChatRole = "user"
	ChatRoleSystem    ChatRole = "system"
	ChatRoleAssistant ChatRole = "assistant"
)

type Message struct {
	ID       *string             `json:"id,omitempty"`
	ReplyTo  map[string]struct{} `json:"reply_to,omitempty"`
	MetaInfo interface{}         `json:"meta,omitempty"`
	Role     ChatRole            `json:"role"`
	Content  string              `json:"content"`
	lock     sync.RWMutex
}

func (m *Message) Lock() {
	m.lock.Lock()
}

func (m *Message) Unlock() {
	m.lock.Unlock()
}

func (m *Message) RLock() {
	m.lock.RLock()
}

func (m *Message) RUnlock() {
	m.lock.RUnlock()
}

func NewMessage(role ChatRole, content string) *Message {
	id := GenerateMessageId(content)
	return &Message{
		ID:      &id,
		Content: content,
		Role:    role,
	}
}

func GenerateMessageId(body string) string {
	return uuid.NewHash(sha512.New(), uuid.Nil, []byte(body), 5).String()
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
