package tools

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

func NewChatPromptWithMessages(messages []*engines.Message) *ChatPrompt {
	return &ChatPrompt{
		messages: messages,
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

func (p *ChatPrompt) DefString() string {
	return p.String(PSChatML)
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
	} else if style == PSChatML {
		for _, m := range p.messages {
			if m == nil {
				continue
			}
			switch m.Role {
			case engines.ChatRoleSystem:
				finalPrompt.WriteString("<|im_start|>system\n")
				finalPrompt.WriteString(m.Content)
				finalPrompt.WriteString("<|im_end|>")
			case engines.ChatRoleAssistant:
				finalPrompt.WriteString("<|im_start|>assistant\n")
				finalPrompt.WriteString(m.Content)
				finalPrompt.WriteString("<|im_end|>")
			case engines.ChatRoleUser:
				finalPrompt.WriteString("<|im_start|>user\n")
				finalPrompt.WriteString(m.Content)
				finalPrompt.WriteString("<|im_end|>")
			}
		}

		finalPrompt.WriteString("<im|start|>assistant\n")
	}

	return finalPrompt.String()
}
