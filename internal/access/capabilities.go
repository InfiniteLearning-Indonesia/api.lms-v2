// Package access is the explicit Class capability matrix, not identity ownership.
package access

func Allows(admin bool, roles []string, state, action, targetRole string) bool {
	known := map[string]bool{"create": true, "read": true, "edit": true, "publish": true, "archive": true, "close": true, "reopen": true, "add": true, "end": true, "suspend": true, "reactivate": true}
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
			case "add":
				return targetRole == "student"
			case "end", "suspend", "reactivate":
				return true
			}
		}
		if role == "student" && action == "read" && state != "DRAFT" {
			return true
		}
	}
	return false
}
