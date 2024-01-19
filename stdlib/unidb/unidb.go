package unidb

import (
	"embed"
	"fmt"
	"github.com/d0rc/agent-os/stdlib/sshtunnel"
	"net"
	"strings"
	"time"

	"github.com/gchaincl/dotsql"
	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

type Builder struct {
	dbName                 string
	privateKey             ssh.AuthMethod
	privateKeyFile         string
	queriesFS              []*embed.FS
	logger                 zerolog.Logger
	hostString             string
	userName               string
	hostAddr               string
	hostDirectlyAccessible bool
	tcpTimeout             time.Duration
	dbPort                 uint
	dbDriverArgs           string
	tunnel                 *sshtunnel.SSHTunnel
	maxIdleConns           int
	maxConns               int
	maxIdleConnTime        time.Duration
	maxConnTime            time.Duration
	dotSql                 *dotsql.DotSql
	parseTime              bool
	shortHostNameSet       bool
	shortHostName          string
	ignoreEmptyQueriesFS   bool
}

type UniDB struct {
	builder *Builder
	db      *sqlx.DB
	dotSql  *dotsql.DotSql
}

func (db *UniDB) GetHost() string {
	return db.builder.hostAddr
}

func (db *UniDB) GetQueries() map[string]string {
	return db.dotSql.QueryMap()
}

func (db *UniDB) GetShortHostName() string {
	return db.builder.shortHostName
}

func NewUniDB() *Builder {
	return &Builder{
		logger:           log.Logger.With().Str("mod", "unidb").Logger(),
		shortHostNameSet: false,
		parseTime:        false,
	}
}

func (builder *Builder) WithHost(host string) *Builder {
	builder.hostString = host
	if !builder.shortHostNameSet {
		builder.logger = builder.logger.With().Str("host", host).Logger()
	}
	return builder
}

func (builder *Builder) WithHostShortName(host string) *Builder {
	builder.logger = builder.logger.With().Str("host", host).Logger()
	builder.shortHostNameSet = true
	builder.shortHostName = host
	return builder
}

func (builder *Builder) WithTCPTimeout(timeout time.Duration) *Builder {
	builder.tcpTimeout = timeout
	return builder
}

func (builder *Builder) WithDB(dbName string) *Builder {
	builder.dbName = dbName
	return builder
}

func (builder *Builder) WithPort(port uint) *Builder {
	builder.dbPort = port
	return builder
}

func (builder *Builder) WithSshKey(privateKey ssh.AuthMethod) *Builder {
	builder.privateKey = privateKey
	return builder
}

func (builder *Builder) WithSshKeyFile(keyPath string) *Builder {
	builder.privateKeyFile = keyPath
	return builder
}

func (builder *Builder) WithQueries(queries *embed.FS) *Builder {
	builder.queriesFS = append(builder.queriesFS, queries)
	return builder
}

func (builder *Builder) WithQueriesList(queries []*embed.FS) *Builder {
	if len(queries) == 0 {
		return builder
	}
	for _, q := range queries {
		if q != nil {
			builder.queriesFS = append(builder.queriesFS, q)
		}
	}
	return builder
}

func (builder *Builder) WithLogger(logger zerolog.Logger) *Builder {
	builder.logger = logger
	return builder
}

func (builder *Builder) WithDBDriverArgs(args string) *Builder {
	builder.dbDriverArgs = args
	return builder
}

func (builder *Builder) WithMaxIdleConns(mid int) *Builder {
	builder.maxIdleConns = mid
	return builder
}

func (builder *Builder) WithMaxConns(mid int) *Builder {
	builder.maxConns = mid
	return builder
}

func (builder *Builder) WithIgnoreEmptyQueriesFS(ignore bool) *Builder {
	builder.ignoreEmptyQueriesFS = ignore
	return builder
}

func (builder *Builder) WithParseTime() *Builder {
	builder.parseTime = true
	return builder
}

func (builder *Builder) WithMaxIdleConnTime(mit time.Duration) *Builder {
	builder.maxIdleConnTime = mit
	return builder
}

func (builder *Builder) WithMaxConnTime(mit time.Duration) *Builder {
	builder.maxConnTime = mit
	return builder
}

func (builder *Builder) ShouldConnect() *UniDB {
	res, err := builder.Connect()
	if err != nil {
		log.Panic().Err(err).Msgf("Failed to setup unidb[%v %v %v]: %v", builder.hostString, builder.dbPort, builder.dbName, err.Error())
	}
	return res
}

func (builder *Builder) Connect() (*UniDB, error) {
	if builder.tcpTimeout == 0 {
		builder.tcpTimeout = 3 * time.Second
	}

	if builder.dbPort == 0 {
		builder.dbPort = 3306
	}

	if builder.dbDriverArgs == "" {
		builder.dbDriverArgs = "timeout=30s&readTimeout=30s"
	}

	if builder.parseTime {
		builder.dbDriverArgs = fmt.Sprintf("%s&parseTime=true", builder.dbDriverArgs)
	}

	if builder.privateKey == nil {
		// we
	}

	if builder.privateKeyFile == "" {
		builder.privateKeyFile = "~/.ssh/id_rsa"
	}

	// first attempting to read private key from file
	if builder.privateKey == nil && builder.privateKeyFile != "" {
		pk, err := sshtunnel.PrivateKeyFile(builder.privateKeyFile)
		if err != nil {
			builder.logger.Error().
				Err(err).
				Str("key-path", builder.privateKeyFile).
				Msg("failed to read private key")
		} else {
			builder.logger.Info().Msg("loaded local user's secret key")
			builder.privateKey = pk
		}
	}

	// now let's check if need to parse host addr
	if strings.Contains(builder.hostString, "@") {
		// there should be username here... let's see
		tokens := strings.Split(builder.hostString, "@")
		if len(tokens) == 2 {
			builder.userName = tokens[0]
			builder.hostAddr = tokens[1]
		}
	} else {
		builder.userName = "root"
		builder.hostAddr = builder.hostString
	}

	if builder.queriesFS != nil {
		dotSql, err := getQueries(builder.queriesFS)
		if err != nil {
			builder.logger.Error().Err(err).
				Msg("failed to read queries fs")
			return nil, err
		}
		builder.dotSql = dotSql
	} else if !builder.ignoreEmptyQueriesFS {
		builder.logger.Warn().
			Msg("no queries defined connecting UniDB!!!")
	}

	// now let's check if host is directly accessible
	conn, err := net.DialTimeout(
		"tcp",
		fmt.Sprintf("%s:%v", builder.hostAddr, builder.dbPort),
		builder.tcpTimeout)

	connFailed := false
	var db *UniDB
	if err == nil {
		builder.hostDirectlyAccessible = true
		builder.logger.Debug().
			Str("tcp-host", builder.hostAddr).
			Msg("host is directly accessible")
		_ = conn.Close()
		db, err = builder.directConnect(builder.getDirectDbConnectString())
		if mysqlError, ok := err.(*mysql.MySQLError); ok {
			if mysqlError.Number == 1049 {
				builder.logger.Error().Err(err).
					Msg("database does not exist on directly available host")
				return nil, err
			}
		}
		if err != nil {
			builder.logger.Error().Err(err).Msg("error establishing direct connection")
			connFailed = true
		}
	}

	if err != nil || connFailed {
		builder.logger.Debug().Err(err).
			Str("tcp-host", builder.hostAddr).
			Msg("failed to dial host directly")
		builder.hostDirectlyAccessible = false
		return builder.sshConnect()
	}

	return db, nil
}

func (builder *Builder) getDirectDbConnectString() string {
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?%s",
		builder.userName,
		builder.hostAddr,
		builder.dbPort,
		builder.dbName,
		builder.dbDriverArgs)
}

