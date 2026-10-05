package provider

import (
	"context"
	"sync"
)

type FakeTelnyx struct {
	Mu    sync.Mutex
	Sent  []SendRequest
	Owned []string
	Err   error
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

func (f *FakeTelnyx) OwnedNumbers(context.Context) ([]string, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	return append([]string(nil), f.Owned...), nil
}

type FakeHighLevel struct {
	Mu                    sync.Mutex
	Inbound               []Inbound
	Promoted              []Inbound
	DND                   []string
	Statuses              map[string]string
	CRM                   []CRMJob
	Contacts              []ContactHit
	Conversations         []Conversation
	FromNumbers           map[string]string
	LastConversationLimit int
	Err                   error
}

func (f *FakeHighLevel) PromoteInbound(_ context.Context, i Inbound) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.Promoted = append(f.Promoted, i)
	return nil
}

func (f *FakeHighLevel) ForwardInbound(_ context.Context, i Inbound) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.Inbound = append(f.Inbound, i)
	return nil
}
func (f *FakeHighLevel) SetSMSDND(_ context.Context, n string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.DND = append(f.DND, n)
	return nil
}

func (f *FakeHighLevel) UpdateMessageStatus(_ context.Context, messageID, status string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	if f.Statuses == nil {
		f.Statuses = make(map[string]string)
	}
	f.Statuses[messageID] = status
	return nil
}

func (f *FakeHighLevel) ExecuteCRM(_ context.Context, job CRMJob) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return f.Err
	}
	f.CRM = append(f.CRM, job)
	return nil
}

type StaticToken string

func (s StaticToken) Token(context.Context) (string, error) { return string(s), nil }

func (f *FakeHighLevel) SearchContacts(_ context.Context, _ string) ([]ContactHit, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return nil, f.Err
	}
	return append([]ContactHit(nil), f.Contacts...), nil
}

func (f *FakeHighLevel) RecentConversations(_ context.Context, _ string, limit int) ([]Conversation, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	f.LastConversationLimit = limit
	if f.Err != nil {
		return nil, f.Err
	}
	if limit <= 0 || limit > len(f.Conversations) {
		limit = len(f.Conversations)
	}
	return append([]Conversation(nil), f.Conversations[:limit]...), nil
}

func (f *FakeHighLevel) SendingNumber(_ context.Context, contactID string) (string, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Err != nil {
		return "", f.Err
	}
	return f.FromNumbers[contactID], nil
}
