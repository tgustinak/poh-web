package db

import (
	"context"
	"database/sql"
	"errors"
)

// InTx runs fn with queries bound to a single transaction. It commits
// when fn returns nil and rolls back otherwise; fn's error is returned
// unchanged so callers can still inspect it. Inside an existing
// transaction fn simply joins it.
//
// The pool allows a single connection, so fn must use only the queries
// it receives, never the outer ones.
func (q *Queries) InTx(ctx context.Context, fn func(*Queries) error) error {
	if _, ok := q.db.(*sql.Tx); ok {
		return fn(q)
	}

	beginner, ok := q.db.(interface {
		BeginTx(context.Context, *sql.TxOptions) (*sql.Tx, error)
	})
	if !ok {
		return errors.New("db: connection does not support transactions")
	}

	tx, err := beginner.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if err := fn(q.WithTx(tx)); err != nil {
		_ = tx.Rollback()

		return err
	}

	return tx.Commit()
}
