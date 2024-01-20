package fetcher

import (
	"github.com/d0rc/agent-os/stdlib/storage"
	"github.com/rs/zerolog"
	"sync/atomic"
	"time"
)

type Fetcher struct {
	db *storage.Storage
	lg zerolog.Logger
}

func NewFetcher(db *storage.Storage, lg zerolog.Logger) *Fetcher {
	return &Fetcher{
		db: db,
		lg: lg,
	}
}

func (f *Fetcher) FetchMessages(agentName string, dbChunkSize, dbThreads int) (map[string]*DotNode, map[string]string, error) {
	f.lg.Info().Msgf("loading root messages for agent %s", agentName)

	messages := make(map[string]*DotNode)
	edges := make(map[string]string)

	ids2fetch, err := f.getRootMessages(agentName)
	if err != nil {
		f.lg.Error().Err(err).Msg("error getting root messages")
		return nil, nil, err
	}

	for {
		// fetching information about all messages we need
		f.lg.Info().Msgf("fetching %d messages", len(ids2fetch))

		ids2fetchChunks := splitIntoChunks(ids2fetch, dbChunkSize)
		messageCounter := int32(0)
		tmpMessages := pMap(func(chunk []string, chunkIdx int) []DbMessage {
			ts := time.Now()
			chunkMessages := make([]DbMessage, 0)
			err = f.db.Db.GetStructsSlice("get-messages-by-ids", &chunkMessages, chunk)
			if err != nil {
				f.lg.Fatal().Err(err).Msg("error getting messages")
			}

			f.lg.Info().Msgf("Done fetching chunk#%d [%d of %d] (total links: %d) in %v",
				chunkIdx+1,
				atomic.AddInt32(&messageCounter, 1),
				len(ids2fetchChunks),
				len(ids2fetch),
				time.Since(ts))
			return chunkMessages
		}, ids2fetchChunks, dbThreads)

		// once we have all the messages, let's add them to the graph
		newMessages := make(map[string]struct{})
		for _, tmpMsg := range tmpMessages {
			if _, exists := messages[tmpMsg.Id]; !exists {
				// it's a new message
				newMessages[tmpMsg.Id] = struct{}{}
				messages[tmpMsg.Id] = &DotNode{
					Id:      tmpMsg.Id,
					Label:   getLabel(tmpMsg),
					Role:    tmpMsg.Role,
					Content: tmpMsg.Content,
				}
			}
		}

		// for all new messages, fetch their replies
		f.lg.Info().Msgf("fetching links for %d", len(newMessages))
		// parallel implementation
		newMessageIds := getMapKeys(newMessages)
		newMessageIdsChunks := splitIntoChunks(newMessageIds, dbChunkSize)
		linksCounter := int32(0)
		links := pMap(func(chunk []string, chunkIdx int) []DbMessageLink {
			ts := time.Now()
			chunkLinks := make([]DbMessageLink, 0)
			err = f.db.Db.GetStructsSlice("get-messages-links-by-reply-to", &chunkLinks, chunk)
			if err != nil {
				f.lg.Fatal().Err(err).Msg("error getting messages")
			}

			f.lg.Info().Msgf("Done fetching chunk#%d [%d of %d] (total links: %d) in %v",
				chunkIdx+1,
				atomic.AddInt32(&linksCounter, 1),
				len(newMessageIdsChunks),
				len(newMessageIds),
				time.Since(ts))

			return chunkLinks
		}, newMessageIdsChunks, dbThreads)

		newIds := make(map[string]struct{})
		for _, link := range links {
			edges[link.Id] = link.ReplyTo
			if _, exists := messages[link.Id]; !exists {
				newIds[link.Id] = struct{}{}
			}
		}

		ids2fetch = getMapKeys(newIds)
		if len(ids2fetch) == 0 {
			// we have no more messages to fetch
			break
		}
	}

	return messages, edges, nil
}

func splitIntoChunks(fetch []string, chunkSize int) [][]string {
	chunks := make([][]string, 0)
	for i := 0; i < len(fetch); i += chunkSize {
		end := i + chunkSize
		if end > len(fetch) {
			end = len(fetch)
		}
		chunks = append(chunks, fetch[i:end])
	}

	return chunks
}

func (f *Fetcher) getRootMessages(agentName string) ([]string, error) {
	type idListRow struct {
		Id string `db:"id"`
	}
	rootMessages := make([]idListRow, 0)
	err := f.db.Db.GetStructsSlice("get-agent-roots", &rootMessages, agentName)
	if err != nil {
		f.lg.Fatal().Err(err).Msg("error getting agent roots")
	}

	f.lg.Info().Msgf("got %d root messages for %s",
		len(rootMessages), agentName)

	result := make([]string, len(rootMessages))
	for i, row := range rootMessages {
		result[i] = row.Id
	}

	return result, err
}
