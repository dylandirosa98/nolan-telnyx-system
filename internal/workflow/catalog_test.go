package workflow

import (
	"testing"
	"time"
)

func TestLegacyCatalogContainsEveryInspectedWorkflow(t *testing.T) {
	defs := LegacyCatalog()
	if len(defs) != 17 {
		t.Fatalf("got %d workflows, want 17", len(defs))
	}

	seen := make(map[string]bool, len(defs))
	published := 0
	drafts := 0
	for _, def := range defs {
		if def.Key == "" || def.Name == "" {
			t.Fatalf("workflow has empty identity: %#v", def)
		}
		if seen[def.Key] {
			t.Fatalf("duplicate key %q", def.Key)
		}
		seen[def.Key] = true
		switch def.LegacyStatus {
		case StatusPublished:
			published++
		case StatusDraft:
			drafts++
		default:
			t.Fatalf("%s has invalid legacy status %q", def.Name, def.LegacyStatus)
		}
		if err := def.Validate(); err != nil {
			t.Fatalf("%s: %v", def.Name, err)
		}
	}
	if published != 12 || drafts != 5 {
		t.Fatalf("published=%d drafts=%d, want 12 and 5", published, drafts)
	}

	for _, name := range []string{
		"1 x month Text",
		"1x 3months text",
		"1x week follow up",
		"6 day offer followup",
		"Copy - Copy - SMS Blast",
		"Copy - Manual Follow Up 6 Month",
		"Copy - SMS Blast",
		"Every 2 Day check in",
		"Manual Follow Up 1 Month",
		"Manual Follow Up 1 Week",
		"Manual Follow Up 2 Month",
		"Manual Follow Up 2 Week",
		"Manual Follow Up 3 Month",
		"SMS Blast",
		"SMS Blast Full Address",
		"Upper Darby- SMS Blast",
		"smsv2",
	} {
		if _, ok := FindByName(defs, name); !ok {
			t.Errorf("missing workflow %q", name)
		}
	}
}

func TestLegacyCatalogPreservesObservedTimingAndRateLimits(t *testing.T) {
	defs := LegacyCatalog()
	tests := []struct {
		name         string
		initialDelay time.Duration
		repeat       time.Duration
		batchSize    int
		per          time.Duration
	}{
		{name: "1 x month Text", initialDelay: 30 * 24 * time.Hour, repeat: 30 * 24 * time.Hour},
		{name: "1x 3months text", initialDelay: 90 * 24 * time.Hour, repeat: 90 * 24 * time.Hour},
		{name: "1x week follow up", initialDelay: 7 * 24 * time.Hour, repeat: 7 * 24 * time.Hour},
		{name: "6 day offer followup", initialDelay: 6 * 24 * time.Hour, repeat: 6 * 24 * time.Hour},
		{name: "Every 2 Day check in", initialDelay: 2 * 24 * time.Hour, repeat: 2 * 24 * time.Hour},
		{name: "SMS Blast", batchSize: 1, per: 135 * time.Second},
		{name: "Copy - SMS Blast", batchSize: 6, per: time.Minute},
		{name: "SMS Blast Full Address", batchSize: 7, per: time.Minute},
		{name: "Upper Darby- SMS Blast", batchSize: 10, per: time.Minute},
	}
	for _, tc := range tests {
		def, ok := FindByName(defs, tc.name)
		if !ok {
			t.Fatalf("missing %q", tc.name)
		}
		if def.InitialDelay != tc.initialDelay || def.RepeatEvery != tc.repeat {
			t.Errorf("%s delays=(%s,%s), want (%s,%s)", tc.name, def.InitialDelay, def.RepeatEvery, tc.initialDelay, tc.repeat)
		}
		if tc.batchSize != 0 && (def.Rate.BatchSize != tc.batchSize || def.Rate.Per != tc.per) {
			t.Errorf("%s rate=%+v, want batch=%d per=%s", tc.name, def.Rate, tc.batchSize, tc.per)
		}
	}
}

func TestLegacyCatalogDoesNotActivateLegacyWorkflowsByDefault(t *testing.T) {
	for _, def := range LegacyCatalog() {
		if def.Enabled {
			t.Errorf("%s is enabled before destination mapping and compliance approval", def.Name)
		}
	}
}

func TestEnabledCatalogOnlyActivatesExplicitKeys(t *testing.T) {
	definitions, err := EnabledCatalog([]string{"monthly-text", "sms-blast-full-address"})
	if err != nil {
		t.Fatal(err)
	}
	if len(definitions) != 17 {
		t.Fatalf("got %d definitions", len(definitions))
	}
	if !definitions["monthly-text"].Enabled || !definitions["sms-blast-full-address"].Enabled {
		t.Fatalf("explicit workflows were not enabled")
	}
	if definitions["weekly-follow-up"].Enabled {
		t.Fatal("workflow was enabled without explicit configuration")
	}
	if _, err := EnabledCatalog([]string{"does-not-exist"}); err == nil {
		t.Fatal("unknown workflow key should fail configuration")
	}
	if _, err := EnabledCatalog([]string{"sms-blast"}); err == nil {
		t.Fatal("draft workflow should not be enableable")
	}
}
