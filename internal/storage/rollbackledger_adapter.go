package storage

func (c *RollbackLedgerCache) Get(key string) (string, bool) {
	value, ok := c.memory[key]
	return value, ok
}
func (c *RollbackLedgerCache) Durable(key string) (string, bool) {
	value, ok := c.durable[key]
	return value, ok
}
