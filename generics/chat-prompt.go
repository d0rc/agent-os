package generics

import (
	"github.com/d0rc/agent-os/engines"
	"strings"
)

type PromptStyle string

const (
	PSAlpaca PromptStyle = "alpaca"
	PSChatML PromptStyle = "chat-ml"
)

type ChatPrompt struct {
	messages []*engines.Message
}

func NewChatPrompt() *ChatPrompt {
	return &ChatPrompt{
		messages: make([]*engines.Message, 0),
	}
}

func (p *ChatPrompt) AddSystem(systemMessage string) *ChatPrompt {
	p.messages = append(p.messages, &engines.Message{
		Role:    engines.ChatRoleSystem,
		Content: systemMessage,
	})
	return p
}

func (p *ChatPrompt) AddUser(userMessage string) *ChatPrompt {
	p.messages = append(p.messages, &engines.Message{
		Role:    engines.ChatRoleUser,
		Content: userMessage,
	})
	return p
}

func (p *ChatPrompt) AddAssistant(assistantMessage string) *ChatPrompt {
	p.messages = append(p.messages, &engines.Message{
		Role:    engines.ChatRoleAssistant,
		Content: assistantMessage,
	})
	return p
}

func (p *ChatPrompt) AddMessage(msg *engines.Message) *ChatPrompt {
	p.messages = append(p.messages, msg)
	return p
}

func (p *ChatPrompt) String(style PromptStyle) string {
	finalPrompt := strings.Builder{}
	if style == PSAlpaca {
		for _, m := range p.messages {
			switch m.Role {
			case engines.ChatRoleSystem:
				finalPrompt.WriteString("### Instruction:\n")
			case engines.ChatRoleAssistant:
				finalPrompt.WriteString("### Assistant:\n")
			case engines.ChatRoleUser:
				finalPrompt.WriteString("### User:\n")
			}

			finalPrompt.WriteString(m.Content)
			finalPrompt.WriteString("\n")
		}

		finalPrompt.WriteString("### Assistant:\n")
	}

	return finalPrompt.String()
}
