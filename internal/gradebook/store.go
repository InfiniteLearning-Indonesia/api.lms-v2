package gradebook

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
	"lmsv3/internal/platform/transaction"
)

var ErrInvalid = errors.New("INVALID_INPUT")
var ErrForbidden = errors.New("FORBIDDEN")
var ErrNotFound = errors.New("NOT_FOUND")
var ErrConflict = errors.New("CONFLICT")

var keyPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

type ScaleInput struct {
	Name            string `json:"name"`
	MinimumMilli    int64  `json:"minimum_milli"`
	MaximumMilli    int64  `json:"maximum_milli"`
	DisplayDecimals int16  `json:"display_decimals"`
}

type CriterionInput struct {
	Key          string `json:"key"`
	Title        string `json:"title"`
	ParentKey    string `json:"parent_key,omitempty"`
	WeightBPS    int    `json:"weight_bps"`
	MaximumMilli int64  `json:"maximum_milli"`
	Position     int    `json:"position"`
}

type DefinitionCommand struct {
	Action                      string           `json:"action"`
	ClassID                     string           `json:"class_id"`
	GradeItemID                 string           `json:"grade_item_id,omitempty"`
	ActivityID                  string           `json:"activity_id,omitempty"`
	Title                       string           `json:"title,omitempty"`
	WeightBPS                   int              `json:"weight_bps,omitempty"`
	Scale                       ScaleInput       `json:"scale,omitempty"`
	Criteria                    []CriterionInput `json:"criteria,omitempty"`
	DefinitionVersionID         string           `json:"definition_version_id,omitempty"`
	ExpectedPublicationRevision int64            `json:"expected_publication_revision,omitempty"`
}

type Definition struct {
	GradeItemID         string      `json:"grade_item_id"`
	ClassID             string      `json:"class_id"`
	ActivityID          string      `json:"activity_id"`
	Title               string      `json:"title"`
	DefinitionVersionID string      `json:"definition_version_id"`
	Version             int         `json:"version"`
	ScaleID             string      `json:"scale_id"`
	Scale               ScaleInput  `json:"scale"`
	WeightBPS           int         `json:"weight_bps"`
	Criteria            []Criterion `json:"criteria"`
	CreatedAt           time.Time   `json:"created_at"`
}

type Criterion struct {
	ID           string `json:"id"`
	Key          string `json:"key"`
	Title        string `json:"title"`
	ParentID     string `json:"parent_id,omitempty"`
	WeightBPS    int    `json:"weight_bps"`
	MaximumMilli int64  `json:"maximum_milli"`
	Position     int    `json:"position"`
}

type Publication struct {
	ID                          string    `json:"id"`
	GradeItemID                 string    `json:"grade_item_id"`
	ClassID                     string    `json:"class_id"`
	DefinitionVersionID         string    `json:"definition_version_id"`
	Revision                    int64     `json:"revision"`
	PreviousDefinitionVersionID *string   `json:"previous_definition_version_id,omitempty"`
	ActorID                     string    `json:"actor_id"`
	PublishedAt                 time.Time `json:"published_at"`
}

type GradeCommand struct {
	ClassID              string `json:"class_id"`
	GradeItemID          string `json:"grade_item_id"`
	AggregateID          string `json:"aggregate_id"`
	SubmissionRevisionID string `json:"submission_revision_id"`
	ExpectedVersion      int64  `json:"expected_version"`
	ValueMilli           int64  `json:"value_milli"`
	Reason               string `json:"reason,omitempty"`
}

type Grade struct {
	ID                   string    `json:"id"`
	ClassID              string    `json:"class_id"`
	GradeItemID          string    `json:"grade_item_id"`
	DefinitionVersionID  string    `json:"definition_version_id"`
	AggregateID          string    `json:"aggregate_id"`
	AttemptID            string    `json:"attempt_id"`
	SubmissionRevisionID string    `json:"submission_revision_id"`
	LearnerUserID        string    `json:"learner_user_id"`
	GraderUserID         string    `json:"grader_user_id"`
	Version              int64     `json:"version"`
	ValueMilli           int64     `json:"value_milli"`
	Reason               string    `json:"reason"`
	CreatedAt            time.Time `json:"created_at"`
}

