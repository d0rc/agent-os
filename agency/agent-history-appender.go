package agency

import (
	"github.com/d0rc/agent-os/engines"
	message_store "github.com/d0rc/agent-os/message-store"
	"sync/atomic"
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
			trajectoryId := message_store.TrajectoryID(keys(message.ReplyTo)[0])

			_ = agentState.space.AddMessage(&trajectoryId, message)

			alreadyExists := false
			for _, storedMessage := range agentState.History {
				if *storedMessage.ID == *messageId {
					//fmt.Printf("got message with ID %s already in history, merging replyTo maps\n", *messageId)
					if storedMessage.Content != message.Content {
						storedMessage.Content = message.Content // fatal error!
					}
					alreadyExists = true
					storedMessage.Lock()
					for k, _ := range message.ReplyTo {
						storedMessage.ReplyTo[k] = message.ReplyTo[k]
					}
					storedMessage.Unlock()
				}
			}
			if !alreadyExists {
				//fmt.Printf("Adding new message to history: %s\n", *messageId)
				agentState.History = append(agentState.History, message)
				//agentState.historyUpdated <- struct{}{}
				atomic.AddInt32(&agentState.historySize, 1)
			}
		case message = <-agentState.systemWriterChannel:
		}
		writeMessagesTrace(agentState, message)
	}
}

func keys(to map[string]struct{}) []string {
	result := make([]string, 0, len(to))
	for k, _ := range to {
		result = append(result, k)
	}

	return result
}
func writeMessagesTrace(agentState *GeneralAgentInfo, message *engines.Message) {
	/*_, _ = agentState.Server.RunRequest(&cmds.ClientRequest{
		ProcessName:        agentState.SystemName,
		WriteMessagesTrace: []*engines.Message{message},
	}, 120*time.Second, os_client.REP_IO)*/
}
