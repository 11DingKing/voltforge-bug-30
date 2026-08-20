package storage

import "errors"

var ErrRollbackLedgerPersist = errors.New("rollbackledger persistence failed")

type RollbackLedgerCache struct {
	memory  map[string]string
	durable map[string]string
}

func NewRollbackLedgerCache() *RollbackLedgerCache {
	return &RollbackLedgerCache{memory: make(map[string]string), durable: make(map[string]string)}
}
func (c *RollbackLedgerCache) Put(key, value string, fail bool) error {
	c.memory[key] = value
	if fail {
		return ErrRollbackLedgerPersist
	}
	c.durable[key] = value
	return nil
}