func (builder *Builder) getTunneledConnectString() string {
	return fmt.Sprintf("%s@tcp(%s:%d)/%s?%s",
		builder.userName,
		"127.0.0.1",
		builder.tunnel.Local.Port,
		builder.dbName,
		builder.dbDriverArgs)
}

func (builder *Builder) sshConnect() (*UniDB, error) {
	tunnel, err := sshtunnel.NewSSHTunnel(fmt.Sprintf("%s@%s",
		builder.userName,
		builder.hostAddr),
		builder.privateKey,
		fmt.Sprintf("127.0.0.1:%v", builder.dbPort),
		builder.logger)
	builder.tunnel = tunnel

	if err != nil {
		builder.logger.Error().Err(err).
			Str("ssh-host", builder.hostAddr).
			Str("ssh-user", builder.userName).
			Msg("error creating ssh tunnel")
		return nil, err
	}

	tunnel.Start()

	builder.logger = builder.logger.With().
		Bool("ssh", true).
		Logger()

	return builder.directConnect(builder.getTunneledConnectString())
}

func (builder *Builder) directConnect(dbString string) (*UniDB, error) {
	db, err := sqlx.Open("mysql", dbString)
	if err != nil {
		builder.logger.Error().Err(err).
			Str("db-string", dbString).
			Msg("error opening connection")
		return nil, err
	}

	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	builder.logger.Debug().
		Str("host", builder.hostAddr).
		Str("db-string", dbString).
		Msg("database connected")

	if builder.maxIdleConns != 0 {
		db.SetMaxIdleConns(builder.maxIdleConns)
	} else {
		db.SetMaxIdleConns(16)
	}

	if builder.maxIdleConnTime != 0 {
		db.SetConnMaxIdleTime(builder.maxIdleConnTime)
	} else {
		db.SetConnMaxIdleTime(time.Minute)
	}

	if builder.maxConnTime != 0 {
		db.SetConnMaxLifetime(builder.maxConnTime)
	} else {
		db.SetConnMaxLifetime(30 * time.Minute)
	}

	if builder.maxConns > 0 {
		db.SetMaxOpenConns(builder.maxConns)
	} else {
		db.SetMaxOpenConns(16)
	}

	return &UniDB{
		builder: builder,
		db:      db,
		dotSql:  builder.dotSql,
	}, nil
}
