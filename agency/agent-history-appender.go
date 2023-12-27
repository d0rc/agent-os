package agency

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	os_client "github.com/d0rc/agent-os/os-client"
)

func (agentState *GeneralAgentInfo) historyAppender() {
	var message *engines.Message
	for {
		select {
		case <-agentState.quitHistoryAppender:
			return
		case message = <-agentState.historyAppenderChannel:
			// let's see if message already in the History
			messageId := message.ID
			alreadyExists := false
			for _, storedMessage := range agentState.History {
				if *storedMessage.ID == *messageId {
					fmt.Printf("got message with ID %s already in history, merging replyTo maps\n", *messageId)
					if storedMessage.Content != message.Content {
						storedMessage.Content = message.Content // fatal error!
					}
					alreadyExists = true
					for k, _ := range message.ReplyTo {
						storedMessage.ReplyTo[k] = message.ReplyTo[k]
					}
				}
			}
			if !alreadyExists {
				fmt.Printf("Adding new message to history: %s\n", *messageId)
				agentState.History = append(agentState.History, message)
				atomic.AddInt32(&agentState.historySize, 1)
			}
		case message = <-agentState.systemWriterChannel:
		}
		runClientRequest(agentState, message)
	}
}
func runClientRequest(agentState *GeneralAgentInfo, message *engines.Message) {
	_, _ = agentState.Server.RunRequest(&cmds.ClientRequest{
		WriteMessagesTrace: []*engines.Message{message},
	}, 120*time.Second, os_client.REP_IO)
}
