package agency

import (
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/stdlib/message-store"
	"github.com/d0rc/agent-os/stdlib/os-client"
	"time"
)

func (agentState *GeneralAgentInfo) historyAppender() {
	var message *engines.Message
	for {
		select {
		case <-agentState.quitHistoryAppender:
			return
		case message = <-agentState.historyAppenderChannel:
			trajectoryId := message_store.TrajectoryID(keys(message.ReplyTo)[0])
			_ = agentState.space.AddMessage(&trajectoryId, message)
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
	if ShouldWriteMessageTrace {
		_, _ = agentState.Server.RunRequest(&cmds.ClientRequest{
			ProcessName:        agentState.SystemName,
			WriteMessagesTrace: []*engines.Message{message},
		}, 120*time.Second, os_client.REP_IO)
	}
}
