package kv

import "errors"

var (
	ErrKeyNotFound = errors.New("kv: key not found")
	ErrTxnTooBig   = errors.New("kv: txn too big")
	ErrInvalidNum  = errors.New("kv: invalid num")
)
