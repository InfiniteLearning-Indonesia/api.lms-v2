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
