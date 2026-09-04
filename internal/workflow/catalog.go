package workflow

import (
	"fmt"
	"time"
)

type LegacyStatus string

const (
	StatusPublished LegacyStatus = "published"
	StatusDraft     LegacyStatus = "draft"
)

type Kind string

const (
	KindRecurringSMS Kind = "recurring_sms"
	KindSingleSMS    Kind = "single_sms"
	KindManualFollow Kind = "manual_follow_up"
)

type Rate struct {
	BatchSize int
	Per       time.Duration
}

type SendWindow struct {
	StartMinute int
	EndMinute   int
}

type Definition struct {
	Key                string
	Name               string
	LegacyStatus       LegacyStatus
	Enabled            bool
	Kind               Kind
	InitialDelay       time.Duration
	RepeatEvery        time.Duration
	MaxOccurrences     int
	LegacyVariantCount int
	Messages           []string
	Rate               Rate
	Window             SendWindow
	StopOnAnyReply     bool
	ArchiveNegative    bool
	CRMAction          string
	TaskBody           string
}

func (d Definition) Validate() error {
	if d.Key == "" || d.Name == "" {
		return fmt.Errorf("identity is required")
	}
	switch d.Kind {
	case KindRecurringSMS:
		if d.InitialDelay <= 0 || d.RepeatEvery < 0 || d.MaxOccurrences <= 0 || len(d.Messages) == 0 {
			return fmt.Errorf("delayed SMS requires a delay, bounded occurrences, and message")
		}
	case KindSingleSMS:
		if len(d.Messages) == 0 || d.MaxOccurrences != 1 || d.Rate.BatchSize <= 0 || d.Rate.Per <= 0 {
			return fmt.Errorf("single SMS requires messages and a positive rate")
		}
	case KindManualFollow:
		if d.InitialDelay <= 0 || d.TaskBody == "" {
			return fmt.Errorf("manual follow-up requires delay and task body")
		}
	default:
		return fmt.Errorf("unknown workflow kind %q", d.Kind)
	}
	if len(d.Messages) > 0 && (d.Window.StartMinute < 0 || d.Window.EndMinute > 24*60 || d.Window.StartMinute >= d.Window.EndMinute) {
		return fmt.Errorf("invalid send window")
	}
	return nil
}

func FindByName(definitions []Definition, name string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.Name == name {
			return definition, true
		}
	}
	return Definition{}, false
}

func EnabledCatalog(keys []string) (map[string]Definition, error) {
	enabled := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		if key != "" {
			enabled[key] = struct{}{}
		}
	}
	definitions := make(map[string]Definition, len(LegacyCatalog()))
	for _, definition := range LegacyCatalog() {
		if _, ok := enabled[definition.Key]; ok {
			if definition.LegacyStatus == StatusDraft {
				return nil, fmt.Errorf("draft workflow %q cannot be enabled", definition.Key)
			}
			definition.Enabled = true
			delete(enabled, definition.Key)
		}
		definitions[definition.Key] = definition
	}
	for key := range enabled {
		return nil, fmt.Errorf("unknown workflow key %q", key)
	}
	return definitions, nil
}

