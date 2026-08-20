package service

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
	"voltforge/internal/storage"
)

func setupTestEnv(t *testing.T) (storage.Store, *domain.FakeClock, *EventBus, context.Context, func()) {
	t.Helper()
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	store, err := storage.NewStore(ctx, dir, clock)
	require.NoError(t, err)
	bus := NewEventBus(store.EventRepo())
	cleanup := func() {
		store.Close()
	}
	return store, clock, bus, ctx, cleanup
}

func registerTestHandshake(t *testing.T, ctx context.Context, handSvc *HandshakeService, formNo, date, protocolID string) *domain.ProtocolHandshake {
	t.Helper()
	form, err := handSvc.Register(ctx, RegisterHandshakeRequest{
		FormNo: formNo, Date: date, ProtocolID: protocolID, AdapterModel: "V001",
		OutboundProduct: "JMS-01", ArrivalProduct: "JMS-02", OwnerID: "tester",
		ChargeSessions: []RegisterChargeSession{
			{SessionNo: formNo + "-M1", VendorID: "S1", LabID: "R1"},
		},
	})
	require.NoError(t, err)
	return form
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf []byte
	if i < 0 {
		buf = append(buf, '-')
		i = -i
	}
	var digits []byte
	for i > 0 {
		digits = append(digits, byte('0'+i%10))
		i /= 10
	}
	for j := len(digits) - 1; j >= 0; j-- {
		buf = append(buf, digits[j])
	}
	if len(buf) == 0 {
		return "0"
	}
	return string(buf)
}
