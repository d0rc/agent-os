package engines

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog"
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

func openAICompatibleInference(lg zerolog.Logger, inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask, client *http.Client) ([]*Message, error) {
	if len(inferenceEngine.Models) == 0 {
		err := fetchInferenceEngineModels(inferenceEngine)
		if err != nil {
			inferenceEngine.Models = []string{""}
		}
		if len(inferenceEngine.Models) > 0 {
			inferenceEngine.Models[0] = "mistral-large:latest"
			lg.Info().Msgf("[%s] using model %s ", inferenceEngine.EndpointUrl, inferenceEngine.Models[0])
		}
	}

	var stopTokens = []string{"<|im_end|>", "<|im_start|>"}
	if batch[0].Req.StopTokens != nil {
		stopTokens = append(stopTokens, batch[0].Req.StopTokens...)
	}

	if batch[0].Req.BestOf == 0 {
		batch[0].Req.BestOf = 1
	}

	if len(batch) == 0 {
		return nil, nil
	}

	if len(batch[0].Req.Messages) == 0 {
		return doFullContextCompletion(lg, inferenceEngine, batch, client, stopTokens)
	}

	// need to build chat completion
	request := &ChatCompletionRequest{
		Model:       inferenceEngine.Models[0],
		Messages:    makeChatCompletionMessages(batch[0].Req.Messages),
		MaxTokens:   16384,
		Temperature: batch[0].Req.Temperature,
		N:           1,
		Stream:      false,
		Stop:        batch[0].Req.StopTokens,
	}

	commandBuffer, err := json.Marshal(request)
	if err != nil {
		lg.Fatal().Err(err).Msg("error marshaling command")
	}

	// sending the request here...!
	resp, err := client.Post(fmt.Sprintf("%s/chat/completions", strings.TrimSuffix(inferenceEngine.EndpointUrl, "/")),
		"application/json",
		bytes.NewBuffer(commandBuffer))

	if err != nil {
		lg.Error().
			Msgf("error in request: %v, %s", err, inferenceEngine.EndpointUrl)
		return nil, err
	}

	// read resp.Body to result
	result, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		lg.Error().Err(err).
			//Interface("batch", batch).
			Msgf("error reading response: %v", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("http code is %d, err: %v", resp.StatusCode, string(result))
		lg.Error().Err(err).
			Msgf("err in compl. (%s): %v", inferenceEngine.EndpointUrl, err)
		return nil, err
	}

	parsedResponse := &ChatCompletionResponse{}
	err = json.Unmarshal(result, parsedResponse)
	if err != nil {
		lg.Error().Err(err).
			Msgf("error unmarshalling response: %v", string(result))
		return nil, err
	}

	results := make([]*Message, 1)

	results[0] = &Message{
		Role:    ChatRole(parsedResponse.Choices[0].Message.Role),
		Content: parsedResponse.Choices[0].Message.Content,
	}
	if batch[0].Res != nil {
		batch[0].Res <- results[0]
	}
	return results, nil
}

func makeChatCompletionMessages(messages []Message) []ChatCompletionMessage {
	result := make([]ChatCompletionMessage, 0, len(messages))

	for _, msg := range messages {
		result = append(result, ChatCompletionMessage{
			Role:    string(msg.Role),
			Content: string(msg.Content),
		})
	}

	return result
}

func doFullContextCompletion(lg zerolog.Logger, inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask, client *http.Client, stopTokens []string) ([]*Message, error) {
	// completions

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
			Max:         16384,
			Stop:        stopTokens,
			Temperature: batch[0].Req.Temperature,
			BestOf:      batch[0].Req.BestOf,
			Model:       inferenceEngine.Models[0],
		}

		commandBuffer, err = json.Marshal(cmd)
		if err != nil {
			lg.Fatal().Err(err).Msg("error marshaling command")
		}
	} else {
		cmd := &commandSingle{
			Prompts:     promptBodies[0],
			N:           1,
			Max:         16384,
			Stop:        stopTokens,
			Temperature: batch[0].Req.Temperature,
			BestOf:      batch[0].Req.BestOf,
			Model:       inferenceEngine.Models[0],
		}

		commandBuffer, err = json.Marshal(cmd)
		if err != nil {
			lg.Fatal().Err(err).Msg("error marshaling command")
		}
	}

	// sending the request here...!
	resp, err := client.Post(fmt.Sprintf("%s/completions", strings.TrimSuffix(inferenceEngine.EndpointUrl, "/")),
		"application/json",
		bytes.NewBuffer(commandBuffer))

	// whatever happened here, it's not of our business, we should just log it
	if err != nil {
		lg.Error().
			Msgf("error in request: %v, %s", err, inferenceEngine.EndpointUrl)
		return nil, err
	}

	// read resp.Body to result
	result, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		lg.Error().Err(err).
			//Interface("batch", batch).
			Msgf("error reading response: %v", err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("http code is %d, err: %v", resp.StatusCode, string(result))
		lg.Error().Err(err).
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
		lg.Error().Err(err).
			Msgf("error unmarshalling response: %v", string(result))
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
