package domain

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var e164 = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)

func ValidateE164(number string) error {
	if !e164.MatchString(number) {
		return fmt.Errorf("invalid E.164 number")
	}
	return nil
}

func IsOptOut(text string) bool {
	switch strings.ToUpper(strings.Join(strings.Fields(text), " ")) {
	case "STOP", "STOPALL", "STOP ALL", "UNSUBSCRIBE", "CANCEL", "END", "QUIT":
		return true
	default:
		return false
	}
}

type RetryClass string

const (
	RetryTransient   RetryClass = "transient"
	RetryPermanent   RetryClass = "permanent"
	RetrySuppression RetryClass = "suppression"
)

func ClassifyProviderError(status int, code string) RetryClass {
	if code == "40300" {
		return RetrySuppression
	}
	if status == 408 || status == 425 || status == 429 || status >= 500 {
		return RetryTransient
	}
	return RetryPermanent
}

var ErrSuppressed = errors.New("destination is suppressed")
