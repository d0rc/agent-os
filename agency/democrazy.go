package agency

import (
	"encoding/json"
	"fmt"
	borrow_engine "github.com/d0rc/agent-os/borrow-engine"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
	"github.com/d0rc/agent-os/tools"
	"github.com/tidwall/gjson"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// VoteForAction it's going to be very different from what ancient Greeks thought it should be
// and that's the reason for the file name, nothing else
var votesCache = make(map[string]float32)
var votesCacheLock = sync.RWMutex{}

func (agentState *GeneralAgentInfo) VoteForAction(initialGoal, actionDescription string) (float32, error) {
	if strings.Contains(actionDescription, "final-report") ||
		strings.Contains(actionDescription, "interim-report") {
		return 10, nil
	}

	votesCacheLock.RLock()
	if _, exists := votesCache[actionDescription]; exists {
		voteValue := votesCache[actionDescription]
		votesCacheLock.RUnlock()
		return voteValue, nil
	}
	votesCacheLock.RUnlock()

	question := "How likely is it that executing the command will lead to achieving the goal?"
	systemMessage := `Given goal:
%s
And a command:
%s
%s
Respond in the JSON format:
{
    "thought": "thought text, which provides critics of possible solutions",
    "criticism": "constructive self-criticism, question your assumptions",
    "feedback": "provide your feedback on the command and it's alignment to the purpose, suggest refinements here",
    "rate": "rate probability on scale from 1 to 5"
}`

	systemMessage = fmt.Sprintf(systemMessage, "\n```\n"+initialGoal+"\n```\n", "\n```\n"+actionDescription+"\n```\n", question)
	type votersResponse struct {
		Thought   string      `json:"thought"`
		Criticism string      `json:"criticism"`
		Feedback  string      `json:"feedback"`
		Rate      interface{} `json:"rate"`
	}

	minResults := VoterMinResults
retryVoting:
	serverResponse, err := agentState.Server.RunRequest(&cmds.ClientRequest{
		ProcessName: "action-voter",
		Priority:    borrow_engine.PRIO_User,
		GetCompletionRequests: tools.Replicate(
			cmds.GetCompletionRequest{
				RawPrompt:  systemMessage,
				MinResults: minResults,
			}, minResults),
	}, 120*time.Second, os_client.REP_Default)
	if err != nil {
		return 0, fmt.Errorf("error running voters inference request: %w", err)
	}

	if serverResponse.GetCompletionResponse == nil || len(serverResponse.GetCompletionResponse) == 0 {
		return 0, fmt.Errorf("no completions returned")
	}

	currentRating := float32(0)
	listOfRatings := make([]float32, 0)
	numberOfVotes := 0
	for _, choice := range tools.FlattenChoices(serverResponse.GetCompletionResponse) {
		if choice == "" {
			continue
		}
		var rateValue, thoughtsValue, criticismValue, feedbackValue string
		var parsedVoteString string
		if err := tools.ParseJSON(choice, func(s string) error {
			rateValue = gjson.Get(choice, "rate").String()
			thoughtsValue = gjson.Get(choice, "thought").String()
			criticismValue = gjson.Get(choice, "criticism").String()
			feedbackValue = gjson.Get(choice, "feedback").String()

			if rateValue == "" {
				testFmt := []string{"\"rate\": \"%d\"", "\"rate\": %d"}
				for i := 0; i <= 5; i++ {
					found := false
					for _, test := range testFmt {
						if strings.Contains(choice, fmt.Sprintf(test, i)) {
							rateValue = strconv.Itoa(i)
							found = true
							break
						}
					}

					if found {
						break
					}
				}
			}

			if rateValue == "" {
				return fmt.Errorf("no rateValue parsed: ```\n%s\n```", choice)
			} else {
				reconstructedBytes, _ := json.Marshal(&votersResponse{
					Thought:   thoughtsValue,
					Criticism: criticismValue,
					Feedback:  feedbackValue,
					Rate:      rateValue,
				})
				parsedVoteString = string(reconstructedBytes)
				return nil
			}
		}); err != nil {
			fmt.Printf("error parsing voter's JSON: %s\n", err)
			continue
		}
		var currentVoteRate float32
		tmp, err := strconv.ParseFloat(rateValue, 32)
		if err != nil {
			fmt.Printf("error parsing vote rate: %s\n", err)
			continue
		}
		currentVoteRate = float32(tmp)

		if WriteVotesLog {
			/*appendFile("voting.log", fmt.Sprintf("\nStated goal: ```%s```\n\nAction description:\n```json\n%s\n```\n\nQuestion: ```%s```\nVoting result: \n```json\n%s\n```\nRating: %f\n\n",
				initialGoal,
				strings.TrimSpace(actionDescription),
				question,
				strings.TrimSpace(parsedVoteString),
				currentVoteRate,
			))*/
			exportVoterTrainingData(agentState.SystemName, initialGoal, actionDescription, parsedVoteString, currentVoteRate)
		}
		currentRating += currentVoteRate
		numberOfVotes++
		listOfRatings = append(listOfRatings, currentVoteRate)
	}

	if minResults < len(serverResponse.GetCompletionResponse[0].Choices) {
		minResults = len(serverResponse.GetCompletionResponse[0].Choices) + 1
	}

	if numberOfVotes < MinimumNumberOfVotes && minResults < 50 {
		minResults += 5
		goto retryVoting
	}

	if numberOfVotes == 0 {
		numberOfVotes = 100
		currentRating = 0
	}

	finalRating := currentRating / float32(numberOfVotes)

	//fmt.Printf("Final rating: %f, number of votes: %d\n", finalRating, numberOfVotes)

	if numberOfVotes >= NumberOfVotesToCache {
		votesCacheLock.Lock()
		votesCache[actionDescription] = finalRating
		votesCacheLock.Unlock()

		/*
					_ = os.MkdirAll("action-voting-cache", os.ModePerm)
					_ = os.WriteFile(fmt.Sprintf("action-voting-cache/%s.md",
						engines.GenerateMessageId(systemMessage)), []byte(fmt.Sprintf(`UUID: %s
			%s

			Ratings: %v
			`, engines.GenerateMessageId(systemMessage), systemMessage, listOfRatings)), os.ModePerm)

		*/
	}

	return finalRating, nil
}

func exportVoterTrainingData(agentName, goal string, description string, voteString string, rate float32) {
	type voterTrainingDataRecord struct {
		AgentName string  `json:"agent-name"`
		Goal      string  `json:"goal"`
		Action    string  `json:"action"`
		Vote      string  `json:"vote"`
		Rate      float32 `json:"rate"`
	}

	_ = os.MkdirAll("../voter-training-data/", os.ModePerm)
	data := voterTrainingDataRecord{
		AgentName: agentName,
		Goal:      goal,
		Action:    description,
		Vote:      voteString,
		Rate:      rate,
	}

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		fmt.Printf("error marshalling voter training data: %s\n", err)
		return
	}

	_ = os.WriteFile(fmt.Sprintf("../voter-training-data/%s.json", engines.GenerateMessageId(goal+description+voteString)), jsonBytes, os.ModePerm)
}
