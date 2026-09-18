// Package access is the explicit Class capability matrix, not identity ownership.
package access

func Allows(admin bool, roles []string, state, action, targetRole string) bool {
	known := map[string]bool{"create": true, "read": true, "edit": true, "publish": true, "archive": true, "close": true, "reopen": true, "add": true, "end": true, "suspend": true, "reactivate": true, "create_group": true, "assign_group": true, "assign_mentor": true, "invite": true, "start_episode": true}
	if !known[action] {
		return false
	}
	if admin {
		return true
	}
	for _, role := range roles {
		if role == "teacher" {
			switch action {
			case "read", "edit", "publish":
				return true
			case "add", "invite":
				return targetRole == "student"
			case "end", "suspend", "reactivate", "create_group", "assign_group", "assign_mentor", "start_episode":
				return true
			}
		}
		if role == "student" && action == "read" && state != "DRAFT" {
			return true
		}
	}
	return false
}

// AllowsFile evaluates file capabilities in Class context. Evidence is private
// to its owner and Class teachers; published material is readable by active
// students. Enrollment remains separate from capability assignment.
func AllowsFile(admin bool, roles []string, state, action, purpose string, owner bool) bool {
	if admin {
		return true
	}
	teacher, student := false, false
	for _, role := range roles {
		teacher = teacher || role == "teacher"
		student = student || role == "student"
	}
	if teacher {
		return action == "upload" || action == "metadata" || action == "scan" || action == "grant" || action == "download"
	}
	if !student {
		return false
	}
	switch action {
	case "upload":
		return purpose == "EVIDENCE" && owner
	case "metadata", "grant", "download":
		return (purpose == "EVIDENCE" && owner) || (purpose == "MATERIAL" && state != "DRAFT")
	default:
		return false
	}
}
