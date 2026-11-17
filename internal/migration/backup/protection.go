package backup

import "errors"

var errProtection = errors.New("BACKUP_PROTECTION_REJECTED")

type privateGrant struct {
	SID         string
	Kind, Flags uint8
	Mask        uint32
}

// Deliberately narrow policy, not a general Windows effective-access evaluator.
// Unsupported deny/object/callback ACEs fail closed. Machine administrators
// and the operator can still take ownership; local host trust remains required.
func privateACL(owner, current string, grants []privateGrant) bool {
	if owner == "" || owner != current || len(grants) == 0 || len(grants) > 64 {
		return false
	}
	ownerAccess := false
	for _, g := range grants {
		if g.Kind != 0 || g.Flags & ^uint8(0x13) != 0 || (g.SID != current && g.SID != "S-1-5-18") || g.Mask == 0 {
			return false
		}
		if g.SID == current && g.Mask&0x1f01ff == 0x1f01ff {
			ownerAccess = true
		}
	}
	return ownerAccess
}
