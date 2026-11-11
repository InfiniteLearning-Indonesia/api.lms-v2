//go:build qaintegration

package gradebook

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func importCase(t *testing.T, suffix string) (*gradeFixture, gradeContext, Definition, ImportEntry) {
	t.Helper()
	f := openGradeFixture(t, "m09-cp03")
	unique, _ := token(4)
	work := f.submission(t, suffix+unique, "student", false)
	definition := createPublishedDefinition(t, f, work)
	entry := ImportEntry{AggregateID: work.aggregateID, SubmissionRevisionID: work.revisionID, ExpectedVersion: 0, ValueMilli: 81000}
	return f, work, definition, entry
}

func TestSyntheticImportLifecycle(t *testing.T) {
	f, work, definition, entry := importCase(t, "i1")
	ctx := context.Background()
	command := ImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, Entries: []ImportEntry{entry}}
	preview, replay, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "preview-i1", "request-i1", command)
	if err != nil || replay || preview.InputHash == "" || preview.DefinitionVersionID != definition.DefinitionVersionID || preview.PublicationRevision != 1 {
		t.Fatal(preview, replay, err)
	}
	previewAgain, replay, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "preview-i1", "request-i1b", command)
	if err != nil || !replay || previewAgain.ID != preview.ID {
		t.Fatal(previewAgain, replay, err)
	}
	if _, err = f.store.ReadGrade(ctx, f.tokens["teacher"], work.classID, definition.GradeItemID, work.aggregateID); !errors.Is(err, ErrNotFound) {
		t.Fatal("preview wrote grade", err)
	}
	confirm := ConfirmImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, PreviewID: preview.ID, Entries: []ImportEntry{entry}}
	result, replay, err := f.store.ConfirmImport(ctx, f.tokens["teacher"], "confirm-i1", "request-i2", confirm)
	if err != nil || replay {
		t.Fatal(string(result), replay, err)
	}
	var grades []Grade
	if json.Unmarshal(result, &grades) != nil || len(grades) != 1 || grades[0].Reason != "IMPORT_INITIAL" || grades[0].Version != 1 {
		t.Fatal(string(result))
	}
	resultAgain, replay, err := f.store.ConfirmImport(ctx, f.tokens["teacher"], "confirm-i1", "request-i3", confirm)
	if err != nil || !replay || string(resultAgain) != string(result) {
		t.Fatal(string(resultAgain), replay, err)
	}
	var history, receipts, audits, outbox int
	err = f.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM v3.grade_revisions WHERE source_preview_id=$1),
	 (SELECT count(*) FROM v3.grade_import_confirms WHERE preview_id=$1),
	 (SELECT count(*) FROM v3.audit WHERE class_id=$2 AND action LIKE 'gradebook.import.%'),
	 (SELECT count(*) FROM v3.outbox o JOIN v3.audit a ON a.id=o.event_id WHERE a.class_id=$2 AND a.action LIKE 'gradebook.import.%')`, preview.ID, work.classID).Scan(&history, &receipts, &audits, &outbox)
	if err != nil || history != 1 || receipts != 1 || audits != 2 || outbox != 2 {
		t.Fatal(history, receipts, audits, outbox, err)
	}
}

func TestSyntheticImportBoundary(t *testing.T) {
	f, work, definition, entry := importCase(t, "i2")
	ctx := context.Background()
	command := ImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, Entries: []ImportEntry{entry}}
	if _, _, err := f.store.PreviewImport(ctx, f.tokens["outsider"], "preview-out", "request-out", command); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider preview", err)
	}
	if _, _, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "preview-dup", "request-dup", ImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, Entries: []ImportEntry{entry, entry}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate", err)
	}
	preview, _, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "preview-i2", "request-i2", command)
	if err != nil {
		t.Fatal(err)
	}
	confirm := ConfirmImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, PreviewID: preview.ID, Entries: []ImportEntry{entry}}
	changed := confirm
	changed.Entries = []ImportEntry{{AggregateID: entry.AggregateID, SubmissionRevisionID: entry.SubmissionRevisionID, ExpectedVersion: 0, ValueMilli: 82000}}
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher"], "confirm-changed", "request-changed", changed); !errors.Is(err, ErrConflict) {
		t.Fatal("changed input", err)
	}
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher2"], "confirm-other", "request-other", confirm); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign actor", err)
	}
	grade := GradeCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, SubmissionRevisionID: work.revisionID, ExpectedVersion: 0, ValueMilli: 70000}
	if _, _, err = f.store.Grade(ctx, f.tokens["teacher"], "grade-before", "request-grade", grade); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher"], "confirm-stale", "request-stale", confirm); !errors.Is(err, ErrConflict) {
		t.Fatal("stale grade", err)
	}
	var importCount int
	if err = f.owner.QueryRow(ctx, "SELECT count(*) FROM v3.grade_revisions WHERE source_preview_id=$1", preview.ID).Scan(&importCount); err != nil || importCount != 0 {
		t.Fatal("partial import", importCount, err)
	}
	// Owner-only synthetic fixture insertion tests expiry without mutating immutable preview.
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.grade_import_previews(id,actor_id,key,class_id,grade_item_id,definition_version_id,publication_revision,input_hash,entries,expires_at)
	 SELECT $1,actor_id,$2,class_id,grade_item_id,definition_version_id,publication_revision,input_hash,entries,clock_timestamp()-interval '1 second' FROM v3.grade_import_previews WHERE id=$3`, "expired-"+preview.ID, "expired-"+preview.ID, preview.ID)
	if err != nil {
		t.Fatal(err)
	}
	confirm.PreviewID = "expired-" + preview.ID
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher"], "confirm-expired", "request-expired", confirm); !errors.Is(err, ErrConflict) {
		t.Fatal("expired", err)
	}
	unique, _ := token(4)
	foreign := f.submission(t, "foreign"+unique, "student", false)
	foreignEntry := entry
	foreignEntry.AggregateID, foreignEntry.SubmissionRevisionID = foreign.aggregateID, foreign.revisionID
	if _, _, err = f.store.PreviewImport(ctx, f.tokens["teacher"], "foreign-preview"+unique, "foreign-request", ImportCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, Entries: []ImportEntry{foreignEntry}}); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign ancestry", err)
	}
	self := f.submission(t, "importself"+unique, "teacher2", true)
	selfDefinition := createPublishedDefinition(t, f, self)
	selfEntry := ImportEntry{AggregateID: self.aggregateID, SubmissionRevisionID: self.revisionID, ExpectedVersion: 0, ValueMilli: 80000}
	if _, _, err = f.store.PreviewImport(ctx, f.tokens["teacher2"], "self-preview"+unique, "self-request", ImportCommand{ClassID: self.classID, GradeItemID: selfDefinition.GradeItemID, Entries: []ImportEntry{selfEntry}}); !errors.Is(err, ErrForbidden) {
		t.Fatal("self import", err)
	}
	policy := f.submission(t, "policy"+unique, "student", false)
	policyDefinition := createPublishedDefinition(t, f, policy)
	policyEntry := ImportEntry{AggregateID: policy.aggregateID, SubmissionRevisionID: policy.revisionID, ExpectedVersion: 0, ValueMilli: 80000}
	policyPreview, _, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "policy-preview"+unique, "policy-request", ImportCommand{ClassID: policy.classID, GradeItemID: policyDefinition.GradeItemID, Entries: []ImportEntry{policyEntry}})
	if err != nil {
		t.Fatal(err)
	}
	version := definitionCommand(policy, 5000, 5000)
	version.Action, version.ActivityID, version.Title, version.GradeItemID = "create_version", "", "", policyDefinition.GradeItemID
	var next Definition
	if json.Unmarshal(executeDefinition(t, f, "teacher", version), &next) != nil {
		t.Fatal("decode next version")
	}
	executeDefinition(t, f, "teacher", DefinitionCommand{Action: "publish", ClassID: policy.classID, GradeItemID: policyDefinition.GradeItemID, DefinitionVersionID: next.DefinitionVersionID, ExpectedPublicationRevision: 1})
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher"], "policy-confirm"+unique, "policy-confirm-request", ConfirmImportCommand{ClassID: policy.classID, GradeItemID: policyDefinition.GradeItemID, PreviewID: policyPreview.ID, Entries: []ImportEntry{policyEntry}}); !errors.Is(err, ErrConflict) {
		t.Fatal("stale policy", err)
	}
	// Two independent learners in one Class: row one must roll back if row two
	// becomes stale after preview. No cleanup or destructive fixture SQL.
	atomic := f.submission(t, "atomic"+unique, "student", false)
	atomicDefinition := createPublishedDefinition(t, f, atomic)
	var assignmentRevision string
	if err = f.owner.QueryRow(ctx, "SELECT assignment_revision_id FROM v3.submission_aggregates WHERE id=$1", atomic.aggregateID).Scan(&assignmentRevision); err != nil {
		t.Fatal(err)
	}
	otherAggregate, otherAttempt, otherRevision := "zzagg"+unique, "zzat"+unique, "zzsr"+unique
	_, err = f.owner.Exec(ctx, `INSERT INTO v3.enrollments(id,class_id,user_id,status) VALUES($1,$2,$3,'ACTIVE');
	 INSERT INTO v3.episodes(id,enrollment_id,created_by) VALUES($4,$1,$3);
	 INSERT INTO v3.roles(id,enrollment_id,class_id,role) VALUES($5,$1,$2,'student');
	 INSERT INTO v3.submission_aggregates(id,class_id,activity_id,assignment_revision_id,enrollment_id,episode_id,owner_user_id,created_by) VALUES($6,$2,$7,$8,$1,$4,$3,$3);
	 INSERT INTO v3.submission_attempts(id,aggregate_id,class_id,activity_id,assignment_revision_id,attempt_no,created_by) VALUES($9,$6,$2,$7,$8,1,$3);
	 INSERT INTO v3.submission_revisions(id,class_id,aggregate_id,attempt_id,revision_no,body,body_sha256,created_by) VALUES($10,$2,$6,$9,1,'other answer',$11,$3);
	 UPDATE v3.submission_attempts SET state='LOCKED',version=2,updated_at=clock_timestamp(),locked_at=clock_timestamp() WHERE id=$9;
	 INSERT INTO v3.submission_receipts(id,class_id,aggregate_id,attempt_id,revision_id,submitted_by) VALUES($12,$2,$6,$9,$10,$3)`, pgx.QueryExecModeSimpleProtocol,
		"zaen"+unique, atomic.classID, f.users["outsider"], "zaep"+unique, "zaro"+unique, otherAggregate, atomic.activityID, assignmentRevision, otherAttempt, otherRevision, digest([]byte("other answer")), "zarec"+unique)
	if err != nil {
		t.Fatal(err)
	}
	firstEntry := ImportEntry{AggregateID: atomic.aggregateID, SubmissionRevisionID: atomic.revisionID, ExpectedVersion: 0, ValueMilli: 81000}
	secondEntry := ImportEntry{AggregateID: otherAggregate, SubmissionRevisionID: otherRevision, ExpectedVersion: 0, ValueMilli: 82000}
	atomicPreview, _, err := f.store.PreviewImport(ctx, f.tokens["teacher"], "atomic-preview"+unique, "atomic-request", ImportCommand{ClassID: atomic.classID, GradeItemID: atomicDefinition.GradeItemID, Entries: []ImportEntry{firstEntry, secondEntry}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.Grade(ctx, f.tokens["teacher"], "atomic-staler"+unique, "atomic-grade", GradeCommand{ClassID: atomic.classID, GradeItemID: atomicDefinition.GradeItemID, AggregateID: otherAggregate, SubmissionRevisionID: otherRevision, ExpectedVersion: 0, ValueMilli: 72000}); err != nil {
		t.Fatal(err)
	}
	if _, _, err = f.store.ConfirmImport(ctx, f.tokens["teacher"], "atomic-confirm"+unique, "atomic-confirm-request", ConfirmImportCommand{ClassID: atomic.classID, GradeItemID: atomicDefinition.GradeItemID, PreviewID: atomicPreview.ID, Entries: []ImportEntry{firstEntry, secondEntry}}); !errors.Is(err, ErrConflict) {
		t.Fatal("multirow rollback", err)
	}
	if _, err = f.store.ReadGrade(ctx, f.tokens["teacher"], atomic.classID, atomicDefinition.GradeItemID, atomic.aggregateID); !errors.Is(err, ErrNotFound) {
		t.Fatal("partial first row", err)
	}
}

func TestSyntheticReleaseLifecycle(t *testing.T) {
	f, work, definition, entry := importCase(t, "r1")
	ctx := context.Background()
	command := GradeCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, SubmissionRevisionID: work.revisionID, ExpectedVersion: 0, ValueMilli: entry.ValueMilli}
	if _, _, err := f.store.Grade(ctx, f.tokens["teacher"], "grade-r1", "request-r1", command); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ReadGrade(ctx, f.tokens["student"], work.classID, definition.GradeItemID, work.aggregateID); !errors.Is(err, ErrNotFound) {
		t.Fatal("unreleased visible", err)
	}
	release := ReleaseCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, ExpectedVersion: 1}
	first, replay, err := f.store.ReleaseGrade(ctx, f.tokens["teacher"], "release-r1", "request-r2", release)
	if err != nil || replay || first.GradeVersion != 1 {
		t.Fatal(first, replay, err)
	}
	visible, err := f.store.ReadGrade(ctx, f.tokens["student"], work.classID, definition.GradeItemID, work.aggregateID)
	if err != nil || visible.ValueMilli != 81000 {
		t.Fatal(visible, err)
	}
	command.ExpectedVersion = 1
	command.ValueMilli = 90000
	command.Reason = "Moderated"
	if _, _, err = f.store.Grade(ctx, f.tokens["teacher2"], "grade-r2", "request-r3", command); err != nil {
		t.Fatal(err)
	}
	visible, err = f.store.ReadGrade(ctx, f.tokens["student"], work.classID, definition.GradeItemID, work.aggregateID)
	if err != nil || visible.Version != 1 || visible.ValueMilli != 81000 {
		t.Fatal("regrade leaked", visible, err)
	}
	release.ExpectedVersion = 2
	if _, _, err = f.store.ReleaseGrade(ctx, f.tokens["teacher"], "release-r2", "request-r4", release); err != nil {
		t.Fatal(err)
	}
	visible, err = f.store.ReadGrade(ctx, f.tokens["student"], work.classID, definition.GradeItemID, work.aggregateID)
	if err != nil || visible.Version != 2 || visible.ValueMilli != 90000 {
		t.Fatal(visible, err)
	}
}

