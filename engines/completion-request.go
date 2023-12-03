package engines

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	if inferenceEngine.Protocol == "http-openai" {
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
				Max:         512,
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
			err = fmt.Errorf("error sending request http code is %d", resp.StatusCode)
			zlog.Error().Err(err).
				Msgf("completion: http code is %d, url: %s, err: %v", resp.StatusCode, inferenceEngine.EndpointUrl, err)
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

	if inferenceEngine.Protocol == "http-together" {
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
			Model:       "mistralai/Mistral-7B-Instruct-v0.1",
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
		client := http.Client{Timeout: InferenceTimeout}
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

		defer resp.Body.Close()
		result, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("error sending request http code is %d", resp.StatusCode)
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

	return nil, fmt.Errorf("unsupported protocol %s", inferenceEngine.Protocol)
}
