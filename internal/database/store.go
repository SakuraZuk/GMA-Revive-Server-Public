package database

import "context"

// Store is the boundary for persistent game data. Handlers must use context
// deadlines and never hold a transaction while doing network I/O.
type Store interface {
	Ping(context.Context) error
	Close()
}

type Unconfigured struct{}

func (Unconfigured) Ping(context.Context) error { return nil }
func (Unconfigured) Close()                     {}