func TestSyntheticReleaseBoundary(t *testing.T) {
	f, work, definition, entry := importCase(t, "r2")
	ctx := context.Background()
	command := GradeCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, SubmissionRevisionID: work.revisionID, ExpectedVersion: 0, ValueMilli: entry.ValueMilli}
	if _, _, err := f.store.Grade(ctx, f.tokens["teacher"], "grade-bound", "request-bound", command); err != nil {
		t.Fatal(err)
	}
	release := ReleaseCommand{ClassID: work.classID, GradeItemID: definition.GradeItemID, AggregateID: work.aggregateID, ExpectedVersion: 1}
	if _, _, err := f.store.ReleaseGrade(ctx, f.tokens["student"], "student-rel", "request-student", release); !errors.Is(err, ErrForbidden) {
		t.Fatal("student release", err)
	}
	if _, _, err := f.store.ReleaseGrade(ctx, f.tokens["outsider"], "outside-rel", "request-outside", release); !errors.Is(err, ErrForbidden) {
		t.Fatal("outsider release", err)
	}
	bad := release
	bad.ExpectedVersion = 2
	if _, _, err := f.store.ReleaseGrade(ctx, f.tokens["teacher"], "stale-rel", "request-stale", bad); !errors.Is(err, ErrConflict) {
		t.Fatal("stale release", err)
	}
	first, replay, err := f.store.ReleaseGrade(ctx, f.tokens["teacher"], "release-bound", "request-first", release)
	if err != nil || replay {
		t.Fatal(first, replay, err)
	}
	again, replay, err := f.store.ReleaseGrade(ctx, f.tokens["teacher"], "release-bound", "request-replay", release)
	if err != nil || !replay || again.ID != first.ID {
		t.Fatal(again, replay, err)
	}
	if _, _, err = f.store.ReleaseGrade(ctx, f.tokens["teacher2"], "duplicate-rel", "request-duplicate", release); !errors.Is(err, ErrConflict) {
		t.Fatal("duplicate release", err)
	}
	// A contextual teacher who owns the submission cannot release their own grade.
	unique, _ := token(4)
	self := f.submission(t, "self"+unique, "teacher2", true)
	d := createPublishedDefinition(t, f, self)
	selfGrade := GradeCommand{ClassID: self.classID, GradeItemID: d.GradeItemID, AggregateID: self.aggregateID, SubmissionRevisionID: self.revisionID, ExpectedVersion: 0, ValueMilli: 70000}
	if _, _, err = f.store.Grade(ctx, f.tokens["teacher"], "self-grade", "request-self-grade", selfGrade); err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.ReadGrade(ctx, f.tokens["teacher2"], self.classID, d.GradeItemID, self.aggregateID); !errors.Is(err, ErrNotFound) {
		t.Fatal("teacher-learner read bypass", err)
	}
	if _, _, err = f.store.ReleaseGrade(ctx, f.tokens["teacher2"], "self-release", "request-self-release", ReleaseCommand{ClassID: self.classID, GradeItemID: d.GradeItemID, AggregateID: self.aggregateID, ExpectedVersion: 1}); !errors.Is(err, ErrForbidden) {
		t.Fatal("self release", err)
	}
	if _, _, err = f.store.ReleaseGrade(ctx, f.tokens["teacher"], "other-release", "request-other-release", ReleaseCommand{ClassID: self.classID, GradeItemID: d.GradeItemID, AggregateID: self.aggregateID, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if visible, readErr := f.store.ReadGrade(ctx, f.tokens["teacher2"], self.classID, d.GradeItemID, self.aggregateID); readErr != nil || visible.ValueMilli != 70000 {
		t.Fatal("teacher-learner released read", visible, readErr)
	}
}
