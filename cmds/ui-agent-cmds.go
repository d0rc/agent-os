package cmds

import "github.com/d0rc/agent-os/engines"

type UIAgentGetMessage struct {
	AgentID             string            `json:"agent-id"`
	Messages            []engines.Message `json:"messages"`
	InlineButton        *string           `json:"inline-button"`
	DocumentCollections []string          `json:"rag-ids"`
}

type UIAgentGetMessageResponse struct {
	Message        engines.Message `json:"message"`
	VisibleMessage string          `json:"visible-message"`
	InlineButtons  []string        `json:"inline-buttons"`
	Error          string          `json:"error"`
}
