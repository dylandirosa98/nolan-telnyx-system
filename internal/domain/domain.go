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

func SelectSendingNumber(raw, fallback string, owned []string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if err := ValidateE164(fallback); err != nil {
			return "", err
		}
		return fallback, nil
	}
	number, err := normalizeUSNumber(raw)
	if err != nil {
		return "", err
	}
	for _, candidate := range owned {
		if candidate == number {
			return number, nil
		}
	}
	return "", fmt.Errorf("%w", ErrUnknownSendingNumber)
}

var ErrUnknownSendingNumber = errors.New("sending number is not a Telnyx number")

func normalizeUSNumber(raw string) (string, error) {
	var digits strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	n := digits.String()
	switch {
	case strings.HasPrefix(strings.TrimSpace(raw), "+") && ValidateE164(strings.TrimSpace(raw)) == nil:
		return strings.TrimSpace(raw), nil
	case len(n) == 10:
		n = "1" + n
	case len(n) == 11 && strings.HasPrefix(n, "1"):
	default:
		return "", fmt.Errorf("%w", ErrUnknownSendingNumber)
	}
	number := "+" + n
	if err := ValidateE164(number); err != nil {
		return "", err
	}
	return number, nil
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
