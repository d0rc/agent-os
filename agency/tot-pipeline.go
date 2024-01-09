package agency

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"github.com/logrusorgru/aurora"
	"sync/atomic"
	"time"
)

func (agentState *GeneralAgentInfo) ToTPipeline() {
	var jobsSent = int(0)
	timer := time.NewTimer(5 * time.Second)
	for {
		jobsFinished := atomic.LoadUint64(&agentState.jobsFinished)
		jobsReceived := atomic.LoadUint64(&agentState.jobsReceived)

		if jobsFinished >= jobsReceived {
			jobsSent, _ = agentState.totPipelineStep()
		} else {
			jobsSent = 0
		}

		if len(agentState.historyUpdated) > 0 {
			<-agentState.historyUpdated
		}

		if jobsSent > 0 {
			continue
		}

		select {
		case <-agentState.historyUpdated:
			// we can try again - we have a new message here...!
		case <-timer.C:
			// five seconds have passed...!
			timer.Reset(5 * time.Second)
		}
	}
}

func (agentState *GeneralAgentInfo) totPipelineStep() (int, error) {
	ts := time.Now()
	systemMessage, err := agentState.getSystemMessage()
	if err != nil {
		return 0, fmt.Errorf("error getting system message: %v", err)
	}

	// traverse agentState.History and find all paths
	// which lead to terminal messages
	fmt.Printf("Starting to traverse agentState.History(%d) and find all paths\n", len(agentState.History))
	terminalMessages := 0
	jobsSubmitted := 0
	lengthStats := make(map[int]int)
	//finalMessageCommand := make(map[string]int)
	//finalMessageRating := make(map[int]int)
	traverseAndExecute(*systemMessage.ID, append(agentState.History, systemMessage), func(messages []*engines.Message) {
		terminalMessages++
		lengthStats[len(messages)]++
		//fmt.Printf("Got path of length %d\n", len(messages))
		if agentState.visitTerminalMessage(messages) {
			jobsSubmitted++
		}
	})

	if jobsSubmitted > 0 {
		agentState.jobsSubmittedTs = time.Now()
	}

	fmt.Printf("[%s] Done in %v, found %d terminal messages, jobs submitted: %d, length stats: %v\n",
		aurora.BrightBlue(agentState.Settings.Agent.Name),
		time.Since(ts), terminalMessages, jobsSubmitted, lengthStats)

	if jobsSubmitted == 0 {
		tsljb := time.Since(agentState.jobsSubmittedTs)
		if tsljb > ResubmitSystemPromptAfter &&
			atomic.LoadUint64(&agentState.jobsFinished) == atomic.LoadUint64(&agentState.jobsReceived) {
			agentState.Stop()
		}
	}

	return jobsSubmitted, nil
}

// CallbackFunctionType is the type for callback functions
type CallbackFunctionType func([]*engines.Message)

type traverseContext struct {
	MessageMap map[string]*engines.Message
	RepliesMap map[string][]*engines.Message
}

// populateMaps populates MessageMap and RepliesMap from the History
func createTraverseContext(history []*engines.Message) *traverseContext {
	var MessageMap = make(map[string]*engines.Message)
	var RepliesMap = make(map[string][]*engines.Message)
	for _, m := range history {
		if m.ID != nil {
			MessageMap[*m.ID] = m
		}
		m.RLock()
		if m.ReplyTo != nil {
			for k, _ := range m.ReplyTo {
				RepliesMap[k] = append(RepliesMap[k], m)
			}

		}
		m.RUnlock()
	}

	return &traverseContext{
		MessageMap: MessageMap,
		RepliesMap: RepliesMap,
	}
}

// findMessageByID finds a message by ID using the map
func (ctx *traverseContext) findMessageByID(id string) *engines.Message {
	return ctx.MessageMap[id]
}

// traverse recursively traverses the message tree
func (ctx *traverseContext) traverse(msg *engines.Message, path []*engines.Message, callback CallbackFunctionType) {
	path = append(path, msg)
	replies := ctx.RepliesMap[*msg.ID]

	if len(replies) == 0 || len(path) > ToTPathLenToTriggerTerminalCallback {
		// Terminal message reached, execute callback
		callback(path)
		return
	}

	if (msg.Role == engines.ChatRoleSystem || msg.Role == engines.ChatRoleUser) && (len(replies) < 4) {
		// we can still find something interesting on this step
		callback(path)
	}

	for _, reply := range replies {
		msg.RLock()
		if msg.ReplyTo != nil {
			_, exists := msg.ReplyTo[*reply.ID]
			msg.RUnlock()
			if exists {
				// the msg can be a reply to `reply`
				continue
			}
		} else {
			msg.RUnlock()
		}
		ctx.traverse(reply, append([]*engines.Message{}, path...), callback)
	}
}

// TraverseAndExecute starts traversal from a given message ID and executes callback for each path
func traverseAndExecute(startID string, history []*engines.Message, callback CallbackFunctionType) {
	//rand.Seed(time.Now().UnixNano()) // Initialize the random seed here if needed for Monte-Carlo methods

	ctx := createTraverseContext(history) // Populate the maps for efficient lookups

	startMsg := ctx.findMessageByID(startID)
	if startMsg == nil {
		return
	}
	ctx.traverse(startMsg, []*engines.Message{}, callback)
}
