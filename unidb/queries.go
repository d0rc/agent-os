package unidb

import (
	"database/sql"
	"embed"
	"fmt"
	"strings"

	"github.com/blockloop/scan"
	"github.com/gchaincl/dotsql"
	"github.com/jmoiron/sqlx"
)

func (db *UniDB) TxBegin() (*sqlx.Tx, error) {
	return db.db.Beginx()
}

func TxRollback(tx *sqlx.Tx) {
	if tx != nil {
		_ = tx.Rollback()
	}
}

// rows fetching with ? params

func (db *UniDB) GetStructsSlice(name string, v interface{}, args ...interface{}) error {
	rows, err := db.getRows(db.db, name, args)
	if err != nil {
		return fmt.Errorf("error running query `%s`: %w", name, err)
	}

	err = scan.RowsStrict(v, rows)
	if err != nil {
		return fmt.Errorf("error scanning query `%s` result: %w", name, err)
	}

	return nil
}
func (db *UniDB) GetRows(name string, args ...interface{}) (*sqlx.Rows, error) {
	rows, err := db.getRows(db.db, name, args)
	if err != nil {
		return nil, fmt.Errorf("error running query `%s`: %w", name, err)
	}
	return rows, err
}
func (db *UniDB) TxGetRows(tx *sqlx.Tx, name string, args ...interface{}) (*sqlx.Rows, error) {
	rows, err := db.getRows(tx, name, args)
	if err != nil {
		return nil, fmt.Errorf("error running query `%s`: %w", name, err)
	}
	return rows, err
}
func (db *UniDB) getRows(querier sqlx.Queryer, name string, args []interface{}) (*sqlx.Rows, error) {
	query, args, err := db.getQueryAndArgsWithIn(name, args)
	if err != nil {
		return nil, err
	}
	return querier.Queryx(query, args...)
}

// rows fetching with named params

func (db *UniDB) NamedGetRows(name string, args interface{}) (*sqlx.Rows, error) {
	return db.namedGetRows(db.db, name, args)
}

func (db *UniDB) TxNamedGetRows(tx *sqlx.Tx, name string, args interface{}) (*sqlx.Rows, error) {
	return db.namedGetRows(tx, name, args)
}
func (db *UniDB) namedGetRows(queryer sqlx.Queryer, name string, arg interface{}) (*sqlx.Rows, error) {
	query, args, err := db.getNamedQueryAndArgsWithIn(name, arg)
	if err != nil {
		return nil, err
	}

	return queryer.Queryx(query, args...)
}

// row fetching

func (db *UniDB) GetRow(name string, args ...interface{}) (*sqlx.Row, error) {
	return db.getRow(db.db, name, args)
}
func (db *UniDB) TxGetRow(tx *sqlx.Tx, name string, args ...interface{}) (*sqlx.Row, error) {
	return db.getRow(tx, name, args)
}
func (db *UniDB) getRow(queryer sqlx.Queryer, name string, args []interface{}) (*sqlx.Row, error) {
	query, args, err := db.getQueryAndArgsWithIn(name, args)
	if err != nil {
		return nil, err
	}
	return queryer.QueryRowx(query, args...), nil
}

// row fetching with named parameters

func (db *UniDB) NamedGetRow(name string, arg interface{}) (*sqlx.Row, error) {
	return db.namedGetRow(db.db, name, arg)
}
func (db *UniDB) TxNamedGetRow(tx *sqlx.Tx, name string, arg interface{}) (*sqlx.Row, error) {
	return db.namedGetRow(tx, name, arg)
}
func (db *UniDB) namedGetRow(queryer sqlx.Queryer, name string, arg interface{}) (*sqlx.Row, error) {
	query, args, err := db.getNamedQueryAndArgsWithIn(name, arg)
	if err != nil {
		return nil, err
	}
	return queryer.QueryRowx(query, args...), nil
}

// exec query

func (db *UniDB) ShouldExec(name string, args ...interface{}) sql.Result {
	if res, err := db.exec(db.db, name, args); err != nil {
		panic(err.Error())
	} else {
		return res
	}
}
func (db *UniDB) Exec(name string, args ...interface{}) (sql.Result, error) {
	db.builder.logger.Trace().Msgf("running: %s(%v)", name, args)
	return db.exec(db.db, name, args)
}
func (db *UniDB) TxExec(tx *sqlx.Tx, name string, args ...interface{}) (sql.Result, error) {
	return db.exec(tx, name, args)
}
func (db *UniDB) exec(execer sqlx.Execer, name string, args []interface{}) (sql.Result, error) {
	query, args, err := db.getQueryAndArgsWithIn(name, args)
	if err != nil {
		return nil, err
	}

	return execer.Exec(query, args...)
}

