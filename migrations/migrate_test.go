package migrations

import (
	"reflect"
	"testing"
)

func TestMigrationFilesAreSorted(t *testing.T) {
	files, err := migrationFiles()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"001_init.sql", "002_workflows.sql", "003_reliability.sql", "004_admin_oauth_crm.sql", "005_delivery_sync.sql", "006_inbound_delivery_dead_letter.sql"}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files=%v want=%v", files, want)
	}
}
