package db

import (
	"context"
	"errors"

	"woody-wood-portail/cmd/ctx"
	"woody-wood-portail/cmd/logger"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
)

const (
	queriesContextKey string = "queries"
)

var (
	pool *pgxpool.Pool
)

type Argon2Password struct {
	Salt        string
	Hash        string
	Iterations  int32
	Parallelism int16
	Memory      int32
	Version     int32
}

type WithPassword interface {
	SetPassword(password Argon2Password)
}

func (c *CreateUserParams) SetPassword(password Argon2Password) {
	c.PwdSalt = password.Salt
	c.PwdHash = password.Hash
	c.PwdIterations = password.Iterations
	c.PwdParallelism = password.Parallelism
	c.PwdMemory = password.Memory
	c.PwdVersion = password.Version
}

func (u *UpdatePasswordParams) SetPassword(password Argon2Password) {
	u.PwdSalt = password.Salt
	u.PwdHash = password.Hash
	u.PwdIterations = password.Iterations
	u.PwdParallelism = password.Parallelism
	u.PwdMemory = password.Memory
	u.PwdVersion = password.Version
}

func Connect() (*pgxpool.Pool, error) {
	var err error
	pool, err = pgxpool.New(context.Background(), "user=postgres dbname=gate password=postgres host=localhost")
	if err != nil {
		return nil, err
	}
	return pool, nil
}

type queriesWitTx struct {
	queries *Queries
	tx      pgx.Tx
}

func Q(c echo.Context) *Queries {
	if queries, ok := c.Get(queriesContextKey).(*queriesWitTx); ok {
		return queries.queries
	}

	tx, err := pool.Begin(c.Request().Context())
	if err != nil {
		panic(err)
	}
	queries := New(tx)
	c.Set(queriesContextKey, &queriesWitTx{queries, tx})
	return queries
}

func Qtempl(templCtx context.Context) *Queries {
	c := ctx.GetEchoFromTempl(templCtx)
	return Q(c)
}

func Commit(c echo.Context) error {
	if queries, ok := c.Get(queriesContextKey).(*queriesWitTx); ok {
		logger.Log.Debug().Msg("Committing transaction")
		return queries.tx.Commit(c.Request().Context())
	}
	return errors.New("no transaction to commit")
}

func TransactionMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer func() {
				if queries, ok := c.Get(queriesContextKey).(*queriesWitTx); ok {
					if err := queries.tx.Rollback(c.Request().Context()); err == nil {
						logger.Log.Debug().Msg("Transaction rolled back because it was not committed before the end of the request handling chain")
					}
				}
			}()

			return next(c)
		}
	}
}
