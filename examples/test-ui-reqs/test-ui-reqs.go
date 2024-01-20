package main

import (
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
)

func main() {
	req := &cmds.ClientRequest{UIRequest: &cmds.UIRequest{
		Token: "",
		UIGetMessages: []cmds.UIGetMessage{
			{
				ChatID: "",
				GenerationSettings: &cmds.UIGenSettings{
					Temperature:   0,
					BestOf:        0,
					TopK:          0,
					TopP:          0,
					PreProcessor:  "",
					PostProcessor: "",
					Model:         "",
					StopTokens:    nil,
				},
				Messages: []*engines.Message{
					{
						Role:    engines.ChatRoleSystem,
						Content: "You are helpful assistant. You're role to assist user the best way you can.",
					},
					{
						Role:    engines.ChatRoleUser,
						Content: "Hello..! What is the capital of France?",
					},
				},
				InlineButton:        nil,
				DocumentCollections: []string{},
				MaxRequiredResults:  1,
				NoCache:             false,
			},
		},
		UIUploadDocuments: nil,
		UITagDocuments:    nil,
		UIDeleteDocuments: nil,
	}}

	jsonBytes, _ := json.Marshal(req)

	fmt.Printf("Got JSON:\n%s\n", string(jsonBytes))
}
