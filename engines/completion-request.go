package engines

import (
	"bytes"
	"encoding/json"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
)

func RunCompletionRequest(inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask) ([]*Message, error) {
	if len(batch) == 0 {
		return nil, nil
	}
	client := http.Client{
		Timeout: InferenceTimeout,
	}

	type commandList struct {
		Prompts     []string `json:"prompt"`
		N           int      `json:"n"`
		Max         int      `json:"max_tokens"`
		Stop        []string `json:"stop"`
		Temperature float32  `json:"temperature"`
		Model       string   `json:"model"`
		BestOf      int      `json:"best_of"`
	}

	type commandSingle struct {
		Prompts     string   `json:"prompt"`
		N           int      `json:"n"`
		Max         int      `json:"max_tokens"`
		Stop        []string `json:"stop"`
		Temperature float32  `json:"temperature"`
		Model       string   `json:"model"`
		BestOf      int      `json:"best_of"`
	}

	var stopTokens = []string{"###"}
	if batch[0].Req.StopTokens != nil {
		stopTokens = append(stopTokens, batch[0].Req.StopTokens...)
	}

	if batch[0].Req.BestOf == 0 {
		batch[0].Req.BestOf = 1
	}

	promptBodies := make([]string, len(batch))
	for i, b := range batch {
		promptBodies[i] = b.Req.RawPrompt
	}

	var commandBuffer []byte
	var err error
	if len(batch) > 1 {
		cmd := &commandList{
			Prompts:     promptBodies,
			N:           1,
			Max:         4096,
			Stop:        stopTokens,
			Temperature: batch[0].Req.Temperature,
			BestOf:      batch[0].Req.BestOf,
		}

		commandBuffer, err = json.Marshal(cmd)
		if err != nil {
			zlog.Fatal().Err(err).Msg("error marshaling command")
		}
	} else {
		cmd := &commandSingle{
			Prompts:     promptBodies[0],
			N:           1,
			Max:         4096,
			Stop:        stopTokens,
			Temperature: batch[0].Req.Temperature,
			BestOf:      batch[0].Req.BestOf,
		}

		commandBuffer, err = json.Marshal(cmd)
		if err != nil {
			zlog.Fatal().Err(err).Msg("error marshaling command")
		}
	}

	// sending the request here...!
	resp, err := client.Post(inferenceEngine.EndpointUrl,
		"application/json",
		bytes.NewBuffer(commandBuffer))

	// whatever happened here, it's not of our business, we should just log it
	if err != nil {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msg("error sending request")
		return nil, err
	}

	// read resp.Body to result
	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msg("error reading response")
		return nil, err
	}

	if resp.StatusCode != 200 {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msgf("error sending request http code is %d", resp.StatusCode)
		return nil, err
	}

	// now, let us parse all the response in choices
	type response struct {
		Choices []struct {
			Text string `json:"text"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	parsedResponse := &response{}
	err = json.Unmarshal(result, parsedResponse)
	if err != nil {
		zlog.Error().Err(err).
			Str("response", string(result)).
			Msg("error unmarshalling response")
		return nil, err
	}

	results := make([]*Message, len(batch))
	// ok now each choice goes to its caller
	for idx, job := range batch {
		results[idx] = &Message{
			Role:    ChatRoleAssistant,
			Content: parsedResponse.Choices[idx].Text,
		}
		if job.Res != nil {
			job.Res <- results[idx]
		}
	}

	return results, nil
}
