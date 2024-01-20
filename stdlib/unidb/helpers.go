package unidb

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

type Args map[string]any

func Transactional[T any](db *UniDB, fn func(tx *sqlx.Tx) (T, error)) (res T, err error) {
	tx, err := db.TxBegin()
	if err != nil {
		return res, fmt.Errorf("failed to start transaction: %w", err)
	}

	res, err = fn(tx)
	if err != nil {
		_ = tx.Rollback()
		return res, err
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return
}

func TransactionalExec(db *UniDB, fn func(tx *sqlx.Tx) error) error {
	tx, err := db.TxBegin()
	if err != nil {
		return fmt.Errorf("failed to start transaction: %w", err)
	}

	err = fn(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}
