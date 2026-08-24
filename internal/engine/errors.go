package engine

import "errors"

var (
	ErrClosed  = errors.New("manager is closed")
	ErrTxDone  = errors.New("transaction already committed or rolled back")
	ErrCorrupt = errors.New("corrupt entry")
	ErrDDLInTx = errors.New("DDL is not allowed inside a transaction")
)
