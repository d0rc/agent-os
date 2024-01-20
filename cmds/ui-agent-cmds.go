package cmds

import (
	"fmt"
	"github.com/d0rc/agent-os/engines"
	"github.com/d0rc/agent-os/syslib/borrow-engine"
	"github.com/d0rc/agent-os/syslib/server"
	"strings"
)

type UIRequest struct {
	Token             string             `json:"token"`
	UIGetMessages     []UIGetMessage     `json:"ui-get-messages"`
	UIUploadDocuments []UIUploadDocument `json:"ui-upload-documents"`
	UITagDocuments    []UITagDocument    `json:"ui-tag-documents"`
	UIDeleteDocuments []UIDeleteDocument `json:"ui-delete-documents"`
}

type UIResponse struct {
	UIGetMessagesResponse     []UIGetMessageResponse     `json:"ui-get-messages-response"`
	UIUploadDocumentsResponse []UIUploadDocumentResponse `json:"ui-upload-documents-response"`
	UITagDocumentsResponse    []UITagDocumentResponse    `json:"ui-tag-documents-response"`
	UIDeleteDocumentsResponse []UIDeleteDocumentResponse `json:"ui-delete-documents-response"`
}

type UIGenSettings struct {
	Temperature   float32  `json:"temp"`
	BestOf        int      `json:"best-of"`
	TopK          int      `json:"top-k"`
	TopP          float32  `json:"top-p"`
	PreProcessor  string   `json:"pre-process"`
	PostProcessor string   `json:"post-process"`
	Model         string   `json:"model"`
	StopTokens    []string `json:"stop-tokens"`
}

type UIGetMessage struct {
	ChatID              string             `json:"agent-id"`
	GenerationSettings  *UIGenSettings     `json:"generation-settings"`
	Messages            []*engines.Message `json:"messages"`
	InlineButton        *string            `json:"inline-button"`
	DocumentCollections []string           `json:"rag-ids"`
	MaxRequiredResults  int                `json:"max-required-results"`
	NoCache             bool               `json:"no-cache"`
}

type UIGetMessageResponse struct {
	Choices        []engines.Message `json:"choices"`
	VisibleMessage []string          `json:"visible-message"`
	InlineButtons  [][]string        `json:"inline-buttons"`
	Error          string            `json:"error"`
}

type UIUploadDocument struct {
	FileName    string   `json:"file-name"`
	ContentType string   `json:"content-type"`
	FileBody    []byte   `json:"file-body"`
	Tags        []string `json:"tags"`
}

type UIUploadDocumentResponse struct {
	DocumentId string `json:"document-id"`
	Error      string `json:"error"`
}

type UITagDocument struct {
	DocumentId string   `json:"document-id"`
	Tags       []string `json:"tags"`
}

type UITagDocumentResponse struct {
	Error string `json:"error"`
}

type UIDeleteDocument struct {
	DocumentId string `json:"document-id"`
}

type UIDeleteDocumentResponse struct {
	Error string `json:"error"`
}

func ProcessUIRequest(uiReq *UIRequest, ctx *server.Context) (*ServerResponse, error) {
	result := &UIResponse{
		UIGetMessagesResponse:     make([]UIGetMessageResponse, 0),
		UIUploadDocumentsResponse: make([]UIUploadDocumentResponse, 0),
		UITagDocumentsResponse:    make([]UITagDocumentResponse, 0),
		UIDeleteDocumentsResponse: make([]UIDeleteDocumentResponse, 0),
	}

	if uiReq.UIGetMessages != nil && len(uiReq.UIGetMessages) > 0 {
		for _, uiGetMessage := range uiReq.UIGetMessages {
			result.UIGetMessagesResponse = append(result.UIGetMessagesResponse, processUIGetMessage(
				uiGetMessage,
				ctx))
		}
	}
	if uiReq.UIUploadDocuments != nil && len(uiReq.UIUploadDocuments) > 0 {
		for _, uiUploadDocument := range uiReq.UIUploadDocuments {
			result.UIUploadDocumentsResponse = append(result.UIUploadDocumentsResponse, ProcessUIUploadDocument(
				uiUploadDocument,
				ctx))
		}
	}
	if uiReq.UITagDocuments != nil && len(uiReq.UITagDocuments) > 0 {
		for _, uiTagDocument := range uiReq.UITagDocuments {
			result.UITagDocumentsResponse = append(result.UITagDocumentsResponse, ProcessUITagDocument(
				uiTagDocument,
				ctx))
		}
	}
	if uiReq.UIDeleteDocuments != nil && len(uiReq.UIDeleteDocuments) > 0 {
		for _, uiDeleteDocument := range uiReq.UIDeleteDocuments {
			result.UIDeleteDocumentsResponse = append(result.UIDeleteDocumentsResponse, ProcessUIDeleteDocument(
				uiDeleteDocument,
				ctx))
		}
	}

	return &ServerResponse{
		UIResponse: result,
	}, nil
}

