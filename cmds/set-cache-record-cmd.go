package cmds

import "github.com/d0rc/agent-os/server"

func ProcessSetCacheRecords(requests []SetCacheRecord, ctx *server.Context, process string) (response *ServerResponse, err error) {
	var results = make([]*SetCacheRecordResponse, 0, len(requests))
	for _, request := range requests {
		_, err := ctx.Storage.GetTaskCachedResult(request.Namespace, request.Key)
		if err != nil {
			ctx.Log.Error().Err(err).
				Msgf("error getting cached result for %s/%s", request.Namespace, request.Key)
			results = append(results, &SetCacheRecordResponse{
				Done: false,
			})
			continue
		}

		results = append(results, &SetCacheRecordResponse{
			Done: true,
		})
	}

	return &ServerResponse{
		SetCacheRecords: results,
	}, nil
}
