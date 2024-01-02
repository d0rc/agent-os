package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/examples/extract-agent-tree/fetcher"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/utils"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
	"log"
	"os"
	"strings"
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

	ctx := fetcher.NewFetcher(db, lg)
	messages, edges, err := ctx.FetchMessages(*agentName)

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

func storeNodesToParquetFile(fName string, data map[string]*fetcher.DotNode) {
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
