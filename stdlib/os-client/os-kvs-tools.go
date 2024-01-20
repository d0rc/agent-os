package os_client

import (
	"fmt"
	"github.com/d0rc/agent-os/cmds"
	"github.com/d0rc/agent-os/engines"
	"time"
)

func (c *AgentOSClient) GetTaskCachedResult(namespace, key string) ([]byte, error) {
	documentId := engines.GenerateMessageId(key)
	cachedResult, err := c.RunRequest(&cmds.ClientRequest{
		GetCacheRecords: []cmds.GetCacheRecord{
			{
				Namespace: namespace,
				Key:       documentId,
			},
		},
	}, 60*time.Second, REP_IO)

	if err != nil {
		return nil, err
	}

	if len(cachedResult.GetCacheRecords) == 0 {
		return nil, nil
	}

	return cachedResult.GetCacheRecords[0].Value, nil
}

func (c *AgentOSClient) SetTaskCachedResult(namespace, key string, result []byte) error {
	documentId := engines.GenerateMessageId(key)
	response, err := c.RunRequest(&cmds.ClientRequest{
		SetCacheRecords: []cmds.SetCacheRecord{
			{
				Namespace: namespace,
				Key:       documentId,
				Value:     result,
			},
		},
	}, 60*time.Second, REP_IO)

	if err != nil {
		return err
	}

	done := response.SetCacheRecords[0].Done

	if done {
		return nil
	}

	return fmt.Errorf("error setting cached result")
}
