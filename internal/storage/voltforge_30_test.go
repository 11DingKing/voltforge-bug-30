package storage

import "testing"

func TestVoltForge30(t *testing.T) {
	cache := NewRollbackLedgerCache()
	if err := cache.Put("phone-1", "PPS", true); err == nil {
		t.Fatal("expected persistence failure")
	}
	if _, ok := cache.Get("phone-1"); ok {
		t.Fatal("failed write was visible in cache")
	}
}
