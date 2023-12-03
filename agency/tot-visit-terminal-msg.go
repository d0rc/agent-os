package agency

import (
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
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
		/*
			votes, err := agentState.VoteForAction(messages[0].Content, messages[len(messages)-1].Content)
			if err != nil {
				return
			}

			agentState.terminalsVotesMap[chainSignature] = votes
			agentState.terminalsVisitsMap[chainSignature] = timesVisited + 1
		*/
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

	return true
}
