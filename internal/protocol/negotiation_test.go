package protocol

import (
	"context"
	"errors"
	"testing"
)

func endpoint(id string, caps ...Capability) Endpoint {
	return Endpoint{ID: id, Capabilities: caps, CableID: "cable-1", CableWatts: 100, TemperatureC: 25}
}

func TestNegotiatorChoosesHighestCertifiedCommonProtocol(t *testing.T) {
	n, err := NewNegotiator(5, 20)
	if err != nil {
		t.Fatal(err)
	}
	result, err := n.Negotiate(context.Background(), Request{
		Device:         endpoint("phone", Capability{Name: "PPS", MaxWatts: 80, Certified: true}, Capability{Name: "QC", MaxWatts: 30, Certified: true}),
		Charger:        endpoint("adapter", Capability{Name: "QC", MaxWatts: 50, Certified: true}, Capability{Name: "PPS", MaxWatts: 65, Certified: true}),
		RequestedWatts: 70,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Protocol != "PPS" || result.Watts != 65 || result.Fallback {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestNegotiatorClampsToCableAndHonorsContext(t *testing.T) {
	n, _ := NewNegotiator(5, 20)
	device := endpoint("phone", Capability{Name: "PPS", MaxWatts: 90, Certified: true})
	charger := endpoint("adapter", Capability{Name: "PPS", MaxWatts: 90, Certified: true})
	charger.CableWatts = 45
	result, err := n.Negotiate(context.Background(), Request{Device: device, Charger: charger, RequestedWatts: 80})
	if err != nil || result.Watts != 45 {
		t.Fatalf("expected cable clamp, result=%+v err=%v", result, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := n.Negotiate(ctx, Request{Device: device, Charger: charger, RequestedWatts: 20}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
}

func TestNegotiatorRejectsUncertifiedOrHotEndpoints(t *testing.T) {
	n, _ := NewNegotiator(5, 20)
	device := endpoint("phone", Capability{Name: "PPS", MaxWatts: 90, Certified: false})
	charger := endpoint("adapter", Capability{Name: "PPS", MaxWatts: 90, Certified: true})
	if _, err := n.Negotiate(context.Background(), Request{Device: device, Charger: charger, RequestedWatts: 20}); !errors.Is(err, ErrNoCommonProtocol) {
		t.Fatalf("expected no common protocol, got %v", err)
	}
	device.Capabilities[0].Certified = true
	device.TemperatureC = 49
	if _, err := n.Negotiate(context.Background(), Request{Device: device, Charger: charger, RequestedWatts: 20}); !errors.Is(err, ErrUnsafeRequest) {
		t.Fatalf("expected thermal rejection, got %v", err)
	}
}
