package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	"os"
	"strings"
	"sync"
	"time"
)

var agentName = flag.String("name", "agent-research-agent", "Agent name")
var dbHost = flag.String("db-host", "127.0.0.1", "database host")

func main() {
	f := false
	lg, _ := utils.ConsoleInit("", &f)

	lg.Info().Msgf("starting up...")
	db, err := storage.NewStorage(lg, *dbHost)
	if err != nil {
		lg.Fatal().Err(err).Msg("error initializing storage")
	}

	type idListRow struct {
		Id string `db:"id"`
	}
	rootMessages := make([]idListRow, 0)
	err = db.Db.GetStructsSlice("get-agent-roots", &rootMessages, *agentName)
	if err != nil {
		lg.Fatal().Err(err).Msg("error getting agent roots")
	}

	lg.Info().Msgf("got %d root messages for %s",
		len(rootMessages), *agentName)

	messages := make(map[string]*dotNode)
	edges := make(map[string]string)
	for _, rootMessage := range rootMessages {
		messages[rootMessage.Id] = getMessage(db, rootMessage.Id)
	}

	ts := time.Now()
	checked := make(map[string]struct{})
	linksChan := make(chan *DbMessageLink, 1024*1024*1024)
	for {
		// get all messages which are replies to this one...!
		messagesToAdd := make([]*dotNode, 0)
		wg := sync.WaitGroup{}
		wg.Add(1)
		go func() {
			for id, _ := range messages {
				if _, ok := checked[id]; ok {
					continue
				}
				checked[id] = struct{}{}

				wg.Add(1)
				go func(id string) {
					links := make([]DbMessageLink, 0)
					err = db.Db.GetStructsSlice("get-message-links-by-reply-to", &links, id)
					if err != nil {
						lg.Fatal().Err(err).Msg("error getting message links")
					}

					for _, link := range links {
						linkCopy := link
						linksChan <- &linkCopy

					}

					wg.Done()
				}(id)
			}
			wg.Done()
		}()

		//wg.Add(1)
		go func() {
			addedMessages := make(map[string]struct{})
			for link := range linksChan {
				if time.Since(ts) > 5*time.Second {
					lg.Info().Msgf("total messages: %d, total edges: %d", len(messages), len(edges))
					ts = time.Now()
				}
				if link == nil {
					break
				}
				edges[link.Id] = link.ReplyTo
				if _, exists := messages[link.Id]; !exists {
					if _, exists := addedMessages[link.Id]; !exists {
						messagesToAdd = append(messagesToAdd, getMessage(db, link.Id))
						addedMessages[link.Id] = struct{}{}
					}
				}
			}

			//wg.Done()
		}()

		wg.Wait()
	retry:
		if len(linksChan) == 0 {
			linksChan <- nil
		} else {
			time.Sleep(1 * time.Second)
			goto retry
		}

		for _, msg := range messagesToAdd {
			messages[msg.Id] = msg
		}

		lg.Info().Msgf("added %d new messages", len(messagesToAdd))

		if len(messagesToAdd) == 0 {
			break
		}
	}

	lg.Info().Msgf("got %d messages", len(messages))

	messagesJson, err := json.Marshal(messages)
	if err != nil {
		lg.Fatal().Err(err).Msg("error marshaling messages")
	}
	err = os.WriteFile("/tmp/messages.json", messagesJson, 0644)
	if err != nil {
		lg.Fatal().Err(err).Msg("error writing messages.json")
	}

	edgesJson, err := json.Marshal(edges)
	if err != nil {
		lg.Fatal().Err(err).Msg("error marshaling edges")
	}
	err = os.WriteFile("/tmp/edges.json", edgesJson, 0644)
	if err != nil {
		lg.Fatal().Err(err).Msg("error writing edges.json")
	}

	// let's write it all into .dot file...
	dotFile := `digraph ToTGraph {
%s
%s
}
`
	nodesSlice := make([]string, 0, len(messages))
	for _, msg := range messages {
		nodesSlice = append(nodesSlice,
			fmt.Sprintf("\t%s [label=\"%s\"];\n", uuidToLabel(msg.Id), msg.Label))
	}
	nodesString := strings.Join(nodesSlice, "")

	edgesSlice := make([]string, 0, len(edges))
	for from, to := range edges {
		edgesSlice = append(edgesSlice,
			fmt.Sprintf("\t%s -> %s;\n", uuidToLabel(from), uuidToLabel(to)))
	}
	edgesString := strings.Join(edgesSlice, "")

	dotFile = fmt.Sprintf(dotFile, nodesString, edgesString)

	err = os.WriteFile("/tmp/graph.dot", []byte(dotFile), 0644)
	if err != nil {
		lg.Fatal().Err(err).Msg("error writing graph.dot")
	}
}

func uuidToLabel(from string) string {
	return "u" + strings.ReplaceAll(from, "-", "")
}

func getMessage(db *storage.Storage, id string) *dotNode {
	label, role, content := getMessageInfo(db, id)
	return &dotNode{
		Id:      id,
		Label:   label,
		Content: content,
		Role:    role,
	}
}

var cache = map[string][]string{}
var cacheLock = sync.RWMutex{}

func getMessageInfo(db *storage.Storage, id string) (string, string, string) {
	cacheLock.RLock()
	if resp, exists := cache[id]; exists {
		cacheLock.RUnlock()
		return resp[0], resp[1], resp[2]
	}
	cacheLock.RUnlock()

	messages := make([]DbMessage, 0)
	err := db.Db.GetStructsSlice("get-messages-by-ids", &messages, []string{id})
	if err != nil {
		return id, "", ""
	}

	if len(messages) == 0 {
		return id, "", ""
	}

	message := messages[0]
	content := message.Content
	runesToRemove := []string{"\n", "\t", "{", "}", "(", ")", "\"", "'", "  "}
	for _, runeToRemove := range runesToRemove {
		content = strings.ReplaceAll(content, runeToRemove, " ")
	}

	finalString := fmt.Sprint(tools.CutStringAt(content, 25))
	finalResult := []string{finalString, message.Role, message.Content}
	cacheLock.Lock()
	cache[id] = finalResult
	cacheLock.Unlock()

	return finalResult[0], finalResult[1], finalResult[2]
}

type DbMessage struct {
	Id      string  `db:"id"`
	Name    *string `db:"name"`
	Role    string  `db:"role"`
	Content string  `db:"content"`
}

type DbMessageLink struct {
	Id      string `db:"id"`
	ReplyTo string `db:"reply_to"`
}

type dotNode struct {
	Id      string
	Label   string
	Role    string
	Content string
}
