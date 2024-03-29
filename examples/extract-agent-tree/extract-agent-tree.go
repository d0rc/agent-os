package main

import (
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/examples/extract-agent-tree/fetcher"
	"github.com/d0rc/agent-os/stdlib/storage"
	"github.com/d0rc/agent-os/syslib/utils"
	"github.com/rs/zerolog"
	"github.com/xitongsys/parquet-go/parquet"
	"github.com/xitongsys/parquet-go/writer"
	"log"
	"os"
	"strings"
	"time"
)

var agentName = flag.String("name", "agent-research-agent", "Agent name")
var dbHost = flag.String("db-host", "127.0.0.1", "database host")
var dbChunkSize = flag.Int("db-chunk-size", 512, "database chunk size")
var dbParallelThreads = flag.Int("db-threads", 6, "database threads")
var outputMessagesParquet = flag.String("messages-parquet", "/tmp/messages.parquet", "output messages parquet file")
var outputEdgesParquet = flag.String("edges-parquet", "/tmp/edges.parquet", "output edges parquet file")
var outputDotFile = flag.String("dot-file", "/tmp/graph.dot", "output dot file")

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

	ts := time.Now()
	ctx := fetcher.NewFetcher(db, lg)
	messages, edges, err := ctx.FetchMessages(*agentName, *dbChunkSize, *dbParallelThreads)

	lg.Info().Msgf("got %d messages in %v", len(messages), time.Since(ts))

	ts = time.Now()
	storeNodesToParquetFile(*outputMessagesParquet, messages)
	storeEdgesToParquetFile(*outputEdgesParquet, edges)
	writeDotFile(*outputDotFile, messages, edges, lg)
	lg.Info().Msgf("done exporting files in %v", time.Since(ts))
}

func writeDotFile(fName string, messages map[string]*fetcher.DotNode, edges map[string]string, lg zerolog.Logger) {
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

	err := os.WriteFile(fName, []byte(dotFile), 0644)
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
