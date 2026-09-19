package persistencetest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"sync/atomic"
)

// QueryCounter counts statements issued through a wrapped driver so contract
// tests can assert that a read path stays set-based rather than degrading into
// per-row lookups. The triage queue regressed into thousands of round-trips
// once; this is the guard against it happening again.
type QueryCounter struct {
	n atomic.Int64
}

// Count returns the number of statements issued since the last Reset.
func (c *QueryCounter) Count() int { return int(c.n.Load()) }

// Reset zeroes the counter, typically just before the call under assertion.
func (c *QueryCounter) Reset() { c.n.Store(0) }

func (c *QueryCounter) add() { c.n.Add(1) }

var countingDriverSeq atomic.Int64

// OpenCounting opens dsn through baseDriver with every statement counted.
func OpenCounting(baseDriver, dsn string) (*sql.DB, *QueryCounter, error) {
	probe, err := sql.Open(baseDriver, dsn)
	if err != nil {
		return nil, nil, err
	}
	base := probe.Driver()
	_ = probe.Close()

	counter := &QueryCounter{}
	name := fmt.Sprintf("counting-%s-%d", baseDriver, countingDriverSeq.Add(1))
	sql.Register(name, &countingDriver{base: base, counter: counter})

	db, err := sql.Open(name, dsn)
	if err != nil {
		return nil, nil, err
	}
	return db, counter, nil
}

type countingDriver struct {
	base    driver.Driver
	counter *QueryCounter
}

func (d *countingDriver) Open(name string) (driver.Conn, error) {
	c, err := d.base.Open(name)
	if err != nil {
		return nil, err
	}
	return &countingConn{base: c, counter: d.counter}, nil
}

type countingConn struct {
	base    driver.Conn
	counter *QueryCounter
}

func (c *countingConn) Prepare(query string) (driver.Stmt, error) {
	s, err := c.base.Prepare(query)
	if err != nil {
		return nil, err
	}
	return &countingStmt{base: s, counter: c.counter}, nil
}

func (c *countingConn) Close() error { return c.base.Close() }

func (c *countingConn) Begin() (driver.Tx, error) { return c.base.Begin() } //nolint:staticcheck // driver fallback

func (c *countingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	if p, ok := c.base.(driver.ConnPrepareContext); ok {
		s, err := p.PrepareContext(ctx, query)
		if err != nil {
			return nil, err
		}
		return &countingStmt{base: s, counter: c.counter}, nil
	}
	return c.Prepare(query)
}

func (c *countingConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	if b, ok := c.base.(driver.ConnBeginTx); ok {
		return b.BeginTx(ctx, opts)
	}
	return c.base.Begin() //nolint:staticcheck // driver fallback
}

func (c *countingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := c.base.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.counter.add()
	return q.QueryContext(ctx, query, args)
}

func (c *countingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	e, ok := c.base.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	c.counter.add()
	return e.ExecContext(ctx, query, args)
}

func (c *countingConn) Ping(ctx context.Context) error {
	if p, ok := c.base.(driver.Pinger); ok {
		return p.Ping(ctx)
	}
	return nil
}

func (c *countingConn) ResetSession(ctx context.Context) error {
	if s, ok := c.base.(driver.SessionResetter); ok {
		return s.ResetSession(ctx)
	}
	return nil
}

func (c *countingConn) IsValid() bool {
	if v, ok := c.base.(driver.Validator); ok {
		return v.IsValid()
	}
	return true
}

type countingStmt struct {
	base    driver.Stmt
	counter *QueryCounter
}

func (s *countingStmt) Close() error  { return s.base.Close() }
func (s *countingStmt) NumInput() int { return s.base.NumInput() }

func (s *countingStmt) Exec(args []driver.Value) (driver.Result, error) {
	s.counter.add()
	return s.base.Exec(args) //nolint:staticcheck // driver fallback
}

func (s *countingStmt) Query(args []driver.Value) (driver.Rows, error) {
	s.counter.add()
	return s.base.Query(args) //nolint:staticcheck // driver fallback
}

func (s *countingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	e, ok := s.base.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	s.counter.add()
	return e.ExecContext(ctx, args)
}

func (s *countingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	q, ok := s.base.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	s.counter.add()
	return q.QueryContext(ctx, args)
}
