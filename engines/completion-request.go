package engines

import (
	"fmt"
	"github.com/rs/zerolog"
	"net/http"
)

func RunCompletionRequest(lg zerolog.Logger, inferenceEngine *RemoteInferenceEngine, batch []*JobQueueTask) ([]*Message, error) {
	if len(batch) == 0 {
		return nil, nil
	}
	client := http.Client{
		Timeout: InferenceTimeout,
	}

	if inferenceEngine.Protocol == "http-openai" {
		return openAICompatibleInference(lg, inferenceEngine, batch, &client)
	}

	if inferenceEngine.Protocol == "http-together" {
		return togetherAIInference(inferenceEngine, batch, &client)
	}

	return nil, fmt.Errorf("unsupported protocol %s", inferenceEngine.Protocol)
}
