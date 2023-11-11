package storage

import (
	"crypto/sha512"
	"embed"
	"encoding/hex"
	"github.com/d0rc/agent-os/unidb"
	"github.com/rs/zerolog"
	zlog "github.com/rs/zerolog/log"
	"strings"
)

//go:embed queries.sql
var queriesFs embed.FS

type Storage struct {
	Db *unidb.UniDB
	lg zerolog.Logger
}

func NewStorage(lg zerolog.Logger) (*Storage, error) {
	db, err := unidb.NewUniDB().
		WithDB("ai_srv").
		WithHost("127.0.0.1").
		WithParseTime().
		WithMaxConns(4096).
		WithQueries(&queriesFs).
		Connect()
	if err != nil {
		return nil, err
	}

	storage := &Storage{
		Db: db,
		lg: lg,
	}
	// execute DDLs
	storage.execDDLs()

	return storage, nil
}

func (s *Storage) execDDLs() {
	for qName := range s.Db.GetQueries() {
		if strings.HasPrefix(qName, "ddl-") {
			s.lg.Info().Str("name", qName).Msg("running DDL")
			s.Db.ShouldExec(qName)
		}
	}
}

func GetHash(s string) string {
	// generate SHA-512 hash for string
	h := sha512.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

type computeCacheRecord struct {
	Id         int64  `db:"id"`
	NameSpace  string `db:"namespace"`
	TaskResult []byte `db:"task_result"`
}

func (s *Storage) GetTaskCachedResult(namespace, task string) ([]byte, error) {
	results := make([]computeCacheRecord, 0)
	err := s.Db.GetStructsSlice("get-task-cache-record", &results, namespace, GetHash(task))
	if err != nil {
		return nil, err
	}

	if len(results) > 0 {
		_, err = s.Db.Exec("mark-task-cache-hit", results[0].Id)
		if err != nil {
			zlog.Error().Err(err).Msgf("error marking task cache hit for id: %d", results[0].Id)
		}
		return results[0].TaskResult, nil
	}

	return nil, nil
}

func (s *Storage) SaveTaskCacheResult(namespace, task string, result []byte) error {
	_, err := s.Db.Exec("save-task-cache-record", namespace, GetHash(task), result)
	return err
}