// processUIGetMessage - process single completion request
func processUIGetMessage(uiGetMessage UIGetMessage, ctx *server.Context) UIGetMessageResponse {
	uiGetMessage.Messages = preprocessMessages(uiGetMessage.Messages)

	resp, err := processGetCompletion(
		GetCompletionRequest{
			Model:       uiGetMessage.GenerationSettings.Model,
			RawPrompt:   collectPrompt(uiGetMessage),
			Temperature: uiGetMessage.GenerationSettings.Temperature,
			StopTokens:  uiGetMessage.GenerationSettings.StopTokens,
			MinResults:  uiGetMessage.MaxRequiredResults,
			MaxResults:  0,
			BestOf:      uiGetMessage.GenerationSettings.BestOf,
		}, ctx, "ui", borrow_engine.PRIO_Kernel)
	if err != nil {
		return UIGetMessageResponse{
			Error: err.Error(),
		}
	}

	messages := make([]engines.Message, len(resp.Choices))
	visibleMessage := make([]string, len(resp.Choices))
	inlineButtons := make([][]string, len(resp.Choices))
	for i, choice := range resp.Choices {
		id := engines.GenerateMessageId(choice)
		messages[i] = engines.Message{
			ID:      &id,
			Role:    engines.ChatRoleAssistant,
			Content: choice,
			ReplyTo: map[string]struct{}{
				*uiGetMessage.Messages[len(uiGetMessage.Messages)-1].ID: {},
			},
		}
		visibleMessage[i] = choice
		inlineButtons[i] = make([]string, 0)
	}

	return UIGetMessageResponse{
		Choices:        messages,
		VisibleMessage: visibleMessage,
		InlineButtons:  inlineButtons,
		Error:          "",
	}
}

func collectPrompt(info UIGetMessage) string {
	return chatToRawPrompt(info.Messages)
}

func chatToRawPrompt(sample []*engines.Message) string {
	// following well known ### Instruction ### Assistant ### User format
	rawPrompt := strings.Builder{}
	for _, message := range sample {
		switch message.Role {
		case engines.ChatRoleSystem:
			rawPrompt.WriteString(fmt.Sprintf("### Instruction:\n%s\n", message.Content))
		case engines.ChatRoleAssistant:
			rawPrompt.WriteString(fmt.Sprintf("### Assistant:\n%s\n", message.Content))
		case engines.ChatRoleUser:
			rawPrompt.WriteString(fmt.Sprintf("### User:\n%s\n", message.Content))
		}
	}
	rawPrompt.WriteString("### Assistant:\n")

	return rawPrompt.String()
}

func preprocessMessages(data []*engines.Message) []*engines.Message {
	result := make([]*engines.Message, len(data))
	for i, message := range data {
		if message.ID == nil {
			id := engines.GenerateMessageId(message.Content)
			message.ID = &id
		}
		result[i] = message
	}

	return result
}

// ProcessUIUploadDocument - process single upload document request
func ProcessUIUploadDocument(uiUploadDocument UIUploadDocument, ctx *server.Context) UIUploadDocumentResponse {
	return UIUploadDocumentResponse{
		DocumentId: "",
		Error:      "not implemented",
	}
}

// ProcessUITagDocument - process single tag document request
func ProcessUITagDocument(uiTagDocument UITagDocument, ctx *server.Context) UITagDocumentResponse {
	return UITagDocumentResponse{
		Error: "not implemented",
	}
}

// ProcessUIDeleteDocument - process single delete document request
func ProcessUIDeleteDocument(uiDeleteDocument UIDeleteDocument, ctx *server.Context) UIDeleteDocumentResponse {
	return UIDeleteDocumentResponse{
		Error: "not implemented",
	}
}
