package storage

import (
	"context"
	"errors"
	"testing"
	"time"
	"voltforge/internal/auth"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"voltforge/internal/domain"
)

func TestSessionPersistsAcrossStoreRestart(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 10, 0, 0, 0, time.UTC)}
	first, err := NewStore(ctx, dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	service := auth.New(time.Hour, clock.Now, first)
	session, err := service.Login(ctx, "testengineer", "testengineer-demo")
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewStore(ctx, dir, clock)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	restarted := auth.New(time.Hour, clock.Now, second)
	if _, err := restarted.Authenticate(ctx, session.ID); err != nil {
		t.Fatalf("session was not restored: %v", err)
	}
	if err := restarted.Logout(ctx, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Authenticate(ctx, session.ID); !errors.Is(err, auth.ErrSessionRevoked) {
		t.Fatalf("revocation was not persisted: %v", err)
	}
}

func setupTestStore(t *testing.T) (Store, *domain.FakeClock, context.Context, func()) {
	t.Helper()
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	store, err := NewStore(ctx, dir, clock)
	require.NoError(t, err)
	return store, clock, ctx, func() { store.Close() }
}

func TestRestartRecoverPersist(t *testing.T) {
	store, clock, ctx, cleanup := setupTestStore(t)
	defer cleanup()
	now := clock.Now()
	session := &domain.ChargeSession{
		ID: "session-001", SessionNo: "MN001", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: now, Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"),
		DataVersion: 1,
	}
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
	saved, err := store.ChargeSessionRepo().Get(ctx, "session-001")
	require.NoError(t, err)
	assert.Equal(t, domain.SessionStateNegotiating, saved.State)
}

func TestPersistAfterRestart(t *testing.T) {
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	store1, err := NewStore(ctx, dir, clock)
	require.NoError(t, err)
	session := &domain.ChargeSession{
		ID: "session-persist", SessionNo: "MNP001", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateCapabilityChecked, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now(), Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"),
		DataVersion: 1,
	}
	require.NoError(t, store1.ChargeSessionRepo().Save(ctx, session))
	require.NoError(t, store1.Close())
	store2, err := NewStore(ctx, dir, clock)
	require.NoError(t, err)
	defer store2.Close()
	recovered, err := store2.ChargeSessionRepo().Get(ctx, "session-persist")
	require.NoError(t, err)
	assert.Equal(t, "MNP001", recovered.SessionNo)
	assert.Equal(t, domain.SessionStateCapabilityChecked, recovered.State)
}

func TestReplayFromShards(t *testing.T) {
	dir := t.TempDir()
	clock := &domain.FakeClock{Current: time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)}
	ctx := context.Background()
	store1, err := NewStore(ctx, dir, clock)
	require.NoError(t, err)
	session := &domain.ChargeSession{
		ID: "session-replay", SessionNo: "MNR001", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now(), Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"),
		DataVersion: 1,
	}
	require.NoError(t, store1.ChargeSessionRepo().Save(ctx, session))
	require.NoError(t, store1.Close())
	store2, err := NewStore(ctx, dir, clock)
	require.NoError(t, err)
	defer store2.Close()
	report, err := RecoverFromShards(ctx, store2)
	require.NoError(t, err)
	assert.Equal(t, 1, report.TotalShards)
	assert.Equal(t, 1, report.RebuiltShards)
	assert.True(t, report.TotalRecords >= 1)
}

func TestPaginationBoundary(t *testing.T) {
	store, clock, ctx, cleanup := setupTestStore(t)
	defer cleanup()
	for i := 0; i < 25; i++ {
		session := &domain.ChargeSession{
			ID: "session-pb-" + itoa(i), SessionNo: "PB" + itoa(i), ProtocolID: "R001", AdapterModel: "V001",
			State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
			VendorID: "S", LabID: "R", OwnerID: "test",
			RegisteredAt: clock.Now(), Version: 1, ShardID: domain.ShardIDFor("2026-08-19", "R001"),
			DataVersion: 1,
		}
		require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
		clock.Advance(1 * time.Minute)
	}
	items, total, err := store.ChargeSessionRepo().List(ctx, SessionFilter{PageSize: 10, PageOffset: 0})
	require.NoError(t, err)
	assert.Equal(t, 25, total)
	assert.Equal(t, 10, len(items))
	items2, _, err := store.ChargeSessionRepo().List(ctx, SessionFilter{PageSize: 10, PageOffset: 20})
	require.NoError(t, err)
	assert.Equal(t, 5, len(items2))
	items3, _, err := store.ChargeSessionRepo().List(ctx, SessionFilter{PageSize: 10, PageOffset: 25})
	require.NoError(t, err)
	assert.Equal(t, 0, len(items3))
}

func TestShardChecksum(t *testing.T) {
	store, clock, ctx, cleanup := setupTestStore(t)
	defer cleanup()
	shardID := domain.ShardIDFor("2026-08-19", "R001")
	session := &domain.ChargeSession{
		ID: "session-ck", SessionNo: "MNC001", ProtocolID: "R001", AdapterModel: "V001",
		State: domain.SessionStateNegotiating, DeviceModel: "JMS-01", ChargerModel: "JMS-02",
		VendorID: "S", LabID: "R", OwnerID: "test",
		RegisteredAt: clock.Now(), Version: 1, ShardID: shardID, DataVersion: 1,
	}
	require.NoError(t, store.ChargeSessionRepo().Save(ctx, session))
	assert.True(t, store.(*sqliteStore).shard.Exists(shardID))
	checksum1, count1, err := store.(*sqliteStore).shard.Checksum(shardID)
	require.NoError(t, err)
	assert.True(t, count1 > 0)
	assert.True(t, len(checksum1) > 0)
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
