package cmds

import (
	"github.com/d0rc/agent-os/syslib/server"
)

func ProcessGetCacheRecords(requests []GetCacheRecord, ctx *server.Context, process string) (response *ServerResponse, err error) {
	var results = make([]*GetCacheRecordResponse, 0, len(requests))
	for _, request := range requests {
		result, err := ctx.Storage.GetTaskCachedResult(request.Namespace, request.Key)
		if err != nil {
			ctx.Log.Error().Err(err).
				Msgf("error getting cached result for %s/%s", request.Namespace, request.Key)
			results = append(results, &GetCacheRecordResponse{
				Key:       request.Key,
				Namespace: request.Namespace,
				Value:     nil,
			})
			continue
		}

		results = append(results, &GetCacheRecordResponse{
			Key:       request.Key,
			Namespace: request.Namespace,
			Value:     result,
		})
	}

	return &ServerResponse{
		GetCacheRecords: results,
	}, nil
}
