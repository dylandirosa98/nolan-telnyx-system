package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestStartRequiresDocumentedConsentForSMS(t *testing.T) {
	definition := enabledDefinition(t, "SMS Blast Full Address")
	_, err := Start(definition, Contact{ID: "contact-1", Phone: "+13105551212"}, time.Now())
	if err == nil || !strings.Contains(err.Error(), "consent") {
		t.Fatalf("Start error=%v, want consent error", err)
	}
}

func TestRecurringWorkflowRendersAndRepeatsUntilReply(t *testing.T) {
	definition := enabledDefinition(t, "1 x month Text")
	now := time.Date(2026, 9, 2, 14, 0, 0, 0, time.UTC)
	enrollment, err := Start(definition, consentedContact(now, map[string]string{
		"contact.full_address": "123 Main St",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := enrollment.NextRun, now.Add(30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("NextRun=%s want %s", got, want)
	}

	commands, err := Advance(definition, &enrollment, enrollment.NextRun)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Type != CommandSendSMS {
		t.Fatalf("commands=%#v", commands)
	}
	if got := commands[0].Body; got != "Hi, this is Example Company following up about 123 Main St. Has anything changed? Reply STOP to opt out." {
		t.Fatalf("body=%q", got)
	}
	if got, want := enrollment.NextRun, now.Add(60*24*time.Hour); !got.Equal(want) {
		t.Fatalf("repeat NextRun=%s want %s", got, want)
	}

	replyCommands := Reply(definition, &enrollment, "Yes, call me")
	if enrollment.State != StateCompleted || !enrollment.NextRun.IsZero() {
		t.Fatalf("reply did not stop recurrence: %#v", enrollment)
	}
	if len(replyCommands) != 1 || replyCommands[0].Type != CommandCRMAction || replyCommands[0].Action != "monthly_reply" {
		t.Fatalf("reply commands=%#v", replyCommands)
	}
}

func TestBlastSelectsOneRealVariantAndStopsOnReply(t *testing.T) {
	definition := enabledDefinition(t, "SMS Blast Full Address")
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	enrollment, err := Start(definition, consentedContact(now, map[string]string{
		"contact.first_name":   "Sam",
		"contact.full_address": "123 Main St",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := Advance(definition, &enrollment, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Type != CommandSendSMS || !strings.Contains(commands[0].Body, "Sam") || !strings.Contains(commands[0].Body, "123 Main St") {
		t.Fatalf("commands=%#v", commands)
	}
	if enrollment.State != StateAwaitingReply || !enrollment.NextRun.IsZero() {
		t.Fatalf("blast state=%#v", enrollment)
	}

	replyCommands := Reply(definition, &enrollment, "not interested")
	if enrollment.State != StateCompleted {
		t.Fatalf("state=%q want completed", enrollment.State)
	}
	if len(replyCommands) != 2 || replyCommands[0].Type != CommandCRMAction || replyCommands[0].Action != "not_interested" || replyCommands[1].Type != CommandArchiveConversation {
		t.Fatalf("reply commands=%#v", replyCommands)
	}
}

func TestOptOutAlwaysSuppressesAndStops(t *testing.T) {
	definition := enabledDefinition(t, "1x week follow up")
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	enrollment, err := Start(definition, consentedContact(now, map[string]string{
		"contact.full_address": "123 Main St",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	commands := Reply(definition, &enrollment, " STOP ")
	if enrollment.State != StateSuppressed || len(commands) != 1 || commands[0].Type != CommandSuppress {
		t.Fatalf("enrollment=%#v commands=%#v", enrollment, commands)
	}
}

func TestRecurringWorkflowStopsAtConfiguredMaximum(t *testing.T) {
	definition := enabledDefinition(t, "1x week follow up")
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	enrollment, err := Start(definition, consentedContact(now, map[string]string{
		"contact.full_address": "123 Main St",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	enrollment.SentCount = definition.MaxOccurrences
	commands, err := Advance(definition, &enrollment, enrollment.NextRun)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 0 || enrollment.State != StateCompleted || !enrollment.NextRun.IsZero() {
		t.Fatalf("enrollment=%#v commands=%#v", enrollment, commands)
	}
}

func TestManualFollowUpProducesCRMCommandsWithoutSMSConsent(t *testing.T) {
	definition := enabledDefinition(t, "Manual Follow Up 2 Week")
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	enrollment, err := Start(definition, Contact{ID: "contact-1"}, now)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := Advance(definition, &enrollment, enrollment.NextRun)
	if err != nil {
		t.Fatal(err)
	}
	if enrollment.State != StateCompleted || len(commands) != 2 {
		t.Fatalf("enrollment=%#v commands=%#v", enrollment, commands)
	}
	if commands[0].Type != CommandCreateTask || commands[0].Body != "14 day follow uo" || commands[1].Type != CommandCRMAction {
		t.Fatalf("commands=%#v", commands)
	}
}

func TestQuietHoursRescheduleInsteadOfSending(t *testing.T) {
	definition := enabledDefinition(t, "SMS Blast Full Address")
	now := time.Date(2026, 9, 2, 2, 0, 0, 0, time.FixedZone("EDT", -4*60*60))
	enrollment, err := Start(definition, consentedContact(now, map[string]string{
		"contact.first_name":   "Sam",
		"contact.full_address": "123 Main St",
	}), now)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := Advance(definition, &enrollment, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 0 {
		t.Fatalf("sent during quiet hours: %#v", commands)
	}
	want := time.Date(2026, 9, 2, 11, 30, 0, 0, now.Location())
	if !enrollment.NextRun.Equal(want) {
		t.Fatalf("NextRun=%s want %s", enrollment.NextRun, want)
	}
}

func TestEveryCatalogWorkflowCanStartAndAdvance(t *testing.T) {
	now := time.Date(2026, 9, 2, 16, 0, 0, 0, time.UTC)
	variables := map[string]string{
		"contact.first_name":   "Sam",
		"contact.full_address": "123 Main St, Baltimore, MD",
		"contact.address1":     "123 Main St",
		"contact.city":         "Baltimore",
		"location.name":        "Example Company",
	}
	for _, original := range LegacyCatalog() {
		definition := original
		definition.Enabled = true
		contact := Contact{ID: "contact-" + definition.Key, Phone: "+13105551212", Variables: variables}
		if len(definition.Messages) > 0 {
			contact.ConsentAt = now.Add(-time.Hour)
			contact.ConsentSource = "test affirmative reply"
		}
		enrollment, err := Start(definition, contact, now)
		if err != nil {
			t.Errorf("%s start: %v", definition.Name, err)
			continue
		}
		commands, err := Advance(definition, &enrollment, enrollment.NextRun)
		if err != nil {
			t.Errorf("%s advance: %v", definition.Name, err)
			continue
		}
		if len(commands) == 0 {
			t.Errorf("%s produced no command", definition.Name)
		}
	}
}

func enabledDefinition(t *testing.T, name string) Definition {
	t.Helper()
	definition, ok := FindByName(LegacyCatalog(), name)
	if !ok {
		t.Fatalf("missing workflow %q", name)
	}
	definition.Enabled = true
	return definition
}

func consentedContact(now time.Time, variables map[string]string) Contact {
	if variables == nil {
		variables = map[string]string{}
	}
	if _, ok := variables["location.name"]; !ok {
		variables["location.name"] = "Example Company"
	}
	return Contact{ID: "contact-1", Phone: "+13125551212", ConsentAt: now.Add(-time.Hour), ConsentSource: "affirmative inbound SMS", Variables: variables}
}
