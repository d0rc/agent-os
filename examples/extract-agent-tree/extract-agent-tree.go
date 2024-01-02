package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/tools"
	"github.com/d0rc/agent-os/utils"
	zlog "github.com/rs/zerolog/log"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var agentName = flag.String("name", "agent-research-agent", "Agent name")
var dbHost = flag.String("db-host", "127.0.0.1", "database host")

var db *storage.Storage

func main() {
	f := false
	lg, _ := utils.ConsoleInit("", &f)

	lg.Info().Msgf("starting up...")
	var err error
	db, err = storage.NewStorage(lg, *dbHost)
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

	messages := make(map[string]*DotNode)
	edges := make(map[string]string)
	for _, rootMessage := range rootMessages {
		messages[rootMessage.Id] = getMessage(db, rootMessage.Id)
	}

	ts := time.Now()
	checked := make(map[string]struct{})
	linksChan := make(chan *DbMessageLink, 1024*1024*1024)
	for {
		// get all messages which are replies to this one...!
		messagesToAdd := make([]*LazyNodeInfo, 0)
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
						messagesToAdd = append(messagesToAdd, newLazyNodeInfo(link.Id))
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
			res := <-msg.ch
			if res == nil {
				continue
			}
			messages[res.Id] = res
		}

		lg.Info().Msgf("added %d new messages", len(messagesToAdd))

		if len(messagesToAdd) == 0 {
			break
		}
	}

	lg.Info().Msgf("got %d messages", len(messages))

	storeNodesToParquetFile("/tmp/messages.parquet", messages)
	storeEdgesToParquetFile("/tmp/edges.parquet", edges)

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

type ParquetNodeRecord struct {
	Id      string `parquet:"name=id,type=BYTE_ARRAY,convertedtype=UTF8"`
	Role    string `parquet:"name=role,type=BYTE_ARRAY,convertedtype=UTF8"`
	Content string `parquet:"name=content,type=BYTE_ARRAY,convertedtype=BINARY"`
}

type ParquetEdgeRecord struct {
	Id      string `parquet:"name=id,type=BYTE_ARRAY,convertedtype=UTF8"`
	ReplyTo string `parquet:"name=replyTo,type=BYTE_ARRAY,convertedtype=UTF8"`
}

func storeNodesToParquetFile(fName string, data map[string]*DotNode) {
	var err error
	w, err := os.Create(fName)
	if err != nil {
		log.Println("Can't create local file", err)
		return
	}

	//write
	pw, err := writer.NewParquetWriterFromWriter(w, new(ParquetNodeRecord), 4)
	if err != nil {
		log.Println("Can't create parquet writer", err)
		return
	}

	pw.RowGroupSize = 128 * 1024 * 1024 //128M
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	for _, node := range data {
		err = pw.Write(ParquetNodeRecord{
			Id:      node.Id,
			Role:    node.Role,
			Content: node.Content,
		})
		if err != nil {
			log.Println("Can't write to parquet file", err)
			return
		}
	}

	_ = pw.Flush(true)
	_ = w.Close()
}

func storeEdgesToParquetFile(fName string, data map[string]string) {
	var err error
	w, err := os.Create(fName)
	if err != nil {
		log.Println("Can't create local file", err)
		return
	}

	//write
	pw, err := writer.NewParquetWriterFromWriter(w, new(ParquetEdgeRecord), 4)
	if err != nil {
		log.Println("Can't create parquet writer", err)
		return
	}

	pw.RowGroupSize = 128 * 1024 * 1024 //128M
	pw.CompressionType = parquet.CompressionCodec_SNAPPY

	for msgId, msgReplyTo := range data {
		err = pw.Write(ParquetEdgeRecord{
			Id:      msgId,
			ReplyTo: msgReplyTo,
		})
		if err != nil {
			log.Println("Can't write to parquet file", err)
			return
		}
	}

	_ = pw.Flush(true)
	_ = w.Close()
}

func uuidToLabel(from string) string {
	return "u" + strings.ReplaceAll(from, "-", "")
}

func getMessage(db *storage.Storage, id string) *DotNode {
	label, role, content := getMessageInfo(db, id)
	return &DotNode{
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
	finalResult := []string{getLabel(message), message.Role, message.Content}

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

type DotNode struct {
	Id      string
	Label   string
	Role    string
	Content string
}

type LazyNodeInfo struct {
	id          string
	ch          chan *DotNode
	cachedValue *DotNode
}

var fetchTasks = make(chan *LazyNodeInfo, 1024*1024*32)

func init() {
	go func() {
		time.Sleep(1 * time.Second)
		batch := make([]*LazyNodeInfo, 0)
		ts := time.Now()
		for lni := range fetchTasks {
			batch = append(batch, lni)
			if len(batch) >= 512 || (time.Since(ts) > 100*time.Millisecond && len(batch) > 0) {
				// fetch multiple values....
				ids := make([]string, len(batch))
				for i, lni := range batch {
					ids[i] = lni.id
				}
				messages := make([]DbMessage, 0)
				err := db.Db.GetStructsSlice("get-messages-by-ids", &messages, ids)
				if err != nil {
					zlog.Fatal().Err(err).Msg("error getting messages")
				}

				for i, _ := range batch {
					done := false
					for _, message := range messages {
						if batch[i].id == message.Id {
							batch[i].cachedValue = &DotNode{
								Id:      message.Id,
								Label:   getLabel(message),
								Role:    message.Role,
								Content: message.Content,
							}
							batch[i].ch <- batch[i].cachedValue
							done = true
							break
						}
					}

					if !done {
						batch[i].ch <- nil // not found...!
					}
				}

				batch = make([]*LazyNodeInfo, 0)
			}
		}
	}()
}

func getLabel(message DbMessage) string {
	content := message.Content
	runesToRemove := []string{"\n", "\t", "{", "}", "(", ")", "\"", "'", "  "}
	for _, runeToRemove := range runesToRemove {
		content = strings.ReplaceAll(content, runeToRemove, " ")
	}
	content = strings.TrimSpace(content)

	return fmt.Sprint(tools.CutStringAt(content, 75))
}

func newLazyNodeInfo(id string) *LazyNodeInfo {
	ch := make(chan *DotNode, 1)
	lni := &LazyNodeInfo{
		id: id,
		ch: ch,
	}

	fetchTasks <- lni

	return lni
}
