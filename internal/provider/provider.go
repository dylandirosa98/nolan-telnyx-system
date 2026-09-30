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
	OwnedNumbers(context.Context) ([]string, error)
}

type CRMJob struct {
	Action, Body, ContactID, LocationID, Phone, Reply string
}

type ContactHit struct {
	ID, Name, Phone string
}

type HighLevel interface {
	ForwardInbound(context.Context, Inbound) error
	SetSMSDND(context.Context, string) error
	UpdateMessageStatus(context.Context, string, string) error
	ExecuteCRM(context.Context, CRMJob) error
	SearchContacts(context.Context, string) ([]ContactHit, error)
	SendingNumber(context.Context, string) (string, error)
}

type Inbound struct {
	LocationID, ContactID, ConversationID string
	From, To, Text, ProviderEventID       string
}

type Tokens interface {
	Token(context.Context) (string, error)
}