type Store struct {
	pool    *pgxpool.Pool
	resolve func(context.Context, pgx.Tx, string) (identity.Actor, error)
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool, resolve: identity.ResolveBridge} }

func token(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func digest(b []byte) string { value := sha256.Sum256(b); return hex.EncodeToString(value[:]) }

func validIdempotency(key, requestID string) bool {
	if len(key) < 8 || len(key) > 80 || requestID == "" {
		return false
	}
	for _, r := range key {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}

func mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23503" || pgErr.Code == "23505" || pgErr.Code == "23514") {
		return fmt.Errorf("%w: %s", ErrConflict, pgErr.Message)
	}
	return err
}

func teacher(ctx context.Context, tx pgx.Tx, actor identity.Actor, classID string) (bool, error) {
	if actor.Admin {
		return true, nil
	}
	var allowed bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.classes c JOIN v3.enrollments e ON e.class_id=c.id JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
		WHERE c.id=$1 AND c.state='PUBLISHED' AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
		AND NOT EXISTS(SELECT 1 FROM v3.class_access x WHERE x.class_id=c.id AND x.closed_at IS NOT NULL)
		AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access x WHERE x.enrollment_id=e.id AND x.suspended_at IS NOT NULL))`, classID, actor.ID).Scan(&allowed)
	return allowed, err
}

func audit(ctx context.Context, tx pgx.Tx, actor, classID, action, requestID string) error {
	id, err := token(16)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$5)", id, actor, classID, action, requestID); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, "INSERT INTO v3.outbox(event_id) VALUES($1)", id)
	return err
}

func normalizeDefinition(c DefinitionCommand) (DefinitionCommand, error) {
	c.Action = strings.TrimSpace(strings.ToLower(c.Action))
	c.Title = strings.TrimSpace(c.Title)
	c.Scale.Name = strings.TrimSpace(c.Scale.Name)
	if c.ClassID == "" {
		return c, ErrInvalid
	}
	switch c.Action {
	case "create_item", "create_version":
		if (c.Action == "create_item" && (c.ActivityID == "" || c.GradeItemID != "")) || (c.Action == "create_version" && (c.GradeItemID == "" || c.ActivityID != "" || c.Title != "")) || c.WeightBPS < 1 || c.WeightBPS > 10000 || len(c.Scale.Name) < 1 || len(c.Scale.Name) > 120 || c.Scale.MinimumMilli >= c.Scale.MaximumMilli || c.Scale.MinimumMilli < -1000000000 || c.Scale.MaximumMilli > 1000000000 || c.Scale.DisplayDecimals < 0 || c.Scale.DisplayDecimals > 3 || len(c.Criteria) < 1 || len(c.Criteria) > 100 {
			return c, ErrInvalid
		}
		if c.Action == "create_item" && (len(c.Title) < 1 || len(c.Title) > 160) {
			return c, ErrInvalid
		}
		keys, positions := map[string]bool{}, map[int]bool{}
		for i := range c.Criteria {
			c.Criteria[i].Key = strings.ToLower(strings.TrimSpace(c.Criteria[i].Key))
			c.Criteria[i].ParentKey = strings.ToLower(strings.TrimSpace(c.Criteria[i].ParentKey))
			c.Criteria[i].Title = strings.TrimSpace(c.Criteria[i].Title)
			r := c.Criteria[i]
			if !keyPattern.MatchString(r.Key) || keys[r.Key] || len(r.Title) < 1 || len(r.Title) > 240 || r.WeightBPS < 1 || r.WeightBPS > 10000 || r.MaximumMilli < 1 || r.MaximumMilli > 1000000000 || r.Position < 1 || r.Position > 1000 || positions[r.Position] {
				return c, ErrInvalid
			}
			keys[r.Key], positions[r.Position] = true, true
		}
		for _, r := range c.Criteria {
			if r.ParentKey != "" && !keys[r.ParentKey] {
				return c, ErrInvalid
			}
		}
	case "publish":
		if c.GradeItemID == "" || c.DefinitionVersionID == "" || c.ExpectedPublicationRevision < 0 || c.ActivityID != "" || c.Title != "" || len(c.Criteria) != 0 {
			return c, ErrInvalid
		}
	default:
		return c, ErrInvalid
	}
	return c, nil
}

func (s *Store) ExecuteDefinition(ctx context.Context, session, key, requestID string, command DefinitionCommand) (json.RawMessage, bool, error) {
	if !validIdempotency(key, requestID) {
		return nil, false, ErrInvalid
	}
	command, err := normalizeDefinition(command)
	if err != nil {
		return nil, false, err
	}
	request, _ := json.Marshal(command)
	requestHash := digest(request)
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, command.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":"+key); e != nil {
			return e
		}
		var prior string
		e = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.grade_definition_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
		if e == nil {
			if prior != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		var value any
		if command.Action == "create_item" || command.Action == "create_version" {
			itemID := command.GradeItemID
			version := 1
			if command.Action == "create_item" {
				itemID, e = token(16)
				if e != nil {
					return e
				}
				_, e = tx.Exec(ctx, "INSERT INTO v3.grade_items(id,class_id,activity_id,title,created_by) VALUES($1,$2,$3,$4,$5)", itemID, command.ClassID, command.ActivityID, command.Title, actor.ID)
				if e != nil {
					return mapWriteError(e)
				}
			} else {
				e = tx.QueryRow(ctx, "SELECT coalesce(max(version),0)+1 FROM v3.grade_definition_versions WHERE grade_item_id=$1 AND class_id=$2", itemID, command.ClassID).Scan(&version)
				if e != nil || version == 1 {
					if e != nil {
						return e
					}
					return ErrNotFound
				}
			}
			scaleID, _ := token(16)
			definitionID, _ := token(16)
			_, e = tx.Exec(ctx, "INSERT INTO v3.grade_scales(id,class_id,name,minimum_milli,maximum_milli,display_decimals,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)", scaleID, command.ClassID, command.Scale.Name, command.Scale.MinimumMilli, command.Scale.MaximumMilli, command.Scale.DisplayDecimals, actor.ID)
			if e != nil {
				return mapWriteError(e)
			}
			_, e = tx.Exec(ctx, "INSERT INTO v3.grade_definition_versions(id,grade_item_id,class_id,version,scale_id,weight_bps,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)", definitionID, itemID, command.ClassID, version, scaleID, command.WeightBPS, actor.ID)
			if e != nil {
				return mapWriteError(e)
			}
			ids := map[string]string{}
			for _, criterion := range command.Criteria {
				ids[criterion.Key], _ = token(16)
			}
			criteria := make([]Criterion, 0, len(command.Criteria))
			for _, input := range command.Criteria {
				parent := any(nil)
				if input.ParentKey != "" {
					parent = ids[input.ParentKey]
				}
				_, e = tx.Exec(ctx, "INSERT INTO v3.rubric_criteria(id,definition_version_id,grade_item_id,class_id,criterion_key,title,parent_criterion_id,weight_bps,maximum_milli,position) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)", ids[input.Key], definitionID, itemID, command.ClassID, input.Key, input.Title, parent, input.WeightBPS, input.MaximumMilli, input.Position)
				if e != nil {
					return mapWriteError(e)
				}
				parentID := ""
				if parent != nil {
					parentID = parent.(string)
				}
				criteria = append(criteria, Criterion{ID: ids[input.Key], Key: input.Key, Title: input.Title, ParentID: parentID, WeightBPS: input.WeightBPS, MaximumMilli: input.MaximumMilli, Position: input.Position})
			}
			value = Definition{GradeItemID: itemID, ClassID: command.ClassID, ActivityID: command.ActivityID, Title: command.Title, DefinitionVersionID: definitionID, Version: version, ScaleID: scaleID, Scale: command.Scale, WeightBPS: command.WeightBPS, Criteria: criteria}
		} else {
			var lastRevision int64
			var previous *string
			e = tx.QueryRow(ctx, "SELECT revision,definition_version_id FROM v3.grade_definition_publications WHERE grade_item_id=$1 AND class_id=$2 ORDER BY revision DESC LIMIT 1", command.GradeItemID, command.ClassID).Scan(&lastRevision, &previous)
			if errors.Is(e, pgx.ErrNoRows) {
				lastRevision, previous, e = 0, nil, nil
			}
			if e != nil {
				return e
			}
			if lastRevision != command.ExpectedPublicationRevision {
				return ErrConflict
			}
			publicationID, _ := token(16)
			var publication Publication
			e = tx.QueryRow(ctx, `INSERT INTO v3.grade_definition_publications(id,grade_item_id,class_id,definition_version_id,revision,previous_definition_version_id,actor_id) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id,grade_item_id,class_id,definition_version_id,revision,previous_definition_version_id,actor_id,published_at`, publicationID, command.GradeItemID, command.ClassID, command.DefinitionVersionID, lastRevision+1, previous, actor.ID).Scan(&publication.ID, &publication.GradeItemID, &publication.ClassID, &publication.DefinitionVersionID, &publication.Revision, &publication.PreviousDefinitionVersionID, &publication.ActorID, &publication.PublishedAt)
			if e != nil {
				return mapWriteError(e)
			}
			value = publication
		}
		response, e = json.Marshal(value)
		if e != nil {
			return e
		}
		if e = audit(ctx, tx, actor.ID, command.ClassID, "gradebook.definition."+command.Action, requestID); e != nil {
			return e
		}
		return tx.QueryRow(ctx, "INSERT INTO v3.grade_definition_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb) RETURNING response", actor.ID, key, requestHash, string(response)).Scan(&response)
	})
	return response, replayed, mapWriteError(err)
}

func normalizeGrade(c GradeCommand) (GradeCommand, error) {
	c.Reason = strings.TrimSpace(c.Reason)
	if c.ClassID == "" || c.GradeItemID == "" || c.AggregateID == "" || c.SubmissionRevisionID == "" || c.ExpectedVersion < 0 || !utf8.ValidString(c.Reason) || len(c.Reason) > 1000 || (c.ExpectedVersion > 0 && c.Reason == "") {
		return c, ErrInvalid
	}
	if c.ExpectedVersion == 0 {
		c.Reason = "INITIAL"
	}
	return c, nil
}

func scanGrade(row pgx.Row, g *Grade) error {
	return row.Scan(&g.ID, &g.ClassID, &g.GradeItemID, &g.DefinitionVersionID, &g.AggregateID, &g.AttemptID, &g.SubmissionRevisionID, &g.LearnerUserID, &g.GraderUserID, &g.Version, &g.ValueMilli, &g.Reason, &g.CreatedAt)
}

func (s *Store) Grade(ctx context.Context, session, key, requestID string, command GradeCommand) (json.RawMessage, bool, error) {
	if !validIdempotency(key, requestID) {
		return nil, false, ErrInvalid
	}
	command, err := normalizeGrade(command)
	if err != nil {
		return nil, false, err
	}
	request, _ := json.Marshal(command)
	requestHash := digest(request)
	var response json.RawMessage
	replayed := false
	err = transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, command.ClassID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", actor.ID+":"+key); e != nil {
			return e
		}
		var prior string
		e = tx.QueryRow(ctx, "SELECT request_hash,response FROM v3.grade_commands WHERE actor_id=$1 AND key=$2", actor.ID, key).Scan(&prior, &response)
		if e == nil {
			if prior != requestHash {
				return ErrConflict
			}
			replayed = true
			return nil
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", command.GradeItemID+":"+command.AggregateID); e != nil {
			return e
		}
		var attemptID, learnerID, definitionID string
		var minimum, maximum int64
		e = tx.QueryRow(ctx, `SELECT sr.attempt_id,sa.owner_user_id,p.definition_version_id,sc.minimum_milli,sc.maximum_milli
			FROM v3.submission_revisions sr JOIN v3.submission_receipts receipt ON receipt.revision_id=sr.id AND receipt.class_id=sr.class_id AND receipt.attempt_id=sr.attempt_id
			JOIN v3.submission_aggregates sa ON sa.id=sr.aggregate_id AND sa.class_id=sr.class_id
			JOIN v3.grade_items gi ON gi.class_id=sa.class_id AND gi.activity_id=sa.activity_id
			JOIN v3.grade_definition_publications p ON p.grade_item_id=gi.id AND p.revision=(SELECT max(x.revision) FROM v3.grade_definition_publications x WHERE x.grade_item_id=gi.id)
			JOIN v3.grade_definition_versions d ON d.id=p.definition_version_id AND d.grade_item_id=p.grade_item_id AND d.class_id=p.class_id JOIN v3.grade_scales sc ON sc.id=d.scale_id AND sc.class_id=d.class_id
			WHERE sr.id=$1 AND sr.aggregate_id=$2 AND sr.class_id=$3 AND gi.id=$4`, command.SubmissionRevisionID, command.AggregateID, command.ClassID, command.GradeItemID).Scan(&attemptID, &learnerID, &definitionID, &minimum, &maximum)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		if learnerID == actor.ID {
			return ErrForbidden
		}
		if command.ValueMilli < minimum || command.ValueMilli > maximum {
			return ErrInvalid
		}
		var current int64
		e = tx.QueryRow(ctx, "SELECT version FROM v3.grade_current WHERE grade_item_id=$1 AND aggregate_id=$2 FOR UPDATE", command.GradeItemID, command.AggregateID).Scan(&current)
		if errors.Is(e, pgx.ErrNoRows) {
			current, e = 0, nil
		}
		if e != nil {
			return e
		}
		if current != command.ExpectedVersion {
			return ErrConflict
		}
		gradeID, _ := token(16)
		var grade Grade
		e = scanGrade(tx.QueryRow(ctx, `INSERT INTO v3.grade_revisions(id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,learner_user_id,grader_user_id,version,value_milli,reason) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id,class_id,grade_item_id,definition_version_id,aggregate_id,attempt_id,submission_revision_id,learner_user_id,grader_user_id,version,value_milli,reason,created_at`, gradeID, command.ClassID, command.GradeItemID, definitionID, command.AggregateID, attemptID, command.SubmissionRevisionID, learnerID, actor.ID, current+1, command.ValueMilli, command.Reason), &grade)
		if e != nil {
			return mapWriteError(e)
		}
		if current == 0 {
			_, e = tx.Exec(ctx, "INSERT INTO v3.grade_current(grade_item_id,aggregate_id,class_id,revision_id,version) VALUES($1,$2,$3,$4,1)", command.GradeItemID, command.AggregateID, command.ClassID, gradeID)
		} else {
			var tag pgconn.CommandTag
			tag, e = tx.Exec(ctx, "UPDATE v3.grade_current SET revision_id=$4,version=version+1,updated_at=clock_timestamp() WHERE grade_item_id=$1 AND aggregate_id=$2 AND class_id=$3 AND version=$5", command.GradeItemID, command.AggregateID, command.ClassID, gradeID, current)
			if e == nil && tag.RowsAffected() != 1 {
				return ErrConflict
			}
		}
		if e != nil {
			return mapWriteError(e)
		}
		response, e = json.Marshal(grade)
		if e != nil {
			return e
		}
		if e = audit(ctx, tx, actor.ID, command.ClassID, "gradebook.grade", requestID); e != nil {
			return e
		}
		return tx.QueryRow(ctx, "INSERT INTO v3.grade_commands(actor_id,key,request_hash,response) VALUES($1,$2,$3,$4::jsonb) RETURNING response", actor.ID, key, requestHash, string(response)).Scan(&response)
	})
	return response, replayed, mapWriteError(err)
}

func (s *Store) ReadGrade(ctx context.Context, session, classID, itemID, aggregateID string) (Grade, error) {
	var g Grade
	if classID == "" || itemID == "" || aggregateID == "" {
		return g, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, classID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		e = scanGrade(tx.QueryRow(ctx, `SELECT r.id,r.class_id,r.grade_item_id,r.definition_version_id,r.aggregate_id,r.attempt_id,r.submission_revision_id,r.learner_user_id,r.grader_user_id,r.version,r.value_milli,r.reason,r.created_at FROM v3.grade_current c JOIN v3.grade_revisions r ON r.id=c.revision_id WHERE c.class_id=$1 AND c.grade_item_id=$2 AND c.aggregate_id=$3`, classID, itemID, aggregateID), &g)
		if errors.Is(e, pgx.ErrNoRows) {
			return ErrNotFound
		}
		return e
	})
	return g, err
}

func (s *Store) ReadDefinition(ctx context.Context, session, classID, itemID string) ([]Definition, error) {
	result := []Definition{}
	if classID == "" || itemID == "" {
		return result, ErrInvalid
	}
	err := transaction.Within(ctx, s.pool, func(tx pgx.Tx) error {
		actor, e := s.resolve(ctx, tx, session)
		if e != nil {
			return e
		}
		allowed, e := teacher(ctx, tx, actor, classID)
		if e != nil {
			return e
		}
		if !allowed {
			return ErrForbidden
		}
		rows, e := tx.Query(ctx, `SELECT gi.id,gi.class_id,gi.activity_id,gi.title,d.id,d.version,d.scale_id,sc.name,sc.minimum_milli,sc.maximum_milli,sc.display_decimals,d.weight_bps,d.created_at FROM v3.grade_items gi JOIN v3.grade_definition_versions d ON d.grade_item_id=gi.id AND d.class_id=gi.class_id JOIN v3.grade_scales sc ON sc.id=d.scale_id AND sc.class_id=d.class_id WHERE gi.id=$1 AND gi.class_id=$2 ORDER BY d.version`, itemID, classID)
		if e != nil {
			return e
		}
		for rows.Next() {
			var d Definition
			if e = rows.Scan(&d.GradeItemID, &d.ClassID, &d.ActivityID, &d.Title, &d.DefinitionVersionID, &d.Version, &d.ScaleID, &d.Scale.Name, &d.Scale.MinimumMilli, &d.Scale.MaximumMilli, &d.Scale.DisplayDecimals, &d.WeightBPS, &d.CreatedAt); e != nil {
				return e
			}
			d.Criteria = []Criterion{}
			result = append(result, d)
		}
		if e = rows.Err(); e != nil {
			rows.Close()
			return e
		}
		rows.Close()
		for index := range result {
			criteria, queryErr := tx.Query(ctx, "SELECT id,criterion_key,title,coalesce(parent_criterion_id,''),weight_bps,maximum_milli,position FROM v3.rubric_criteria WHERE definition_version_id=$1 ORDER BY position,id", result[index].DefinitionVersionID)
			if queryErr != nil {
				return queryErr
			}
			for criteria.Next() {
				var c Criterion
				if e = criteria.Scan(&c.ID, &c.Key, &c.Title, &c.ParentID, &c.WeightBPS, &c.MaximumMilli, &c.Position); e != nil {
					criteria.Close()
					return e
				}
				result[index].Criteria = append(result[index].Criteria, c)
			}
			if e = criteria.Err(); e != nil {
				criteria.Close()
				return e
			}
			criteria.Close()
		}
		if len(result) == 0 {
			return ErrNotFound
		}
		return nil
	})
	return result, err
}
