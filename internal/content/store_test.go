package content

import (
	"strings"
	"testing"
	"time"
)

func TestContentInputAndRouteBoundaries(t *testing.T) {
	valid := Command{Action: "create_activity", ClassID: "class", SectionID: "section", ActivityType: "assignment", Title: "Week 1", Position: 1}
	normalized, err := normalize(valid)
	if err != nil || normalized.ActivityType != "ASSIGNMENT" {
		t.Fatal("valid activity rejected", err)
	}
	for _, command := range []Command{
		{Action: "create_section", ClassID: "class", Title: "", Position: 1},
		{Action: "create_section", ClassID: "class", Title: "unsafe\nname", Position: 1},
		{Action: "create_activity", ClassID: "class", SectionID: "section", ActivityType: "PLUGIN", Title: "Unknown", Position: 1},
		{Action: "update_activity", ClassID: "class", ActivityID: "activity", Title: "Update", Position: 1, Version: 0},
	} {
		if _, err = normalize(command); err != ErrInvalid {
			t.Fatal("invalid content command accepted", command, err)
		}
	}
	for _, path := range []string{"/v3/classes/id/content", "/v3/classes/id/sections", "/v3/classes/id/activities/id"} {
		if !OwnsPath(path) {
			t.Fatal("content route not owned", path)
		}
	}
	if OwnsPath("/v3/classes") || OwnsPath("/v3/files/id") {
		t.Fatal("foreign route captured")
	}
}

func TestSafeCanonicalContentAndSchedule(t *testing.T) {
	canonical, digest, err := canonicalize("Hello <script>alert(1)</script>\r\nnext")
	if err != nil || strings.Contains(canonical, "<script>") || !strings.Contains(canonical, "&lt;script&gt;") || !strings.Contains(canonical, "<br>") || len(digest) != 64 {
		t.Fatal("unsafe canonical content", canonical, err)
	}
	if _, _, err = canonicalize("\x00unsafe"); err != ErrInvalid {
		t.Fatal("control byte accepted", err)
	}
	available := time.Date(2026, 9, 12, 8, 0, 0, 0, time.UTC)
	due := available.Add(time.Hour)
	cutoff := due.Add(time.Hour)
	valid := Command{Action: "create_revision", ClassID: "class", ActivityID: "activity", RawBody: "Safe", Timezone: "Asia/Jakarta", AvailableFrom: &available, DueAt: &due, CutoffAt: &cutoff}
	if _, err = normalize(valid); err != nil {
		t.Fatal("valid revision rejected", err)
	}
	invalid := valid
	invalid.Timezone = "Not/A_Zone"
	if _, err = normalize(invalid); err != ErrInvalid {
		t.Fatal("unknown timezone accepted", err)
	}
	invalid = valid
	invalid.CutoffAt, invalid.DueAt = &available, &cutoff
	if _, err = normalize(invalid); err != ErrInvalid {
		t.Fatal("reversed schedule accepted", err)
	}
	for _, path := range []string{
		"/v3/classes/id/activities/activity/revisions",
		"/v3/classes/id/activities/activity/revisions/revision",
		"/v3/classes/id/activities/activity/revisions/revision/publish",
	} {
		if !OwnsPath(path) {
			t.Fatal("revision route not owned", path)
		}
	}
}

func TestAssignmentRevisionContractBoundary(t *testing.T) {
	valid := &AssignmentRuleInput{SubmissionMode: "text_and_files", MaxAttempts: 3, MaxFiles: 2, MaxFileBytes: 2 << 20, LatePolicy: "reject_after_cutoff"}
	normalized, err := normalizeAssignment(valid)
	if err != nil || normalized.SubmissionMode != "TEXT_AND_FILES" || normalized.LatePolicy != "REJECT_AFTER_CUTOFF" {
		t.Fatal("valid assignment contract rejected", err, normalized)
	}
	invalid := []*AssignmentRuleInput{
		{SubmissionMode: "TEXT", MaxAttempts: 1, MaxFiles: 1, LatePolicy: "REJECT_AFTER_CUTOFF"},
		{SubmissionMode: "FILES", MaxAttempts: 0, MaxFiles: 1, MaxFileBytes: 1024, LatePolicy: "REJECT_AFTER_CUTOFF"},
		{SubmissionMode: "FILES", MaxAttempts: 1, MaxFiles: 21, MaxFileBytes: 1024, LatePolicy: "REJECT_AFTER_CUTOFF"},
		{SubmissionMode: "FILES", MaxAttempts: 1, MaxFiles: 1, MaxFileBytes: (2 << 20) + 1, LatePolicy: "REJECT_AFTER_CUTOFF"},
		{SubmissionMode: "TEXT", MaxAttempts: 1, LatePolicy: "SILENTLY_ACCEPT"},
	}
	for _, value := range invalid {
		if _, err = normalizeAssignment(value); err != ErrInvalid {
			t.Fatal("invalid assignment contract accepted", value, err)
		}
	}
}
