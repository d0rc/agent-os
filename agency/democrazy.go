package agency

import (
	"encoding/json"
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"time"
)

// VoteForAction it's going to be very different from what ancient Greeks thought it should be
// and that's the reason for the file name, nothing else
func (agentState *GeneralAgentInfo) VoteForAction(initialGoal, actionDescription string) (float32, error) {
	systemMessage := `Given goal:

%s

And a command:

%s

How likely is executing the command will lead to achieving the goal?
Respond in the JSON format:
{
    "thought": "thought text, which provides critics of possible solutions",
    "criticism": "constructive self-criticism, question your assumptions",
    "feedback": "provide your feedback on the command and it's alignment to the purpose, suggest refinements here",
    "rate": "rate probability on scale from 0 to 10"
}`

	systemMessage = fmt.Sprintf(systemMessage, "\n```\n"+initialGoal+"\n```\n", "\n```\n"+actionDescription+"\n```\n")
	type votersResponse struct {
		Thought   string  `json:"thought"`
		Criticism string  `json:"criticism"`
		Feedback  string  `json:"feedback"`
		Rate      float32 `json:"rate"`
	}

	serverResponse, err := agentState.Server.RunRequest(&cmds.ClientRequest{
		ProcessName: "action-voter",
		Priority:    borrow_engine.PRIO_User,
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:  systemMessage,
				MinResults: 5,
			},
		},
	}, 120*time.Second, os_client.REP_IO)
	if err != nil {
		return 0, fmt.Errorf("error running voters inference request: %w", err)
	}

	if serverResponse.GetCompletionResponse == nil || len(serverResponse.GetCompletionResponse) == 0 {
		return 0, fmt.Errorf("no completions returned")
	}

	currentRating := float32(0)
	numberOfVotes := 0
	for _, getCompletionResponse := range serverResponse.GetCompletionResponse {
		if getCompletionResponse == nil || getCompletionResponse.Choices == nil {
			continue
		}

		for _, choice := range getCompletionResponse.Choices {
			currentVote := &votersResponse{}
			if err := tools.ParseJSON(choice, func(s string) error {
				return json.Unmarshal([]byte(s), currentVote)
			}); err != nil {
				fmt.Printf("error parsing voter's JSON: %s\n", err)
				continue
			}

			currentRating += currentVote.Rate
			numberOfVotes++
		}
	}

	finalRating := currentRating / float32(numberOfVotes)

	fmt.Printf("Final rating: %f, number of votes: %d\n", finalRating, numberOfVotes)

	return finalRating, nil
}
