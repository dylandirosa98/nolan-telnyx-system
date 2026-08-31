package provider

import (
	"context"
	"sync"
)

type FakeTelnyx struct {
	Mu   sync.Mutex
	Sent []SendRequest
	Err  error
}

func (f *FakeTelnyx) Send(_ context.Context, r SendRequest) (SendResult, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return SendResult{}, f.Err
	}
	f.Sent = append(f.Sent, r)
	return SendResult{ProviderID: "fake-" + r.IdempotencyKey}, nil
}

type FakeHighLevel struct {
	Mu      sync.Mutex
	Inbound []Inbound
	DND     []string
}

func (f *FakeHighLevel) ForwardInbound(_ context.Context, i Inbound) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.Inbound = append(f.Inbound, i)
	return nil
}
func (f *FakeHighLevel) SetSMSDND(_ context.Context, n string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.DND = append(f.DND, n)
	return nil
}
