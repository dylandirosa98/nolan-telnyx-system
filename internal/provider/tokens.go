package provider

import "context"

type TokenRefresher interface {
	Tokens
	ForceRefresh(context.Context) (string, error)
}
