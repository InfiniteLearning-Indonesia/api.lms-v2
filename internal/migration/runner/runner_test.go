package runner

import (
	"strings"
	"testing"
	"time"
)

func TestCatalogueAndGateFailClosed(t *testing.T) {
	base := Migration{Version: "one", SQL: "CREATE SCHEMA safe"}
	if ValidateCatalogue([]Migration{base, {Version: "two", SQL: "CREATE TABLE safe.x(id int)", Requires: []string{"one"}}}) != nil {
		t.Fatal("valid forward catalogue rejected")
	}
	bad := [][]Migration{
		nil,
		{{Version: "../one", SQL: base.SQL}},
		{{Version: "one", SQL: "DROP SCHEMA safe"}},
		{{Version: "one", SQL: base.SQL}, {Version: "two", SQL: "CREATE TABLE safe.x(id int)"}},
		{{Version: "one", SQL: base.SQL}, {Version: "two", SQL: "CREATE TABLE safe.x(id int)", Requires: []string{"missing"}}},
	}
	for _, items := range bad {
		if ValidateCatalogue(items) == nil {
			t.Fatal("unsafe catalogue accepted")
		}
	}
	now := time.Now().UTC()
	target := strings.Repeat("a", 64)
	good := Gate{RunID: "run-1", TargetFingerprint: target, ArchiveSHA256: strings.Repeat("b", 64), RestoreVerified: true, VerifiedAt: now}
	if good.Validate(target, now) != nil {
		t.Fatal("valid restore gate rejected")
	}
	for _, gate := range []Gate{{}, {RunID: good.RunID, TargetFingerprint: target, ArchiveSHA256: good.ArchiveSHA256, RestoreVerified: false, VerifiedAt: now}, {RunID: good.RunID, TargetFingerprint: target, ArchiveSHA256: good.ArchiveSHA256, RestoreVerified: true, VerifiedAt: now.Add(-2 * time.Hour)}} {
		if gate.Validate(target, now) == nil {
			t.Fatal("invalid restore gate accepted")
		}
	}
}
