package classes

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

func participationAction(action string) bool {
	switch action {
	case "create_group", "assign_group", "assign_mentor", "invite", "accept_invitation", "start_episode":
		return true
	default:
		return false
	}
}

func (s *Store) activeEnrollmentRole(ctx context.Context, tx pgx.Tx, classID, userID, role string) (string, error) {
	var enrollment string
	err := tx.QueryRow(ctx, `SELECT e.id FROM v3.enrollments e
 JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role=$3 AND r.revoked_at IS NULL
 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL)
 FOR UPDATE OF e,r`, classID, userID, role).Scan(&enrollment)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrForbidden
	}
	return enrollment, err
}

func (s *Store) executeParticipation(ctx context.Context, token, key, requestID string, cmd Command) (json.RawMessage, error) {
	raw, _ := json.Marshal(cmd)
	hash := identity.TokenHash(string(raw))
	var response json.RawMessage
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, err := s.actor(ctx, tx, token)
		if err != nil {
			return err
		}
		class, err := s.load(ctx, tx, cmd.ClassID)
		if err != nil {
			return err
		}
		if class.State == "ARCHIVED" || class.State == "CLOSED" {
			return ErrConflict
		}
		if cmd.Action == "accept_invitation" {
			var target string
			err = tx.QueryRow(ctx, "SELECT user_id FROM v3.invitations WHERE id=$1 AND class_id=$2 FOR UPDATE", cmd.InvitationID, class.ID).Scan(&target)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && target != actor.ID) {
				return ErrForbidden
			}
			if err != nil {
				return err
			}
		} else {
			if err = s.allowed(ctx, tx, actor, class, cmd); err != nil {
				return err
			}
		}
		if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+"/"+key); err != nil {
			return err
		}
		var previous string
		err = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&previous, &response)
		if err == nil {
			if previous != hash {
				return ErrConflict
			}
			return nil
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}

		var result any
		switch cmd.Action {
		case "create_group":
			groupID, e := ID()
			if e != nil {
				return e
			}
			name := strings.TrimSpace(cmd.Name)
			if _, err = tx.Exec(ctx, "INSERT INTO v3.class_groups(id,class_id,name,created_by) VALUES($1,$2,$3,$4)", groupID, class.ID, name, actor.ID); err != nil {
				return mapConstraint(err)
			}
			result = map[string]string{"id": groupID, "class_id": class.ID, "name": name}

		case "assign_group":
			var group string
			if err = tx.QueryRow(ctx, "SELECT id FROM v3.class_groups WHERE id=$1 AND class_id=$2 FOR SHARE", cmd.GroupID, class.ID).Scan(&group); errors.Is(err, pgx.ErrNoRows) {
				return ErrForbidden
			} else if err != nil {
				return err
			}
			enrollment, e := s.activeEnrollmentRole(ctx, tx, class.ID, cmd.UserID, "student")
			if e != nil {
				return e
			}
			var priorID, priorGroup string
			err = tx.QueryRow(ctx, "SELECT id,group_id FROM v3.group_memberships WHERE enrollment_id=$1 AND ended_at IS NULL FOR UPDATE", enrollment).Scan(&priorID, &priorGroup)
			if err == nil && priorGroup == group {
				return ErrConflict
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if priorID != "" {
				if _, err = tx.Exec(ctx, "UPDATE v3.group_memberships SET ended_at=clock_timestamp() WHERE id=$1 AND ended_at IS NULL", priorID); err != nil {
					return err
				}
			}
			membershipID, e := ID()
			if e != nil {
				return e
			}
			var supersedes any
			if priorID != "" {
				supersedes = priorID
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.group_memberships(id,class_id,group_id,enrollment_id,assigned_by,supersedes_id) VALUES($1,$2,$3,$4,$5,$6)", membershipID, class.ID, group, enrollment, actor.ID, supersedes); err != nil {
				return mapConstraint(err)
			}
			result = map[string]string{"id": membershipID, "class_id": class.ID, "group_id": group, "enrollment_id": enrollment}

		case "assign_mentor":
			student, e := s.activeEnrollmentRole(ctx, tx, class.ID, cmd.UserID, "student")
			if e != nil {
				return e
			}
			mentor, e := s.activeEnrollmentRole(ctx, tx, class.ID, cmd.MentorID, "teacher")
			if e != nil {
				return e
			}
			var priorID, priorMentor string
			err = tx.QueryRow(ctx, "SELECT id,mentor_enrollment_id FROM v3.mentor_assignments WHERE student_enrollment_id=$1 AND ended_at IS NULL FOR UPDATE", student).Scan(&priorID, &priorMentor)
			if err == nil && priorMentor == mentor {
				return ErrConflict
			}
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if priorID != "" {
				if _, err = tx.Exec(ctx, "UPDATE v3.mentor_assignments SET ended_at=clock_timestamp() WHERE id=$1 AND ended_at IS NULL", priorID); err != nil {
					return err
				}
			}
			assignmentID, e := ID()
			if e != nil {
				return e
			}
			var supersedes any
			if priorID != "" {
				supersedes = priorID
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.mentor_assignments(id,class_id,student_enrollment_id,mentor_enrollment_id,assigned_by,supersedes_id) VALUES($1,$2,$3,$4,$5,$6)", assignmentID, class.ID, student, mentor, actor.ID, supersedes); err != nil {
				return mapConstraint(err)
			}
			result = map[string]string{"id": assignmentID, "class_id": class.ID, "student_enrollment_id": student, "mentor_enrollment_id": mentor}

		case "invite":
			var active bool
			err = tx.QueryRow(ctx, `SELECT u.active AND l.active AND l.valid_until>clock_timestamp()
 FROM v3.users u JOIN v3identity.links l ON l.user_id=u.id WHERE u.id=$1`, cmd.UserID).Scan(&active)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrInvalid
			}
			if err != nil {
				return err
			}
			if !active {
				return ErrConflict
			}
			var exists bool
			if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role=$3 AND r.revoked_at IS NULL)`, class.ID, cmd.UserID, cmd.Role).Scan(&exists); err != nil {
				return err
			}
			if exists {
				return ErrConflict
			}
			invitationID, e := ID()
			if e != nil {
				return e
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.invitations(id,class_id,user_id,role,invited_by) VALUES($1,$2,$3,$4,$5)", invitationID, class.ID, cmd.UserID, cmd.Role, actor.ID); err != nil {
				return mapConstraint(err)
			}
			result = map[string]string{"id": invitationID, "class_id": class.ID, "user_id": cmd.UserID, "role": cmd.Role, "status": "INVITED"}

		case "accept_invitation":
			var target, role, status string
			if err = tx.QueryRow(ctx, "SELECT user_id,role,status FROM v3.invitations WHERE id=$1 AND class_id=$2 FOR UPDATE", cmd.InvitationID, class.ID).Scan(&target, &role, &status); err != nil {
				return err
			}
			if target != actor.ID {
				return ErrForbidden
			}
			if status != "INVITED" {
				return ErrConflict
			}
			enrollment, episode, e := s.activateFromInvitation(ctx, tx, actor.ID, class.ID)
			if e != nil {
				return e
			}
			roleID, e := ID()
			if e != nil {
				return e
			}
			var inserted string
			err = tx.QueryRow(ctx, `INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($1,$2,$3,$4)
 ON CONFLICT(enrollment_id,role) WHERE revoked_at IS NULL DO NOTHING RETURNING id`, roleID, enrollment, class.ID, role).Scan(&inserted)
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrConflict
			}
			if err != nil {
				return err
			}
			var acceptedEpisode any
			if episode != "" {
				acceptedEpisode = episode
			}
			if _, err = tx.Exec(ctx, "UPDATE v3.invitations SET status='ACCEPTED',accepted_at=clock_timestamp(),accepted_episode_id=$2 WHERE id=$1 AND status='INVITED'", cmd.InvitationID, acceptedEpisode); err != nil {
				return err
			}
			if err = s.transition(ctx, tx, actor.ID, class.ID, enrollment, "accept_invitation", "INVITED", "ACTIVE"); err != nil {
				return err
			}
			result = map[string]string{"invitation_id": cmd.InvitationID, "class_id": class.ID, "enrollment_id": enrollment, "role": role, "status": "ACCEPTED", "episode_id": episode}

		case "start_episode":
			enrollment, e := s.endedStudentEnrollment(ctx, tx, class.ID, cmd.UserID)
			if e != nil {
				return e
			}
			if e = s.validateEpisodeOrigin(ctx, tx, enrollment, class.ID, cmd.UserID, cmd.OriginEpisode, cmd.EpisodeKind); e != nil {
				return e
			}
			if err = s.closeParticipation(ctx, tx, enrollment); err != nil {
				return err
			}
			episodeID, e := ID()
			if e != nil {
				return e
			}
			if _, err = tx.Exec(ctx, "UPDATE v3.enrollments SET status='ACTIVE' WHERE id=$1 AND status='ENDED'", enrollment); err != nil {
				return err
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.episodes(id,enrollment_id,kind,origin_episode_id,recognition_policy,created_by) VALUES($1,$2,$3,$4,$5,$6)", episodeID, enrollment, cmd.EpisodeKind, cmd.OriginEpisode, cmd.Policy, actor.ID); err != nil {
				return mapConstraint(err)
			}
			roleID, e := ID()
			if e != nil {
				return e
			}
			if _, err = tx.Exec(ctx, "INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($1,$2,$3,'student')", roleID, enrollment, class.ID); err != nil {
				return mapConstraint(err)
			}
			if err = s.transition(ctx, tx, actor.ID, class.ID, enrollment, "start_episode", "ENDED", "ACTIVE"); err != nil {
				return err
			}
			result = map[string]string{"class_id": class.ID, "enrollment_id": enrollment, "episode_id": episodeID, "kind": cmd.EpisodeKind, "origin_episode_id": cmd.OriginEpisode, "recognition_policy": cmd.Policy, "status": "ACTIVE"}
		}

		if err = s.fail("domain"); err != nil {
			return err
		}
		eventID, e := ID()
		if e != nil {
			return e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", eventID, actor.ID, class.ID, cmd.Action, requestID); err != nil {
			return err
		}
		if err = s.fail("audit"); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", eventID); err != nil {
			return err
		}
		if err = s.fail("outbox"); err != nil {
			return err
		}
		response, err = json.Marshal(result)
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO v3.commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb)", actor.ID, key, hash, string(response))
		return err
	})
	return response, err
}

func (s *Store) activateFromInvitation(ctx context.Context, tx pgx.Tx, actorID, classID string) (string, string, error) {
	var enrollment, status string
	err := tx.QueryRow(ctx, "SELECT id,status FROM v3.enrollments WHERE class_id=$1 AND user_id=$2 FOR UPDATE", classID, actorID).Scan(&enrollment, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		enrollment, err = ID()
		if err != nil {
			return "", "", err
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($1,$2,$3,'ACTIVE')", enrollment, classID, actorID); err != nil {
			return "", "", err
		}
		episode, e := ID()
		if e != nil {
			return "", "", e
		}
		if _, err = tx.Exec(ctx, "INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($1,$2,$3)", episode, enrollment, actorID); err != nil {
			return "", "", err
		}
		return enrollment, episode, nil
	}
	if err != nil {
		return "", "", err
	}
	if status == "ACTIVE" {
		var suspended bool
		if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.enrollment_access WHERE enrollment_id=$1 AND suspended_at IS NOT NULL)", enrollment).Scan(&suspended); err != nil {
			return "", "", err
		}
		if suspended {
			return "", "", ErrConflict
		}
		return enrollment, "", nil
	}
	var origin string
	if err = tx.QueryRow(ctx, "SELECT id FROM v3.episodes WHERE enrollment_id=$1 ORDER BY opened_at DESC,id DESC LIMIT 1 FOR UPDATE", enrollment).Scan(&origin); err != nil {
		return "", "", err
	}
	if err = s.closeParticipation(ctx, tx, enrollment); err != nil {
		return "", "", err
	}
	episode, e := ID()
	if e != nil {
		return "", "", e
	}
	if _, err = tx.Exec(ctx, "UPDATE v3.enrollments SET status='ACTIVE' WHERE id=$1", enrollment); err != nil {
		return "", "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.episodes(id,enrollment_id,kind,origin_episode_id,recognition_policy,created_by) VALUES($1,$2,'REJOIN',$3,'SEPARATE_HISTORY',$4)", episode, enrollment, origin, actorID); err != nil {
		return "", "", err
	}
	return enrollment, episode, nil
}

func (s *Store) endedStudentEnrollment(ctx context.Context, tx pgx.Tx, classID, userID string) (string, error) {
	var enrollment, status string
	err := tx.QueryRow(ctx, `SELECT e.id,e.status FROM v3.enrollments e
 WHERE e.class_id=$1 AND e.user_id=$2 FOR UPDATE`, classID, userID).Scan(&enrollment, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if status != "ENDED" {
		return "", ErrConflict
	}
	var wasStudent bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.roles WHERE enrollment_id=$1 AND role='student')", enrollment).Scan(&wasStudent); err != nil {
		return "", err
	}
	if !wasStudent {
		return "", ErrForbidden
	}
	return enrollment, nil
}

func (s *Store) validateEpisodeOrigin(ctx context.Context, tx pgx.Tx, enrollment, classID, userID, origin, kind string) error {
	var originEnrollment, originClass, originUser string
	err := tx.QueryRow(ctx, `SELECT p.enrollment_id,e.class_id,e.user_id FROM v3.episodes p
 JOIN v3.enrollments e ON e.id=p.enrollment_id WHERE p.id=$1 FOR SHARE OF p,e`, origin).Scan(&originEnrollment, &originClass, &originUser)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && originUser != userID) {
		return ErrForbidden
	}
	if err != nil {
		return err
	}
	if kind == "RETAKE" {
		var latest string
		if originEnrollment != enrollment || originClass != classID {
			return ErrInvalid
		}
		if err = tx.QueryRow(ctx, "SELECT id FROM v3.episodes WHERE enrollment_id=$1 ORDER BY opened_at DESC,id DESC LIMIT 1", enrollment).Scan(&latest); err != nil || latest != origin {
			return ErrConflict
		}
		return nil
	}
	if originClass == classID {
		return ErrInvalid
	}
	return nil
}

func (s *Store) transition(ctx context.Context, tx pgx.Tx, actor, classID, enrollment, action, previous, next string) error {
	id, err := ID()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v3.transition_events(id,actor_id,class_id,enrollment_id,action,previous_state,next_state) VALUES($1,$2,$3,$4,$5,$6,$7)", id, actor, classID, enrollment, action, previous, next)
	return err
}

func (s *Store) closeParticipation(ctx context.Context, tx pgx.Tx, enrollment string) error {
	var installed bool
	if err := tx.QueryRow(ctx, "SELECT to_regclass('v3.group_memberships') IS NOT NULL").Scan(&installed); err != nil || !installed {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE v3.group_memberships SET ended_at=clock_timestamp() WHERE enrollment_id=$1 AND ended_at IS NULL", enrollment); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, "UPDATE v3.mentor_assignments SET ended_at=clock_timestamp() WHERE (student_enrollment_id=$1 OR mentor_enrollment_id=$1) AND ended_at IS NULL", enrollment)
	return err
}

func provenanceInstalled(ctx context.Context, tx pgx.Tx) (bool, error) {
	var installed bool
	err := tx.QueryRow(ctx, "SELECT to_regclass('v3.invitations') IS NOT NULL").Scan(&installed)
	return installed, err
}

func mapConstraint(err error) error {
	var pgerr interface{ SQLState() string }
	if errors.As(err, &pgerr) && (pgerr.SQLState() == "23505" || pgerr.SQLState() == "23514" || pgerr.SQLState() == "23503") {
		return ErrConflict
	}
	return err
}
