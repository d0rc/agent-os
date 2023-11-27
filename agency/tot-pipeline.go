package agency

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"time"
)

func (agentState *GeneralAgentInfo) ToTPipeline() {
	for {
		agentState.totPipelineStep()
	}
}

func (agentState *GeneralAgentInfo) totPipelineStep() error {
	ts := time.Now()
	systemMessage, err := agentState.getSystemMessage()
	if err != nil {
		return fmt.Errorf("error getting system message: %v", err)
	}

	// traverse agentState.History and find all paths
	// which lead to terminal messages
	fmt.Printf("Starting to traverse agentState.History(%d) and find all paths\n", len(agentState.History))
	terminalMessages := 0
	traverseAndExecute(*systemMessage.ID, append(agentState.History, systemMessage), func(messages []*engines.Message) {
		if len(messages) > 7 {
			return
		}
		terminalMessages++
		//fmt.Printf("Got path of length %d\n", len(messages))
		agentState.visitTerminalMessage(messages)
	})

	fmt.Printf("Done in %v, found %d terminal messages\n", time.Since(ts), terminalMessages)
	return nil
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
		if m.ReplyTo != nil {
			RepliesMap[*m.ReplyTo] = append(RepliesMap[*m.ReplyTo], m)
		}
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

	if len(replies) == 0 || len(path) > 7 {
		// Terminal message reached, execute callback
		callback(path)
		return
	}

	for _, reply := range replies {
		if msg.ReplyTo == nil || *msg.ReplyTo != *reply.ID { // an obvious optimization to avoid cycles
			ctx.traverse(reply, append([]*engines.Message{}, path...), callback) // pass a copy of the path
		}
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
