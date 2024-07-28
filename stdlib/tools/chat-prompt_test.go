package tools

import (
	"github.com/d0rc/agent-os/engines"
	"math/rand"
	"testing"
)

func TestMessagesSerialization(t *testing.T) {
	for i := 0; i < 1_000; i++ {
		originalMessages := generateRandomMessages()
		serializedString := MessagesToString(originalMessages)
		deserializedMessages := StringToMessages(serializedString)

		if !messagesEqual(originalMessages, deserializedMessages) {
			t.Errorf("Test %d failed: Original and deserialized messages do not match", i+1)
		}
	}
}

func generateRandomMessages() []*engines.Message {
	roles := []string{"user", "system", "assistant"}
	contents := []string{
		"Hello, world!",
		"This is a test message.",
		"Another random message.",
	}

	numMessages := rand.Intn(10) + 1 // Generate between 1 and 10 messages
	messages := make([]*engines.Message, numMessages)

	for i := range messages {
		role := roles[rand.Intn(len(roles))]
		content := contents[rand.Intn(len(contents))]
		messages[i] = &engines.Message{Role: engines.ChatRole(role), Content: content}
	}

	return messages
}

func messagesEqual(a []*engines.Message, b []*engines.Message) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Role != b[i].Role || a[i].Content != b[i].Content {
			return false
		}
	}
	return true
}
