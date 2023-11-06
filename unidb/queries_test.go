package unidb

import (
	"embed"
	"fmt"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"os"
	"testing"
)

//go:embed test_queries.sql
var queries embed.FS

func TestUniDB_Query(t *testing.T) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: "02/01 15:04:05"})

	uniDB, err := NewUniDB().
		WithHost("root@mysql"). //local
		WithDB("mysql").
		WithQueries(&queries).
		Connect()
	if err != nil {
		panic(fmt.Errorf("error connecting to DB: %w", err))
	}

	rows, err := uniDB.GetRows("select-one")
	if err != nil {
		panic(fmt.Errorf("error running query: %w", err))
	}

	for rows.Next() {
		var rowValue uint64
		err = rows.Scan(&rowValue)
		if err != nil {
			panic(fmt.Errorf("error scanning row: %w", err))
		}
		log.Info().Msgf("got row value = %v", rowValue)
	}
}
