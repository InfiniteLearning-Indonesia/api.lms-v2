package classes

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

type Invitation struct {
	ID      string `json:"id"`
	ClassID string `json:"class_id"`
	UserID  string `json:"user_id,omitempty"`
	Role    string `json:"role"`
	Status  string `json:"status"`
}

// The caller holds the Class row until commit. Both invitation entry points
// share the same pending check, including when they race with each other.
func (s *Store) createInvitation(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string, cmd Command) (Invitation, bool, error) {
	var err error
	userID := cmd.UserID
	if cmd.Action == "invite_account" {
		userID, err = s.accounts.ProvisionForClass(ctx, tx, actor, cmd.AccountEmail)
		if errors.Is(err, identity.ErrPasswordInput) {
			return Invitation{}, false, ErrInvalid
		}
		if errors.Is(err, identity.ErrConflict) {
			return Invitation{}, false, ErrConflict
		}
		if err != nil {
			return Invitation{}, false, err
		}
	} else {
		var active bool
		err = tx.QueryRow(ctx, `SELECT u.active AND l.active AND l.valid_until>clock_timestamp()
 FROM v3.users u JOIN v3identity.links l ON l.user_id=u.id WHERE u.id=$1`, userID).Scan(&active)
		if errors.Is(err, pgx.ErrNoRows) {
			return Invitation{}, false, ErrInvalid
		}
		if err != nil {
			return Invitation{}, false, err
		}
		if !active {
			return Invitation{}, false, ErrConflict
		}
	}
	var activeRole bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role=$3 AND r.revoked_at IS NULL)`, classID, userID, cmd.Role).Scan(&activeRole); err != nil {
		return Invitation{}, false, err
	}
	if activeRole {
		return Invitation{}, false, ErrConflict
	}
	result := Invitation{ClassID: classID, UserID: userID, Role: cmd.Role, Status: "INVITED"}
	err = tx.QueryRow(ctx, `SELECT id FROM v3.invitations WHERE class_id=$1 AND user_id=$2 AND role=$3 AND status='INVITED'
 ORDER BY invited_at,id LIMIT 1`, classID, userID, cmd.Role).Scan(&result.ID)
	if err == nil {
		if cmd.Action == "invite_account" {
			return result, true, nil
		}
		return Invitation{}, false, ErrConflict
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Invitation{}, false, err
	}
	result.ID, err = ID()
	if err != nil {
		return Invitation{}, false, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.invitations(id,class_id,user_id,role,invited_by) VALUES($1,$2,$3,$4,$5)", result.ID, classID, userID, cmd.Role, actor.ID); err != nil {
		return Invitation{}, false, mapConstraint(err)
	}
	return result, false, nil
}

func (s *Store) pendingInvitations(ctx context.Context, token string) ([]Invitation, error) {
	result := []Invitation{}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.actor(ctx, tx, token)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT i.id,i.class_id,i.role,i.status FROM v3.invitations i JOIN v3.classes c ON c.id=i.class_id
 LEFT JOIN v3.class_access x ON x.class_id=i.class_id
 WHERE i.user_id=$1 AND i.status='INVITED' AND c.state<>'ARCHIVED' AND x.closed_at IS NULL
 ORDER BY i.invited_at DESC,i.id DESC LIMIT 100`, actor.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var invite Invitation
			if err = rows.Scan(&invite.ID, &invite.ClassID, &invite.Role, &invite.Status); err != nil {
				return err
			}
			result = append(result, invite)
		}
		return rows.Err()
	})
	return result, err
}

type classMembership struct {
	Class
	Roles []string `json:"roles"`
}

// memberClasses lists the Classes readable by the current session. Site Admin
// can manage every Class. Teacher/Student visibility stays Class-scoped.
func (s *Store) memberClasses(ctx context.Context, token string) ([]classMembership, error) {
	result := []classMembership{}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.actor(ctx, tx, token)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT c.id,c.name,c.program_label,c.cohort_label,
 CASE WHEN c.state='PUBLISHED' AND x.closed_at IS NOT NULL THEN 'CLOSED' ELSE c.state END,c.version,m.roles
 FROM v3.classes c LEFT JOIN v3.class_access x ON x.class_id=c.id CROSS JOIN LATERAL (SELECT ARRAY(
 SELECT DISTINCT r.role FROM v3.enrollments e JOIN v3.roles r ON (r.enrollment_id,r.class_id)=(e.id,e.class_id)
 LEFT JOIN v3.enrollment_access ea ON ea.enrollment_id=e.id
 WHERE e.class_id=c.id AND e.user_id=$1 AND e.status='ACTIVE' AND ea.suspended_at IS NULL
 AND r.revoked_at IS NULL AND (r.role='teacher' OR (r.role='student' AND c.state<>'DRAFT')) ORDER BY r.role) AS roles) m
 WHERE $2::bool OR cardinality(m.roles)>0
 ORDER BY c.name,c.id LIMIT 250`, actor.ID, actor.Admin)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var c classMembership
			if err = rows.Scan(&c.ID, &c.Name, &c.ProgramLabel, &c.CohortLabel, &c.State, &c.Version, &c.Roles); err != nil {
				return err
			}
			result = append(result, c)
		}
		return rows.Err()
	})
	return result, err
}
