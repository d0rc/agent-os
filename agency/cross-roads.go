package agency

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"strings"
)

func (agentState *GeneralAgentInfo) applyCrossRoadsMagic(options []*engines.Message) []*engines.Message {
	if len(options) == 0 {
		return options
	}

	allMessagesFromAssistant := true
	for _, msg := range options {
		if msg.Role != engines.ChatRoleAssistant {
			allMessagesFromAssistant = false
			break
		}
	}

	if !allMessagesFromAssistant {
		// no obvious policy to choose from
		return options
	}

	prompt := agentState.Settings.Agent.PromptBased.Prompt
	promptLines := strings.Split(prompt, "\n")
	initialGoal := promptLines[0]
	messageRatings := make([]float32, len(options))

	for i, msg := range options {
		rating, err := agentState.VoteForAction(initialGoal, msg.Content)
		if err != nil {
			fmt.Printf("Error voting for action: %v\n", err)
			continue
		}

		messageRatings[i] = rating
		//fmt.Printf("Vote for message %d of %d finished with rating: %f\n", i, len(options), rating)
	}

	// now calculate the average rating
	averageRating := float32(0.0)
	maxRating := messageRatings[0]
	for _, rating := range messageRatings {
		averageRating += rating
		if rating > maxRating {
			maxRating = rating
		}
	}
	averageRating /= float32(len(messageRatings))

	filteredOptions := make([]*engines.Message, 0)
	for i, msg := range options {
		if messageRatings[i] >= min(8.0, maxRating) {
			filteredOptions = append(filteredOptions, msg)
		}
	}

	//fmt.Printf("Done voting, initial options size: %d, filtered options size: %d, average rating: %f\n",
	//	len(options), len(filteredOptions), averageRating)
	return filteredOptions
}
