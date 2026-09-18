package access

import "testing"

func TestCapabilityMatrix(t *testing.T) {
	for _, a := range []string{"read", "edit", "publish"} {
		if !Allows(false, []string{"teacher"}, "DRAFT", a, "") {
			t.Fatal(a)
		}
	}
	if !Allows(false, []string{"teacher"}, "PUBLISHED", "add", "student") || Allows(false, []string{"teacher"}, "PUBLISHED", "add", "teacher") {
		t.Fatal("teacher participant capability")
	}
	for _, action := range []string{"create_group", "assign_group", "assign_mentor", "start_episode"} {
		if !Allows(false, []string{"teacher"}, "PUBLISHED", action, "student") {
			t.Fatal(action)
		}
	}
	if !Allows(false, []string{"teacher"}, "PUBLISHED", "invite", "student") || Allows(false, []string{"teacher"}, "PUBLISHED", "invite", "teacher") {
		t.Fatal("teacher invitation capability")
	}
	for _, a := range []string{"create", "archive", "close", "reopen"} {
		if Allows(false, []string{"teacher"}, "PUBLISHED", a, "") {
			t.Fatal(a)
		}
	}
	for _, state := range []string{"PUBLISHED", "CLOSED", "ARCHIVED"} {
		if !Allows(false, []string{"student"}, state, "read", "") {
			t.Fatal(state)
		}
	}
	if Allows(false, []string{"student"}, "DRAFT", "read", "") || Allows(false, []string{"mentor"}, "PUBLISHED", "edit", "") || Allows(true, nil, "PUBLISHED", "unknown", "") {
		t.Fatal("denial matrix")
	}
}

func TestFileCapabilities(t *testing.T) {
	if !AllowsFile(false, []string{"teacher"}, "DRAFT", "scan", "EVIDENCE", false) {
		t.Fatal("Class teacher lost file management capability")
	}
	if !AllowsFile(false, []string{"student"}, "PUBLISHED", "upload", "EVIDENCE", true) {
		t.Fatal("student cannot upload own evidence")
	}
	for _, tc := range []struct {
		action, purpose, state string
		owner                  bool
	}{
		{"upload", "MATERIAL", "PUBLISHED", true},
		{"metadata", "EVIDENCE", "PUBLISHED", false},
		{"grant", "MATERIAL", "DRAFT", false},
		{"scan", "EVIDENCE", "PUBLISHED", true},
	} {
		if AllowsFile(false, []string{"student"}, tc.state, tc.action, tc.purpose, tc.owner) {
			t.Fatalf("student file capability escaped policy: %+v", tc)
		}
	}
}
