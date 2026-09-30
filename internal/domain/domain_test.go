package domain

import (
	"testing"
)

func TestValidateE164(t *testing.T) {
	if err := ValidateE164("+13125551212"); err != nil {
		t.Fatal(err)
	}
	for _, number := range []string{"13125551212", "+1 312 555 1212", "+", "+123", "+1234567890123456"} {
		if ValidateE164(number) == nil {
			t.Fatalf("expected invalid number: %s", number)
		}
	}
}

func TestOptOutKeyword(t *testing.T) {
	for _, text := range []string{"STOP", " stop ", "STOPALL", "STOP ALL", "Unsubscribe", "cancel", "END", "quit"} {
		if !IsOptOut(text) {
			t.Fatalf("expected opt-out: %s", text)
		}
	}
	if IsOptOut("please remove me") {
		t.Fatal("natural-language request is not a keyword")
	}
}

func TestSendingNumberUsesCSVColumnWhenItMatchesTelnyx(t *testing.T) {
	owned := []string{"+18563057016", "+17405552852"}
	got, err := SelectSendingNumber("856-305-7016", "+17405552852", owned)
	if err != nil {
		t.Fatal(err)
	}
	if got != "+18563057016" {
		t.Fatalf("selected %s", got)
	}
	got, err = SelectSendingNumber("", "+17405552852", owned)
	if err != nil || got != "+17405552852" {
		t.Fatalf("blank column got %s err %v", got, err)
	}
	if _, err = SelectSendingNumber("8563057017", "+17405552852", owned); err == nil {
		t.Fatal("number that is not a Telnyx number was accepted")
	}
}

func TestRetryClassification(t *testing.T) {
	if ClassifyProviderError(429, "") != RetryTransient {
		t.Fatal("429 should retry")
	}
	if ClassifyProviderError(503, "") != RetryTransient {
		t.Fatal("503 should retry")
	}
	if ClassifyProviderError(403, "40300") != RetrySuppression {
		t.Fatal("40300 should suppress")
	}
	if ClassifyProviderError(400, "bad_request") != RetryPermanent {
		t.Fatal("400 should be permanent")
	}
}