func LegacyCatalog() []Definition {
	const (
		defaultStart = 9 * 60
		defaultEnd   = 20 * 60
		blastStart   = 11*60 + 30
		blastEnd     = 19 * 60
	)
	followUpText := "Hi, this is {{location.name}} following up about {{contact.full_address}}. Has anything changed? Reply STOP to opt out."
	blastText := "Hi {{contact.first_name}}, this is {{location.name}}. Would you be open to discussing {{contact.full_address}}? Reply STOP to opt out."
	manual := func(key, name string, delay time.Duration, body string) Definition {
		return Definition{Key: key, Name: name, LegacyStatus: StatusPublished, Kind: KindManualFollow, InitialDelay: delay, CRMAction: "manual_follow_up", TaskBody: body}
	}
	blast := func(key, name string, status LegacyStatus, messages []string, batch int, per time.Duration, start, end int) Definition {
		return Definition{Key: key, Name: name, LegacyStatus: status, Kind: KindSingleSMS, MaxOccurrences: 1, LegacyVariantCount: 5, Messages: messages, Rate: Rate{BatchSize: batch, Per: per}, Window: SendWindow{StartMinute: start, EndMinute: end}, StopOnAnyReply: true, ArchiveNegative: true, CRMAction: "blast_reply"}
	}

	return []Definition{
		{Key: "monthly-text", Name: "1 x month Text", LegacyStatus: StatusPublished, Kind: KindRecurringSMS, InitialDelay: 30 * 24 * time.Hour, RepeatEvery: 30 * 24 * time.Hour, MaxOccurrences: 12, Messages: []string{followUpText}, Window: SendWindow{StartMinute: defaultStart, EndMinute: defaultEnd}, StopOnAnyReply: true, CRMAction: "monthly_reply"},
		{Key: "three-month-text", Name: "1x 3months text", LegacyStatus: StatusPublished, Kind: KindRecurringSMS, InitialDelay: 90 * 24 * time.Hour, RepeatEvery: 90 * 24 * time.Hour, MaxOccurrences: 12, Messages: []string{followUpText}, Window: SendWindow{StartMinute: defaultStart, EndMinute: defaultEnd}, StopOnAnyReply: true, CRMAction: "three_month_reply"},
		{Key: "weekly-follow-up", Name: "1x week follow up", LegacyStatus: StatusPublished, Kind: KindRecurringSMS, InitialDelay: 7 * 24 * time.Hour, RepeatEvery: 7 * 24 * time.Hour, MaxOccurrences: 12, Messages: []string{followUpText}, Window: SendWindow{StartMinute: defaultStart, EndMinute: defaultEnd}, StopOnAnyReply: true, CRMAction: "weekly_reply"},
		{Key: "six-day-offer-follow-up", Name: "6 day offer followup", LegacyStatus: StatusPublished, Kind: KindRecurringSMS, InitialDelay: 6 * 24 * time.Hour, RepeatEvery: 6 * 24 * time.Hour, MaxOccurrences: 12, Messages: []string{followUpText}, Window: SendWindow{StartMinute: defaultStart, EndMinute: defaultEnd}, StopOnAnyReply: true, CRMAction: "six_day_reply"},
		blast("copy-copy-sms-blast", "Copy - Copy - SMS Blast", StatusDraft, []string{blastText}, 1, 3*time.Minute, defaultStart, defaultEnd),
		manual("manual-follow-up-six-month", "Copy - Manual Follow Up 6 Month", 180*24*time.Hour, "6 mo follow uo"),
		blast("copy-sms-blast", "Copy - SMS Blast", StatusDraft, []string{blastText}, 6, time.Minute, blastStart, blastEnd),
		{Key: "every-two-day-check-in", Name: "Every 2 Day check in", LegacyStatus: StatusPublished, Kind: KindRecurringSMS, InitialDelay: 2 * 24 * time.Hour, RepeatEvery: 2 * 24 * time.Hour, MaxOccurrences: 12, Messages: []string{followUpText}, Window: SendWindow{StartMinute: defaultStart, EndMinute: defaultEnd}, StopOnAnyReply: true, CRMAction: "remove_self_on_reply"},
		manual("manual-follow-up-one-month", "Manual Follow Up 1 Month", 30*24*time.Hour, "1 mo follow uo"),
		manual("manual-follow-up-one-week", "Manual Follow Up 1 Week", 7*24*time.Hour, "7 day follow uo"),
		manual("manual-follow-up-two-month", "Manual Follow Up 2 Month", 60*24*time.Hour, "2 mo follow uo"),
		manual("manual-follow-up-two-week", "Manual Follow Up 2 Week", 14*24*time.Hour, "14 day follow uo"),
		manual("manual-follow-up-three-month", "Manual Follow Up 3 Month", 90*24*time.Hour, "3 mo follow uo"),
		blast("sms-blast", "SMS Blast", StatusDraft, []string{blastText}, 1, 135*time.Second, defaultStart, defaultEnd),
		blast("sms-blast-full-address", "SMS Blast Full Address", StatusPublished, []string{blastText}, 7, time.Minute, blastStart, blastEnd),
		blast("upper-darby-sms-blast", "Upper Darby- SMS Blast", StatusDraft, []string{blastText}, 10, time.Minute, defaultStart, defaultEnd),
		blast("sms-v2", "smsv2", StatusDraft, []string{blastText}, 1, 105*time.Second, 8*60, 17*60+30),
	}
}
