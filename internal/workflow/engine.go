package workflow

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	"example.com/ghl-telnyx-integration/internal/domain"
)

type State string

const (
	StatePending       State = "pending"
	StateAwaitingReply State = "awaiting_reply"
	StateCompleted     State = "completed"
	StateSuppressed    State = "suppressed"
)

type CommandType string

const (
	CommandSendSMS             CommandType = "send_sms"
	CommandSuppress            CommandType = "suppress"
	CommandArchiveConversation CommandType = "archive_conversation"
	CommandCRMAction           CommandType = "crm_action"
	CommandCreateTask          CommandType = "create_task"
)

type Contact struct {
	ID            string
	Phone         string
	Timezone      string
	ConsentAt     time.Time
	ConsentSource string
	Variables     map[string]string
}

type Enrollment struct {
	WorkflowKey string
	Contact     Contact
	State       State
	NextRun     time.Time
	Variant     int
	SentCount   int
}

type Command struct {
	Type   CommandType
	Body   string
	Action string
	Rate   Rate
}

func Start(definition Definition, contact Contact, now time.Time) (Enrollment, error) {
	if !definition.Enabled {
		return Enrollment{}, fmt.Errorf("workflow %q is disabled", definition.Name)
	}
	if err := definition.Validate(); err != nil {
		return Enrollment{}, err
	}
	if contact.ID == "" {
		return Enrollment{}, fmt.Errorf("contact ID is required")
	}
	if len(definition.Messages) > 0 {
		if contact.Phone == "" {
			return Enrollment{}, fmt.Errorf("contact phone is required")
		}
		if contact.ConsentAt.IsZero() || strings.TrimSpace(contact.ConsentSource) == "" {
			return Enrollment{}, fmt.Errorf("documented SMS consent is required")
		}
	}

	next := now
	if definition.InitialDelay > 0 {
		next = now.Add(definition.InitialDelay)
	}
	return Enrollment{
		WorkflowKey: definition.Key,
		Contact:     contact,
		State:       StatePending,
		NextRun:     next,
		Variant:     deterministicVariant(definition, contact.ID),
	}, nil
}

func Advance(definition Definition, enrollment *Enrollment, now time.Time) ([]Command, error) {
	if enrollment == nil {
		return nil, fmt.Errorf("enrollment is required")
	}
	if enrollment.WorkflowKey != definition.Key {
		return nil, fmt.Errorf("workflow definition does not match enrollment")
	}
	if enrollment.State == StateCompleted || enrollment.State == StateSuppressed || enrollment.NextRun.IsZero() || now.Before(enrollment.NextRun) {
		return nil, nil
	}

	if definition.Kind == KindManualFollow {
		enrollment.State = StateCompleted
		enrollment.NextRun = time.Time{}
		return []Command{
			{Type: CommandCreateTask, Body: definition.TaskBody},
			{Type: CommandCRMAction, Action: definition.CRMAction},
		}, nil
	}
	if enrollment.SentCount >= definition.MaxOccurrences {
		enrollment.State = StateCompleted
		enrollment.NextRun = time.Time{}
		return nil, nil
	}

	localNow := now
	if enrollment.Contact.Timezone != "" {
		location, err := time.LoadLocation(enrollment.Contact.Timezone)
		if err != nil {
			return nil, fmt.Errorf("invalid contact timezone: %w", err)
		}
		localNow = now.In(location)
	}
	if next, allowed := nextAllowed(definition.Window, localNow); !allowed {
		enrollment.NextRun = next
		return nil, nil
	}
	if enrollment.Variant < 0 || enrollment.Variant >= len(definition.Messages) {
		return nil, fmt.Errorf("message variant %d is out of range", enrollment.Variant)
	}
	body, err := Render(definition.Messages[enrollment.Variant], enrollment.Contact.Variables)
	if err != nil {
		return nil, err
	}
	enrollment.SentCount++
	enrollment.State = StateAwaitingReply
	if definition.RepeatEvery > 0 {
		enrollment.NextRun = now.Add(definition.RepeatEvery)
	} else {
		enrollment.NextRun = time.Time{}
	}
	rate := definition.Rate
	if rate.BatchSize <= 0 || rate.Per <= 0 {
		rate = Rate{BatchSize: 1, Per: time.Second}
	}
	return []Command{{Type: CommandSendSMS, Body: body, Rate: rate}}, nil
}

func Reply(definition Definition, enrollment *Enrollment, text string) []Command {
	if enrollment == nil || enrollment.State == StateCompleted || enrollment.State == StateSuppressed {
		return nil
	}
	enrollment.NextRun = time.Time{}
	if domain.IsOptOut(text) {
		enrollment.State = StateSuppressed
		return []Command{{Type: CommandSuppress}}
	}
	if definition.ArchiveNegative && classifyReply(text) == "negative" {
		enrollment.State = StateCompleted
		return []Command{{Type: CommandCRMAction, Action: "not_interested"}, {Type: CommandArchiveConversation}}
	}

	enrollment.State = StateCompleted
	if definition.CRMAction != "" {
		return []Command{{Type: CommandCRMAction, Action: definition.CRMAction}}
	}
	return nil
}

func Render(template string, variables map[string]string) (string, error) {
	body := template
	for key, value := range variables {
		body = strings.ReplaceAll(body, "{{"+key+"}}", value)
	}
	if start := strings.Index(body, "{{"); start >= 0 {
		end := strings.Index(body[start:], "}}")
		if end >= 0 {
			return "", fmt.Errorf("missing template variable %s", body[start:start+end+2])
		}
		return "", fmt.Errorf("malformed template variable")
	}
	return body, nil
}

func deterministicVariant(definition Definition, contactID string) int {
	if len(definition.Messages) <= 1 {
		return 0
	}
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(definition.Key + ":" + contactID))
	return int(hash.Sum32() % uint32(len(definition.Messages)))
}

func nextAllowed(window SendWindow, now time.Time) (time.Time, bool) {
	minute := now.Hour()*60 + now.Minute()
	if minute >= window.StartMinute && minute < window.EndMinute {
		return now, true
	}
	if minute < window.StartMinute {
		return time.Date(now.Year(), now.Month(), now.Day(), window.StartMinute/60, window.StartMinute%60, 0, 0, now.Location()), false
	}
	tomorrow := now.AddDate(0, 0, 1)
	return time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), window.StartMinute/60, window.StartMinute%60, 0, 0, now.Location()), false
}

func classifyReply(text string) string {
	normalized := strings.ToLower(strings.Join(strings.Fields(text), " "))
	for _, phrase := range []string{"not interested", "do not contact", "don't contact", "wrong number", "already sold", "no thanks", "no thank you"} {
		if strings.Contains(normalized, phrase) {
			return "negative"
		}
	}
	for _, word := range strings.Fields(normalized) {
		switch strings.Trim(word, ".,!?:;") {
		case "no", "never":
			return "negative"
		case "yes", "sure", "interested":
			return "positive"
		}
	}
	return "unknown"
}
