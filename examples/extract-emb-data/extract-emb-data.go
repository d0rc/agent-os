package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"github.com/d0rc/agent-os/storage"
	"github.com/d0rc/agent-os/utils"
	"github.com/d0rc/agent-os/vectors"
	"math"
	"os"
	"strings"
)

var pathChannel = make(chan []string, 10)
var dbHost = flag.String("db-host", "167.235.115.231", "database host")

func main() {
	termUi := false
	lg, _ := utils.ConsoleInit("", &termUi)
	lg.Info().Msgf("starting up...!")
	// it's a utility function, which does the following:
	// - it looks for all the messages with given string inside
	// - it collects all the inference steps which could lead to this messages
	db, err := storage.NewStorage(lg, *dbHost)
	if err != nil {
		lg.Fatal().Err(err).Msg("error initializing storage")
	}

	type embCacheData struct {
		PromptId              uint64 `db:"lle1_id"`
		PromptNamespace       string `db:"lle1_namespace"`
		PromptNamespaceId     uint64 `db:"lle1_namespace_id"`
		PromptEmbedding       string `db:"lle1_embedding"`
		GenerationId          uint64 `db:"lle2_id"`
		GenerationNamespace   string `db:"lle2_namespace"`
		GenerationNamespaceId uint64 `db:"lle2_namespace_id"`
		GenerationEmbedding   string `db:"lle2_embedding"`
	}
	results := make([]embCacheData, 0)
	err = db.Db.GetStructsSlice("get-paired-embeddings", &results)
	if err != nil {
		lg.Fatal().Err(err).Msg("error getting paired embeddings")
	}

	extractedData := make([]extractedDataRow, len(results))

	for idx, result := range results {
		promptVector := &vectors.Vector{}
		generationVector := &vectors.Vector{}
		err = json.Unmarshal([]byte(result.PromptEmbedding), &promptVector)
		if err != nil {
			lg.Fatal().Err(err).Msg("error unmarshaling prompt embedding")
		}
		err = json.Unmarshal([]byte(result.GenerationEmbedding), &generationVector)
		if err != nil {
			lg.Fatal().Err(err).Msg("error unmarshaling generation embedding")
		}
		fmt.Printf("%d, %d -> %d, distance = %f\n",
			idx,
			result.PromptId,
			result.GenerationId,
			distance(promptVector, generationVector))

		extractedData[idx].p = promptVector.VecF64
		extractedData[idx].g = generationVector.VecF64
	}

	// save csv file
	err = saveCSV(extractedData, "paired_embeddings.csv")
	if err != nil {
		lg.Fatal().Err(err).Msg("error saving csv file")
	}
}

type extractedDataRow struct {
	p []float64
	g []float64
}

func saveCSV(data []extractedDataRow, s string) error {
	f, err := os.Create(s)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = f.Write([]byte("input,output\n"))
	if err != nil {
		return err
	}
	for _, row := range data {
		_, err = f.Write([]byte(fmt.Sprintf("'%s','%s'\n",
			toString(row.p),
			toString(row.g))))
		if err != nil {
			return err
		}
	}
	return nil
}

func toString(p []float64) string {
	tmp := make([]string, len(p))
	for i, f := range p {
		tmp[i] = fmt.Sprintf("%f", f)
	}

	return strings.Join(tmp, ";")
}

func distance(vector *vectors.Vector, vector2 *vectors.Vector) float64 {
	distance := float64(9)
	for i := 0; i < len(vector.VecF64); i++ {
		distance += (vector.VecF64[i] - vector2.VecF64[i]) * (vector.VecF64[i] - vector2.VecF64[i])
	}

	return math.Sqrt(distance)
}
