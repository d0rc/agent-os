package fetcher

import (
	"github.com/d0rc/agent-os/storage"
	"github.com/rs/zerolog"
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

func (f *Fetcher) FetchMessages(agentName string) (map[string]*DotNode, map[string]string, error) {
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

		ids2fetchChunks := splitIntoChunks(ids2fetch, 1024)
		tmpMessages := make([]DbMessage, 0)
		for chunkIdx, chunk := range ids2fetchChunks {
			f.lg.Info().Msgf("[%d/%d] fetching chunk of %d messages",
				chunkIdx+1,
				len(ids2fetchChunks),
				len(ids2fetch))
			chunkMessages := make([]DbMessage, 0)
			err = f.db.Db.GetStructsSlice("get-messages-by-ids", &tmpMessages, chunk)
			if err != nil {
				f.lg.Error().Err(err).Msg("error getting messages")
				return nil, nil, err
			}

			chunkMessages = append(chunkMessages, tmpMessages...)
		}

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

		f.lg.Info().Msgf("fetching links for %d", len(newMessages))
		newMessageIds := getMapKeys(newMessages)
		// for all new messages, fetch their replies
		newMessageIdsChunks := splitIntoChunks(newMessageIds, 1024)
		links := make([]DbMessageLink, 0)
		for chunkIdx, chunk := range newMessageIdsChunks {
			f.lg.Info().Msgf("[%d/%d] fetching chunk of %d links",
				chunkIdx+1,
				len(newMessageIdsChunks),
				len(newMessageIds))
			chunkLinks := make([]DbMessageLink, 0)
			err = f.db.Db.GetStructsSlice("get-messages-links-by-reply-to", &chunkLinks, chunk)
			if err != nil {
				f.lg.Error().Err(err).Msg("error getting messages")
				return nil, nil, err
			}

			links = append(links, chunkLinks...)
		}

		newIds := make(map[string]struct{})
		for _, link := range links {
			edges[link.Id] = link.ReplyTo
			if _, exists := messages[link.Id]; !exists {
				newIds[link.Id] = struct{}{}
			}
		}

		ids2fetch = getMapKeys(newIds)
		if len(ids2fetch) == 0 {
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
