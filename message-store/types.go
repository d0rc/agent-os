package message_store

import "github.com/d0rc/agent-os/engines"

type MessageID uint64
type MessageType uint8
type ChainId []MessageID

const (
	MTSystem MessageType = iota
	MTUser
	MTAssistant
)

type AgentProcessID string

type ProcessingRequestID string

type VoteValue float32

type StoredMessage struct {
	ID                    MessageID
	AgentID               AgentProcessID
	Type                  MessageType
	Content               string
	RepliesTo             map[MessageID]struct{}
	RepliedBy             map[MessageID]struct{}
	RatedBy               map[MessageID]struct{}
	RequestsPending       []ProcessingRequestID
	ObservationsProcessed bool
}

type SerializedMessage struct {
	ID                    MessageID
	AgentID               AgentProcessID
	Type                  MessageType
	Content               string
	RepliesTo             []MessageID
	RepliedBy             []MessageID
	RatedBy               []MessageID
	RequestsPending       []ProcessingRequestID
	RequestsAttempted     int
	ObservationsProcessed bool
}

type SemanticSpace struct {
}

func (ss *SemanticSpace) AddMessage(agent AgentProcessID, message *engines.Message) error {
	//
	// insert into messages (agentId, messageId, messageType, content) values (?, ?, ?, ?);
	// insert into message_replies_to(agentId, messageId, replyToMessageId) values (?,?,?), (?, ?, ?), ....;
	// insert into message_replied_by(agentId, messageId, replyByMessageId) values (?,?,?), (?,?,?),....;
	// insert into message_rated_by(agentId, messageId, ratedByMessageId) values (?,?,?), (?,?,?),....;

	return nil
}

func (ss *SemanticSpace) GetMessage(agent AgentProcessID, id MessageID) (*SerializedMessage, error) {

}

func (ss *SemanticSpace) GetUnobservedMessages(agent AgentProcessID) ([]*SerializedMessage, error) {

}

func (ss *SemanticSpace) GetMessagesToVote(agent AgentProcessID, minVotes int) ([]*SerializedMessage, error) {

}

func (ss *SemanticSpace) GetMessagesToVisit(agent AgentProcessID, minReplies int, minAttempts int, minVote VoteValue) ([]*SerializedMessage, error) {

}

// in order to be able to continue the inference process
// we need to know for each message which hash no requests pending
// and which as processed fully
//
// assistant messages: (with command to execute)
// execute command, obtain observations, need an agent context here...!
//
// user messages:
// second, we will need a way to fetch all messages that have no requests pending
// and which have small enough `replied by`

type JobType string

const (
	JT_EmbeddingsLLMEmbedder768 JobType = "llm-embedder-768"
)

type TasksRequest struct {
	WorkerID       string    `json:"worker-id"`
	CompatibleJobs []JobType `json:"compatible-jobs"`
	MaxTasks       int       `json:"max-tasks"`
}

type TaskData struct {
	TaskId  string `json:"task-id"`
	ReqType string `json:"req-type"` // [query / key] == [request / document]
	Task    string `json:"task"`     // qa, icl, chat, lrlm, tool, convsearch
	Text    string `json:"text"`
}

type TasksResponse struct {
	Tasks []*TaskData `json:"tasks"`
}

type TaskResult struct {
	TaskId string    `json:"task-id"`
	Vector []float32 `json:"vector"`
}

type TasksResultsResponse struct {
	Results []*TaskResult `json:"results"`
}
