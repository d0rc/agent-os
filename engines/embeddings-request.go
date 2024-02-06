package engines

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/d0rc/agent-os/vectors"
	zlog "github.com/rs/zerolog/log"
	"io"
	"net/http"
)

func RunEmbeddingsRequest(inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask) ([]*vectors.Vector, error) {
	if len(batch) == 0 {
		return nil, fmt.Errorf("empty batch for inference engine %v", inferenceEngine)
	}
	if inferenceEngine.EmbeddingsEndpointUrl == "" {
		return nil, fmt.Errorf("embeddings endpoint is not configured for inference engine %v", inferenceEngine)
	}
	client := http.Client{
		Timeout: InferenceTimeout,
	}

	type command struct {
		Input []string `json:"texts"`
	}

	promptBodies := make([]string, len(batch))
	for i, b := range batch {
		promptBodies[i] = b.Req.RawPrompt
	}

	// '{"input":["hello", "hello", "hello", "hello"]}'
	cmd := &command{
		Input: promptBodies,
	}

	commandBuffer, err := json.Marshal(cmd)
	if err != nil {
		zlog.Fatal().Err(err).Msg("error marshaling command")
	}

	// sending the request here...!
	resp, err := client.Post(inferenceEngine.EmbeddingsEndpointUrl,
		"application/json",
		bytes.NewBuffer(commandBuffer))

	// whatever happened here, it's not of our business, we should just log it
	if err != nil {
		//zlog.Error().Err(err).
		//	Interface("batch", batch).
		//	Msg("error sending request")
		return nil, err
	}

	// read resp.Body to result
	defer resp.Body.Close()
	result, err := io.ReadAll(resp.Body)
	if err != nil {
		zlog.Error().Err(err).
			Msgf("embeddings: http code is %d, endpoint: %s, err: %v", resp.StatusCode, inferenceEngine.EmbeddingsEndpointUrl, err)
		return nil, err
	}

	if resp.StatusCode != 200 {
		err = fmt.Errorf("error sending request http code is %d", resp.StatusCode)
		zlog.Error().Err(err).
			Msgf("embeddings: http code is %d, endpoint: %s, err: %s", resp.StatusCode, inferenceEngine.EmbeddingsEndpointUrl, string(result))
		return nil, err
	}

	type embeddingsResponse struct {
		Vectors [][]float64 `json:"vectors"`
	}

	// now, let us parse all the response
	parsedResponse := &embeddingsResponse{}
	err = json.Unmarshal(result, parsedResponse)
	if err != nil || parsedResponse.Vectors == nil {
		zlog.Error().Err(err).
			Str("response", string(result)).
			Msg("error unmarshalling response")

		return nil, err
	}

	results := make([]*vectors.Vector, len(batch))
	// ok now each choice goes to its caller
	parsedModelName := "jinaai/jina-embeddings-v2-base-en"
	for idx, job := range batch {
		results[idx] = &vectors.Vector{
			VecF64: parsedResponse.Vectors[idx],
			Model:  &parsedModelName,
		}
		if job.ResEmbeddings != nil {
			job.ResEmbeddings <- results[idx]
		}
	}

	// zlog.Debug().Msgf("Returning %d results", len(results))

	return results, nil
}
