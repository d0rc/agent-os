package engines

import (
	"bytes"
	"encoding/json"
	"fmt"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
	"strings"
)

func guessModelsEndpoint(engine *RemoteInferenceEngine) string {
	if strings.Contains(engine.EndpointUrl, "/v1/") {
		// strings is http://..../.../v1/something
		// we need to remove something and replace it with models:
		// like this: http://..../.../v1/models/

		// first let's trim the string on /v1/, removing something
		tokens := strings.Split(engine.EndpointUrl, "/v1/")
		return tokens[0] + "/v1/models"
	}

	return engine.EndpointUrl
}

func openAICompatibleInference(inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask, client *http.Client) ([]*Message, error) {
	if len(inferenceEngine.Models) == 0 {
		err := fetchInferenceEngineModels(inferenceEngine)
		if err != nil {
			inferenceEngine.Models = []string{""}
		}
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

	var stopTokens = []string{"<|im_end|>", "<|im_start|>"}
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
			Model:       inferenceEngine.Models[0],
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
			Model:       inferenceEngine.Models[0],
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
		zlog.Error().
			Msgf("error in request: %v, %s", err, inferenceEngine.EndpointUrl)
		return nil, err
	}

	// read resp.Body to result
	result, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		zlog.Error().Err(err).
			Interface("batch", batch).
			Msg("error reading response")
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("http code is %d, err: %v", resp.StatusCode, string(result))
		zlog.Error().Err(err).
			Msgf("err in compl. (%s): %v", inferenceEngine.EndpointUrl, err)
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
