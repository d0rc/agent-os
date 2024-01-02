package agency

import (
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/tools"
	zlog "github.com/rs/zerolog/log"
	"os"
	"strings"
)

func (agentState *GeneralAgentInfo) visitTerminalMessage(messages []*engines.Message) bool {
	// first let's check how many times we've been here
	chainSignature := getChatSignature(messages)

	agentState.terminalsLock.Lock()
	defer agentState.terminalsLock.Unlock()

	timesVisited, exists := agentState.terminalsVisitsMap[chainSignature]
	if exists && timesVisited > 4 {
		// we've been here more than 5 times, let's remove it
		return false
	}

	votesRating, exists := agentState.terminalsVotesMap[chainSignature]
	if exists && votesRating < 5.0 {
		// it's not worth visiting at all
		return false
	}

	// in any other case - start voting...!
	if messages[len(messages)-1].Role == engines.ChatRoleAssistant {
		return false
	} else {
		agentState.terminalsVotesMap[chainSignature] = 10 // it's real-world input, don't ignore just yet...!
		agentState.terminalsVisitsMap[chainSignature] = timesVisited + 1
	}

	// if we've got here, we can go on...!
	agentState.jobsChannel <- &cmds.ClientRequest{
		ProcessName: agentState.SystemName,
		Priority:    borrow_engine.PRIO_User,
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:   chatToRawPrompt(messages),
				MinResults:  9,
				Temperature: 0.9,
			},
		},
		CorrelationId: *messages[len(messages)-1].ID,
	}

	//exportMessageChain(agentState.SystemName, messages)

	return true
}

func exportMessageChain(name string, messages []*engines.Message) {
	_ = os.MkdirAll("../message-chains/", os.ModePerm)
	path := fmt.Sprint("../message-chains/", name, ".md")

	runesToRemove := []string{"\n", "\t", "{", "}", "(", ")", "\"", "'", "  "}

	graphData := ""
	for idx, message := range messages {
		content := message.Content
		for _, runeToRemove := range runesToRemove {
			content = strings.ReplaceAll(content, runeToRemove, " ")
		}
		graphData += fmt.Sprintf("%s(%s)", *message.ID, tools.CutStringAt(content, 15))
		if idx < len(messages)-1 {
			graphData += " --> "
		}
	}
	graphData += "\n"

	// now, append string graphData to file, create if doesn't exist
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		zlog.Error().Err(err).Str("path", path).Msg("failed to open file")
		return
	}
	defer f.Close()

	_, err = f.WriteString(graphData)
	if err != nil {
		zlog.Error().Err(err).Str("path", path).Msg("failed to write to file")
		return
	}
}
