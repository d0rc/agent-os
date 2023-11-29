package agency

import (
	"encoding/json"
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"os"
	"strconv"
	"sync"
	"time"
)

// VoteForAction it's going to be very different from what ancient Greeks thought it should be
// and that's the reason for the file name, nothing else
var votesCache = make(map[string]float32)
var votesCacheLock = sync.RWMutex{}

func (agentState *GeneralAgentInfo) VoteForAction(initialGoal, actionDescription string) (float32, error) {
	votesCacheLock.RLock()
	if _, exists := votesCache[actionDescription]; exists {
		voteValue := votesCache[actionDescription]
		votesCacheLock.RUnlock()
		return voteValue, nil
	}
	votesCacheLock.RUnlock()

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
		Thought   string      `json:"thought"`
		Criticism string      `json:"criticism"`
		Feedback  string      `json:"feedback"`
		Rate      interface{} `json:"rate"`
	}

	minResults := 7
retryVoting:
	serverResponse, err := agentState.Server.RunRequest(&cmds.ClientRequest{
		ProcessName: "action-voter",
		Priority:    borrow_engine.PRIO_User,
		GetCompletionRequests: []cmds.GetCompletionRequest{
			{
				RawPrompt:  systemMessage,
				MinResults: minResults,
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
	listOfRatings := make([]float32, 0)
	numberOfVotes := 0
	for _, getCompletionResponse := range serverResponse.GetCompletionResponse {
		if getCompletionResponse == nil || getCompletionResponse.Choices == nil {
			continue
		}

		for _, choice := range getCompletionResponse.Choices {
			if choice == "" {
				continue
			}
			currentVote := &votersResponse{}
			if err := tools.ParseJSON(choice, func(s string) error {
				return json.Unmarshal([]byte(s), currentVote)
			}); err != nil {
				fmt.Printf("error parsing voter's JSON: %s\n", err)
				continue
			}
			var currentVoteRate float32

			switch currentVote.Rate.(type) {
			case float32:
				currentVoteRate = currentVote.Rate.(float32)
			case float64:
				currentVoteRate = float32(currentVote.Rate.(float64))
			case string:
				tmp, err := strconv.ParseFloat(currentVote.Rate.(string), 32)
				if err != nil {
					fmt.Printf("error parsing vote rate: %s\n", err)
					continue
				}
				currentVoteRate = float32(tmp)
			case int:
				currentVoteRate = float32(currentVote.Rate.(int))
			default:
				fmt.Printf("Don't know what to do with vote rate: %v\n", currentVote.Rate)
			}
			if currentVoteRate >= 0 && currentVoteRate <= 1 {
				fmt.Printf("Strange - Vote rate: %f\n", currentVoteRate)
			}
			currentRating += currentVoteRate
			numberOfVotes++
			listOfRatings = append(listOfRatings, currentVoteRate)
		}
	}

	if minResults < len(serverResponse.GetCompletionResponse[0].Choices) {
		minResults = len(serverResponse.GetCompletionResponse[0].Choices) + 1
	}

	if numberOfVotes == 0 && minResults < 50 {
		minResults += 5
		goto retryVoting
	}

	if numberOfVotes == 0 {
		numberOfVotes = 100
		currentRating = 0
	}

	finalRating := currentRating / float32(numberOfVotes)

	//fmt.Printf("Final rating: %f, number of votes: %d\n", finalRating, numberOfVotes)

	if numberOfVotes >= 5 {
		votesCacheLock.Lock()
		votesCache[actionDescription] = finalRating
		votesCacheLock.Unlock()

		_ = os.MkdirAll("action-voting-cache", os.ModePerm)
		_ = os.WriteFile(fmt.Sprintf("action-voting-cache/%s.md",
			engines.GenerateMessageId(systemMessage)), []byte(fmt.Sprintf(`UUID: %s
%s

Ratings: %v
`, engines.GenerateMessageId(systemMessage), systemMessage, listOfRatings)), os.ModePerm)
	}

	return finalRating, nil
}
