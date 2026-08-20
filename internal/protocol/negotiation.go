package protocol

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

var (
	ErrNoCommonProtocol = errors.New("no compatible fast-charge protocol")
	ErrUnsafeRequest    = errors.New("requested power is outside the certified envelope")
	ErrHandshakeClosed  = errors.New("protocol handshake is closed")
)

type Capability struct {
	Name       string
	MaxWatts   int
	VoltageMin int
	VoltageMax int
	Certified  bool
}

type Endpoint struct {
	ID           string
	Capabilities []Capability
	CableID      string
	CableWatts   int
	TemperatureC float64
}

type Request struct {
	Device         Endpoint
	Charger        Endpoint
	RequestedWatts int
	RequireStable  bool
}

type Result struct {
	Protocol string
	Watts    int
	Voltage  int
	Fallback bool
}

type Negotiator struct {
	minimumVoltage int
	maximumVoltage int
}

func NewNegotiator(minimumVoltage, maximumVoltage int) (*Negotiator, error) {
	if minimumVoltage <= 0 || maximumVoltage < minimumVoltage {
		return nil, fmt.Errorf("invalid voltage range: %d-%d", minimumVoltage, maximumVoltage)
	}
	return &Negotiator{minimumVoltage: minimumVoltage, maximumVoltage: maximumVoltage}, nil
}

func (n *Negotiator) Negotiate(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, fmt.Errorf("negotiate protocol: %w", err)
	}
	if req.Device.ID == "" || req.Charger.ID == "" || req.RequestedWatts <= 0 {
		return Result{}, fmt.Errorf("invalid handshake request: %w", ErrUnsafeRequest)
	}
	if req.Device.TemperatureC >= 48 || req.Charger.TemperatureC >= 55 {
		return Result{}, fmt.Errorf("temperature blocks negotiation: %w", ErrUnsafeRequest)
	}
	charger := make(map[string]Capability, len(req.Charger.Capabilities))
	for _, cap := range req.Charger.Capabilities {
		if cap.Certified && cap.MaxWatts > 0 {
			charger[cap.Name] = cap
		}
	}
	candidates := make([]Capability, 0, len(req.Device.Capabilities))
	for _, cap := range req.Device.Capabilities {
		if other, ok := charger[cap.Name]; ok && cap.Certified {
			candidate := cap
			if other.MaxWatts < candidate.MaxWatts {
				candidate.MaxWatts = other.MaxWatts
			}
			candidates = append(candidates, candidate)
		}
	}
	if len(candidates) == 0 {
		return Result{}, ErrNoCommonProtocol
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].MaxWatts > candidates[j].MaxWatts })
	for index, cap := range candidates {
		if cap.MaxWatts > req.Charger.Capabilities[0].MaxWatts && req.RequireStable {
			continue
		}
		watts := req.RequestedWatts
		if watts > cap.MaxWatts {
			watts = cap.MaxWatts
		}
		if req.Charger.CableWatts > 0 && watts > req.Charger.CableWatts {
			watts = req.Charger.CableWatts
		}
		if watts <= 0 {
			continue
		}
		voltage := n.minimumVoltage
		if voltage > n.maximumVoltage {
			voltage = n.maximumVoltage
		}
		return Result{Protocol: cap.Name, Watts: watts, Voltage: voltage, Fallback: index > 0}, nil
	}
	return Result{}, ErrNoCommonProtocol
}
