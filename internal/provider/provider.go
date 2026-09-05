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

type CRMJob struct {
	Action, Body, ContactID, LocationID, Phone, Reply string
}

type HighLevel interface {
	ForwardInbound(context.Context, Inbound) error
	SetSMSDND(context.Context, string) error
	UpdateMessageStatus(context.Context, string, string) error
	ExecuteCRM(context.Context, CRMJob) error
}

type Inbound struct {
	LocationID, ContactID, ConversationID string
	From, To, Text, ProviderEventID       string
}

type Tokens interface {
	Token(context.Context) (string, error)
}
