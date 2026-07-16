package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Executor is the query surface shared by *pgxpool.Pool and pgx.Tx. Repositories
// bind their generated queries to whatever Store.DB hands them, so the same code
// runs on the pool or inside a transaction without knowing which.
type Executor interface {
	Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error)
	Query(context.Context, string, ...interface{}) (pgx.Rows, error)
	QueryRow(context.Context, string, ...interface{}) pgx.Row
}

// txKey is unexported so a transaction can only enter a context through Do and
// can only leave it through Store.DB — no module can hand one around by hand.
type txKey struct{}

// Store hands out the database executor for the current scope and is the only
// way to open a transaction.
//
// A transaction lives in the context, not in a parameter, which is what lets one
// business operation span modules that know nothing about each other:
//
//	store.Do(ctx, func(ctx context.Context) error {
//	    if err := cart.Load(ctx, id); err != nil { return err }      // same tx
//	    if err := product.Reserve(ctx, items); err != nil { return err }
//	    return wallet.Debit(ctx, userID, total)                      // same tx
//	})
//
// Any error rolls back every write, including those made by modules that never
// heard of the transaction. Since a repository has no route to the database
// except Store.DB(ctx), and Store.DB always consults the context, joining the
// caller's transaction is automatic rather than something each module must
// remember to do.
type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// DB returns the executor for the current scope: the transaction carried by ctx
// when called inside Do, otherwise the pool. Repositories must obtain every
// executor through this method and must never capture the result — the answer is
// per-call, and caching it is exactly the bug this design removes.
func (s *Store) DB(ctx context.Context) Executor {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		return tx
	}
	return s.pool
}

// Pool exposes the underlying pool for work that is not a query — health checks,
// stats, LISTEN/NOTIFY. Never use it to run repository queries: it ignores the
// context's transaction and would silently write outside it.
func (s *Store) Pool() *pgxpool.Pool { return s.pool }

// Do runs fn inside a transaction, passing it a ctx that carries that
// transaction. It commits when fn returns nil and rolls back on any error or
// panic.
//
// Nested calls join the outer transaction through a savepoint: an inner failure
// undoes only the inner work, and the outermost Do still owns the final commit.
// This makes a usecase that opens its own transaction safe to call from a larger
// one.
//
// The transaction runs on a single connection and pgx.Tx is not safe for
// concurrent use, so fn must not fan its ctx out to parallel goroutines.
func (s *Store) Do(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	tx, err := s.begin(ctx)
	if err != nil {
		return err
	}

	defer func() {
		if p := recover(); p != nil {
			_ = rollback(ctx, tx)
			panic(p)
		}
		if err != nil {
			if rbErr := rollback(ctx, tx); rbErr != nil {
				err = errors.Join(err, rbErr)
			}
		}
	}()

	if err = fn(context.WithValue(ctx, txKey{}, tx)); err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit tx: %w", err)
	}
	return nil
}

func (s *Store) begin(ctx context.Context) (pgx.Tx, error) {
	if tx, ok := ctx.Value(txKey{}).(pgx.Tx); ok {
		sp, err := tx.Begin(ctx) // pgx turns Begin on a live tx into a savepoint
		if err != nil {
			return nil, fmt.Errorf("begin savepoint: %w", err)
		}
		return sp, nil
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	return tx, nil
}

// rollback undoes the transaction even when ctx is already cancelled — the usual
// reason a write fails is the very cancellation that would otherwise leave the
// connection holding an open transaction back in the pool. A tx already closed
// by the server (a failed commit rolls back on its own) is not an error.
func rollback(ctx context.Context, tx pgx.Tx) error {
	if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		return fmt.Errorf("rollback tx: %w", err)
	}
	return nil
}