// exec query with named params

func (db *UniDB) NamedExec(name string, arg interface{}) (sql.Result, error) {
	return db.namedExec(db.db, name, arg)
}
func (db *UniDB) TxNamedExec(tx *sqlx.Tx, name string, arg interface{}) (sql.Result, error) {
	return db.namedExec(tx, name, arg)
}
func (db *UniDB) namedExec(execer sqlx.Execer, name string, arg interface{}) (sql.Result, error) {
	query, args, err := db.getNamedQueryAndArgsWithIn(name, arg)
	if err != nil {
		return nil, err
	}

	return execer.Exec(query, args...)
}

// exec query with batch support

func (db *UniDB) ExecWithBatch(name string, args ...interface{}) (sql.Result, error) {
	return db.execWithBatch(db.db, name, args)
}
func (db *UniDB) TxExecWithBatch(tx *sqlx.Tx, name string, args ...interface{}) (sql.Result, error) {
	return db.execWithBatch(tx, name, args)
}
func (db *UniDB) execWithBatch(executor sqlx.Execer, name string, args []interface{}) (sql.Result, error) {
	query, err := db.dotSql.Raw(name)
	if err != nil {
		return nil, err
	}

	return executor.Exec(query, args...)
}

// exec query with named params and batch support

func (db *UniDB) NamedExecWithBatch(name string, args interface{}) (sql.Result, error) {
	return db.namedExecWithBatch(db.db, name, args)
}
func (db *UniDB) TxNamedExecWithBatch(tx *sqlx.Tx, name string, args interface{}) (sql.Result, error) {
	return db.namedExecWithBatch(tx, name, args)
}
func (db *UniDB) namedExecWithBatch(executor sqlx.Execer, name string, arg interface{}) (sql.Result, error) {
	query, err := db.dotSql.Raw(name)
	if err != nil {
		return nil, err
	}

	if named, linearArgs, err := sqlx.Named(query, arg); err != nil {
		return nil, err
	} else {
		return executor.Exec(named, linearArgs...)
	}
}

// utils

func (db *UniDB) getNamedQueryAndArgsWithIn(name string, arg interface{}) (string, []interface{}, error) {
	query, err := db.dotSql.Raw(name)
	if err != nil {
		return "", nil, err
	}

	query, args, err := sqlx.Named(query, arg)
	if err != nil {
		return "", nil, err
	}

	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return "", nil, err
	}
	query = db.db.Rebind(query)

	return query, args, nil
}

func (db *UniDB) getQueryAndArgsWithIn(name string, args []interface{}) (string, []interface{}, error) {
	query, err := db.dotSql.Raw(name)
	if err != nil {
		return "", nil, err
	}

	query, args, err = sqlx.In(query, args...)
	if err != nil {
		return "", nil, err
	}
	query = db.db.Rebind(query)

	return query, args, nil
}

func (db *UniDB) GetRawDB() *sqlx.DB {
	return db.db
}

func (db *UniDB) AddQueries(queriesFS *embed.FS) error {
	newQ, err := getQueries([]*embed.FS{queriesFS})
	if err != nil {
		return err
	}

	db.builder.dotSql = dotsql.Merge(db.builder.dotSql, newQ)
	db.dotSql = db.builder.dotSql
	return nil
}

func (db *UniDB) Close() {
	err := db.db.Close()
	if err != nil {
		db.builder.logger.Debug().
			Msg("failed to closed database")
	}
}

func getQueries(sources []*embed.FS) (*dotsql.DotSql, error) {
	res := &dotsql.DotSql{}

	for _, fs := range sources {
		entries, err := fs.ReadDir(".")
		if err != nil {
			return nil, fmt.Errorf("failed to read sql folder: %v", err.Error())
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
				bytes, err := fs.ReadFile(entry.Name())
				if err != nil {
					return nil, fmt.Errorf("failed to read %v: %v", entry.Name(), err.Error())
				}
				parsed, err := dotsql.LoadFromString(string(bytes))
				if err != nil {
					return nil, fmt.Errorf("failed to parse %v: %v", entry.Name(), err.Error())
				}
				res = dotsql.Merge(res, parsed)
			}
		}
	}

	return res, nil
}
