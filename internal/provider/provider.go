package provider

import "context"

type SendRequest struct{ To, From, Text, IdempotencyKey string }
type SendResult struct{ ProviderID string }
type Error struct {
	Status        int
	Code, Message string
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Telnyx interface {
	Send(context.Context, SendRequest) (SendResult, error)
}
type HighLevel interface {
	ForwardInbound(context.Context, Inbound) error
	SetSMSDND(context.Context, string) error
}
type Inbound struct{ LocationID, From, To, Text, ProviderEventID string }
