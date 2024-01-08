package engines

import (
	"bytes"
	"encoding/json"
	"fmt"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
	"time"
)

func togetherAIInference(inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask, client *http.Client) ([]*Message, error) {
	// {
	//  "model": "togethercomputer/RedPajama-INCITE-7B-Instruct",
	//  "prompt": "Q: The capital of France is?\nA:",
	//  "temperature": 0.7,
	//  "top_p": 0.7,
	//  "top_k": 50,
	//  "max_tokens": 1,
	//  "repetition_penalty": 1
	//}
	type togetherRequest struct {
		Model             string  `json:"model"`
		Prompt            string  `json:"prompt"`
		Temperature       float32 `json:"temperature"`
		TopP              float32 `json:"top_p"`
		TopK              int     `json:"top_k"`
		MaxTokens         int     `json:"max_tokens"`
		RepetitionPenalty float32 `json:"repetition_penalty"`
		Stop              string  `json:"stop"`
	}

	var stopTokens = []string{"###"}
	if len(batch[0].Req.StopTokens) > 0 {
		stopTokens[0] = batch[0].Req.StopTokens[0]
	}
	req := &togetherRequest{
		// Model:       "mistralai/Mistral-7B-Instruct-v0.1",
		Model:       "mistralai/Mixtral-8x7B-Instruct-v0.1",
		Prompt:      batch[0].Req.RawPrompt,
		Temperature: batch[0].Req.Temperature,
		TopP:        0.9,
		TopK:        50,
		MaxTokens:   2048,
		Stop:        stopTokens[0],
	}

	reqJson, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": fmt.Sprintf("Bearer %s", inferenceEngine.Token),
	}

	// send request with the headers
	httpReq, err := http.NewRequest("POST", inferenceEngine.EndpointUrl, bytes.NewBuffer(reqJson))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		httpReq.Header.Set(k, v)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	result, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("http code is %d, err: %v", resp.StatusCode, string(result))
		zlog.Error().Err(err).
			Msgf("err in compl. (%s): %v", inferenceEngine.EndpointUrl, err)
		return nil, err
	}

	// now, let us parse all the response in choices
	type togetherResponse struct {
		Output struct {
			Choices []struct {
				Text string `json:"text"`
			}
		} `json:"output"`
	}
	parsedResponse := &togetherResponse{}

	err = json.Unmarshal(result, parsedResponse)
	if err != nil {
		return nil, err
	}

	results := make([]*Message, 1)
	results[0] = &Message{
		Role:    ChatRoleAssistant,
		Content: parsedResponse.Output.Choices[0].Text,
	}

	if batch[0].Res != nil {
		batch[0].Res <- results[0]
	}

	return results, nil
}

type pointlessOpenAIList struct {
	Object string `json:"object"`
	Data   []struct {
		ModelId string `json:"id"`
	} `json:"data"`
}

func fetchInferenceEngineModels(engine *RemoteInferenceEngine) error {
	client := http.Client{Timeout: 120 * time.Second}

	if engine.ModelsEndpoint == "" {
		engine.ModelsEndpoint = guessModelsEndpoint(engine)
	}

	req, err := http.NewRequest("GET", engine.ModelsEndpoint, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", engine.Token))
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}

	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response: %v", err)
	}

	models := &pointlessOpenAIList{}

	err = json.Unmarshal(result, models)
	if err != nil {
		return fmt.Errorf("error unmarshalling response: %v", err)
	}

	if len(models.Data) > 0 {
		engine.Models = append(engine.Models, models.Data[0].ModelId)
	}

	return nil
}
