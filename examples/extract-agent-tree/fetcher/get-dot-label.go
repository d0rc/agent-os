package fetcher

import (
	"fmt"
	"github.com/d0rc/agent-os/tools"
	"strings"
)

func getLabel(message DbMessage) string {
	content := message.Content
	runesToRemove := []string{"\n", "\t", "{", "}", "(", ")", "\"", "'", "  "}
	for _, runeToRemove := range runesToRemove {
		content = strings.ReplaceAll(content, runeToRemove, " ")
	}
	content = strings.TrimSpace(content)

	return fmt.Sprint(tools.CutStringAt(content, 75))
}
