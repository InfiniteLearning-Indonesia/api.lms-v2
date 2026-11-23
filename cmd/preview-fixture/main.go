// preview-fixture supplies the fixed schema and reads the owner-only synthetic
// mailbox. It has no HTTP surface and accepts no SQL or database URL.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"lmsv3/internal/identity"
	"lmsv3/internal/labdb"
	"lmsv3/internal/localpreview"
)

func main() {
	if len(os.Args) != 2 || os.Getenv("LMS_MODE") != "v3-local-preview" {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "schema":
		for _, m := range labdb.Catalogue() {
			fmt.Println(m.SQL)
			fmt.Println(";")
		}
		fmt.Println(identity.PasswordSchema)
		fmt.Println(identity.GoogleSchema)
		fmt.Print(grants)
	case "mailbox":
		if readMailbox() != nil {
			os.Exit(1)
		}
	default:
		os.Exit(2)
	}
}

func readMailbox() error {
	c, err := localpreview.Load(os.Getenv)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pool, err := localpreview.Open(ctx, c)
	if err != nil {
		return err
	}
	defer pool.Close()
	passwords, err := identity.NewPasswords(&identity.Lifecycle{Pool: pool}, c.DeliveryKey)
	if err != nil {
		return err
	}
	rows, err := pool.Query(ctx, `SELECT d.id,t.token_hash,a.user_id,t.purpose,d.payload,a.email,t.expires_at
 FROM v3password.deliveries d JOIN v3password.tokens t USING(token_hash)
 JOIN v3password.accounts a ON a.user_id=t.user_id JOIN v3.users u ON u.id=a.user_id
 JOIN v3identity.links l ON l.user_id=a.user_id
 WHERE t.consumed_at IS NULL AND t.expires_at>clock_timestamp() AND a.suspended_at IS NULL
 AND l.issuer=$1 AND ((t.purpose='invite' AND a.activated_at IS NULL AND NOT u.active AND NOT l.active) OR
  (t.purpose='reset' AND a.activated_at IS NOT NULL AND u.active AND l.active))
 ORDER BY d.created_at DESC,d.id DESC LIMIT 50`, identity.PasswordIssuer)
	if err != nil {
		return err
	}
	defer rows.Close()
	type message struct {
		Email   string    `json:"email"`
		Purpose string    `json:"purpose"`
		Token   string    `json:"token"`
		Expires time.Time `json:"expires_at"`
	}
	messages := []message{}
	for rows.Next() {
		var id, hash, user string
		var payload []byte
		var m message
		if err = rows.Scan(&id, &hash, &user, &m.Purpose, &payload, &m.Email, &m.Expires); err != nil {
			return err
		}
		m.Token, err = passwords.OpenDelivery(id, hash, user, m.Purpose, payload)
		if err != nil {
			return err
		}
		messages = append(messages, m)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(messages)
}

const grants = `
GRANT USAGE ON SCHEMA v3,v3identity,v3password,v3google,preview_guard TO preview_runtime;
GRANT EXECUTE ON FUNCTION pg_catalog.pg_control_system() TO preview_runtime;
GRANT SELECT ON preview_guard.target TO preview_runtime;
GRANT SELECT ON ALL TABLES IN SCHEMA v3,v3identity,v3password,v3google TO preview_runtime;
GRANT INSERT ON v3.users,v3.audit,v3.outbox,v3.classes,v3.class_access,v3.class_groups,v3.commands,v3.enrollment_access,v3.enrollments,
 v3.episodes,v3.group_memberships,v3.invitations,v3.mentor_assignments,v3.roles,v3.transition_events,
 v3.file_assets,v3.file_access_events,v3.file_commands,v3.file_download_grants,v3.assignment_revision_rules,
 v3.content_activities,v3.content_commands,v3.content_revision_sources,v3.content_revisions,v3.content_sections,
 v3.submission_aggregates,v3.submission_attempts,v3.submission_commands,v3.submission_receipts,v3.submission_revision_files,v3.submission_revisions,
 v3.grade_commands,v3.grade_current,v3.grade_definition_commands,v3.grade_definition_publications,v3.grade_definition_versions,
 v3.grade_import_confirms,v3.grade_import_previews,v3.grade_items,v3.grade_releases,v3.grade_revisions,v3.grade_scales,
 v3.legacy_grade_observations,v3.legacy_grade_reconciliation_runs,v3.rubric_criteria,
 v3.activity_completion_outcomes,v3.completion_commands,v3.completion_correction_commands,v3.completion_corrections,
 v3.completion_rule_versions,v3.credential_commands,v3.credential_events,v3.credential_records,v3.learning_evidence,
 v3.legacy_completion_observations,v3.legacy_completion_runs,v3.transcript_grade_items,v3.transcript_releases,
 v3.class_meetings,v3.meeting_commands,v3.meeting_exceptions,v3.logbook_entries,v3.logbook_entry_commands,v3.logbook_entry_events,
 v3.logbook_entry_revisions,v3.logbook_period_commands,v3.logbook_period_publications,v3.logbook_period_sets,v3.logbook_periods,
 v3.logbook_review_commands,v3.logbook_revision_files,v3.notification_commands,v3.notification_deliveries,
 v3.notification_preference_events,v3.ai_suggestion_commands,v3.ai_suggestions TO preview_runtime;
GRANT UPDATE(active) ON v3.users TO preview_runtime;
GRANT UPDATE(name,program_label,cohort_label,state,version) ON v3.classes TO preview_runtime;
GRANT UPDATE(closed_at) ON v3.class_access TO preview_runtime;
GRANT UPDATE(status) ON v3.enrollments TO preview_runtime;
GRANT UPDATE(suspended_at) ON v3.enrollment_access TO preview_runtime;
GRANT UPDATE(ended_at) ON v3.episodes,v3.group_memberships,v3.mentor_assignments TO preview_runtime;
GRANT UPDATE(revoked_at) ON v3.roles TO preview_runtime;
GRANT UPDATE(id) ON v3.class_groups TO preview_runtime;
GRANT UPDATE(status,accepted_at,accepted_episode_id) ON v3.invitations TO preview_runtime;
GRANT UPDATE(state,scan_status,scan_code,available_at) ON v3.file_assets TO preview_runtime;
GRANT UPDATE(title,position,version,updated_at) ON v3.content_sections TO preview_runtime;
GRANT UPDATE(title,section_id,position,version,updated_at) ON v3.content_activities TO preview_runtime;
GRANT UPDATE(canonical_body,timezone,available_from,due_at,cutoff_at,state,version,updated_at) ON v3.content_revisions TO preview_runtime;
GRANT UPDATE(raw_body,raw_sha256,updated_at) ON v3.content_revision_sources TO preview_runtime;
GRANT UPDATE(submission_mode,max_attempts,max_files,max_file_bytes,late_policy,version,updated_at) ON v3.assignment_revision_rules TO preview_runtime;
GRANT UPDATE(current_attempt_no,version,updated_at) ON v3.submission_aggregates TO preview_runtime;
GRANT UPDATE(state,version,locked_at,returned_at,returned_by,updated_at) ON v3.submission_attempts TO preview_runtime;
GRANT UPDATE(revision_id,version,updated_at) ON v3.grade_current TO preview_runtime;
GRANT INSERT ON v3identity.links,v3identity.site_roles,v3identity.role_events,v3identity.sessions TO preview_runtime;
GRANT UPDATE(active) ON v3identity.links TO preview_runtime;
GRANT UPDATE(admin,version) ON v3identity.site_roles TO preview_runtime;
GRANT UPDATE(revoked_at) ON v3identity.sessions TO preview_runtime;
GRANT INSERT ON ALL TABLES IN SCHEMA v3password,v3google TO preview_runtime;
GRANT UPDATE(activated_at,suspended_at) ON v3password.accounts TO preview_runtime;
GRANT UPDATE(salt,password_hash,updated_at) ON v3password.credentials TO preview_runtime;
GRANT UPDATE(consumed_at) ON v3password.tokens TO preview_runtime;
GRANT UPDATE(window_start,attempts) ON v3password.limits TO preview_runtime;
GRANT UPDATE(consumed_at) ON v3google.flows TO preview_runtime;
GRANT UPDATE(window_start,attempts) ON v3google.start_budget TO preview_runtime;
`
