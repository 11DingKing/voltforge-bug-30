package storage

func (c *RollbackLedgerCache) Get(key string) (string, bool) {
	durable, committed := c.durable[key]
	if !committed {
		return "", false
	}
	memory, cached := c.memory[key]
	if cached && memory == durable {
		return memory, true
	}
	c.memory[key] = durable
	return durable, true
}
func (c *RollbackLedgerCache) Durable(key string) (string, bool) {
	value, ok := c.durable[key]
	return value, ok
}
