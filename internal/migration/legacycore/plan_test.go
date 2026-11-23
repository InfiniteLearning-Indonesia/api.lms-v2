//go:build core_rehearsal

package legacycore

import "testing"

func TestCoreRolesFollowClassEvidence(t *testing.T) {
	program := "00000000-0000-0000-0000-000000000001"
	batch := "00000000-0000-0000-0000-000000000002"
	firstClass := "00000000-0000-0000-0000-000000000003"
	secondClass := "00000000-0000-0000-0000-000000000004"
	dual := "00000000-0000-0000-0000-000000000005"
	admin := "00000000-0000-0000-0000-000000000006"
	rows := map[string][]row{
		"programs": {{"id": program, "name": "Fixture"}},
		"batches":  {{"id": batch, "name": "Batch"}},
		"classes": {
			{"id": firstClass, "programId": program, "batchId": batch, "mentorId": dual},
			{"id": secondClass, "programId": program, "batchId": batch},
		},
		"users": {
			{"id": dual, "status": "active", "roles": `["mentor","student"]`},
			{"id": admin, "status": "suspended", "roles": `["admin"]`},
		},
		"enrollments": {
			{"id": "00000000-0000-0000-0000-000000000007", "studentId": dual, "classId": secondClass, "createdAt": "fixture"},
			{"id": "00000000-0000-0000-0000-000000000008", "studentId": admin, "classId": secondClass, "createdAt": "fixture"},
			{"id": "00000000-0000-0000-0000-000000000009", "studentId": dual, "classId": firstClass, "createdAt": "fixture"},
		},
	}
	plan, err := mapCore(rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Counts.Held != 2 || plan.Counts.TeacherRoles != 1 || plan.Counts.StudentRoles != 1 ||
		plan.Counts.Participations != 2 || plan.Counts.Suspended != 1 || plan.Counts.Mentorless != 1 {
		t.Fatal("incorrect accounting or access during rehearsal")
	}
	for _, p := range plan.Participations {
		expected := "student"
		if p.ClassID == id("class", firstClass) {
			expected = "teacher"
		}
		if p.UserID != id("user", dual) || len(p.Roles) != 1 || p.Roles[0] != expected {
			t.Fatal("class roles leaked or a held record gained a role")
		}
	}
	if plan.Classes[0].Name == plan.Classes[1].Name {
		t.Fatal("duplicate class labels need stable disambiguation")
	}
}

func TestCoreSnapshotPinRejectsAlteredInput(t *testing.T) {
	if _, err := Build(make([]byte, SnapshotSize)); err != ErrSource {
		t.Fatal("same-length substituted archive was not rejected")
	}
}
