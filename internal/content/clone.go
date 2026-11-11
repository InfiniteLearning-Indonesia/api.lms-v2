package content

import (
	"context"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
)

type CloneMapping struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
}

type CloneResult struct {
	SourceClassID string         `json:"source_class_id"`
	TargetClassID string         `json:"target_class_id"`
	Sections      []CloneMapping `json:"sections"`
	Activities    []CloneMapping `json:"activities"`
}

type sourceSection struct {
	id       string
	title    string
	position int
}

type sourceActivity struct {
	id        string
	sectionID string
	kind      string
	title     string
	position  int
}

func (s *Store) cloneStructure(ctx context.Context, tx pgx.Tx, actor identity.Actor, command Command) (CloneResult, error) {
	result := CloneResult{SourceClassID: command.SourceClassID, TargetClassID: command.ClassID,
		Sections: []CloneMapping{}, Activities: []CloneMapping{}}
	var sourceState, targetState string
	if err := tx.QueryRow(ctx, "SELECT state FROM v3.classes WHERE id=$1", command.SourceClassID).Scan(&sourceState); err != nil {
		return result, err
	}
	if err := tx.QueryRow(ctx, "SELECT state FROM v3.classes WHERE id=$1", command.ClassID).Scan(&targetState); err != nil {
		return result, err
	}
	if sourceState != "PUBLISHED" && sourceState != "ARCHIVED" || targetState != "DRAFT" {
		return result, ErrConflict
	}
	if err := s.context(ctx, tx, actor, command.SourceClassID, false); err != nil {
		return result, err
	}
	author, err := actorIsAuthor(ctx, tx, actor, command.SourceClassID)
	if err != nil {
		return result, err
	}
	if !author {
		return result, ErrForbidden
	}
	var occupied bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM v3.content_sections WHERE class_id=$1)", command.ClassID).Scan(&occupied); err != nil {
		return result, err
	}
	if occupied {
		return result, ErrConflict
	}
	sections := []sourceSection{}
	rows, err := tx.Query(ctx, `SELECT id,title,position FROM v3.content_sections WHERE class_id=$1 ORDER BY position,id LIMIT 51`, command.SourceClassID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var row sourceSection
		if err = rows.Scan(&row.id, &row.title, &row.position); err != nil {
			break
		}
		sections = append(sections, row)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(sections) == 0 || len(sections) > 50 {
		return result, ErrConflict
	}
	activities := []sourceActivity{}
	rows, err = tx.Query(ctx, `SELECT id,section_id,activity_type,title,position FROM v3.content_activities WHERE class_id=$1 ORDER BY section_id,position,id LIMIT 201`, command.SourceClassID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var row sourceActivity
		if err = rows.Scan(&row.id, &row.sectionID, &row.kind, &row.title, &row.position); err != nil {
			break
		}
		activities = append(activities, row)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return result, err
	}
	if len(activities) > 200 {
		return result, ErrConflict
	}
	sectionMap := make(map[string]string, len(sections))
	for _, source := range sections {
		id, tokenErr := token(16)
		if tokenErr != nil {
			return result, tokenErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v3.content_sections(id,class_id,title,position,created_by) VALUES($1,$2,$3,$4,$5)`, id, command.ClassID, source.title, source.position, actor.ID); err != nil {
			return result, mapWriteError(err)
		}
		sectionMap[source.id] = id
		result.Sections = append(result.Sections, CloneMapping{SourceID: source.id, TargetID: id})
	}
	for _, source := range activities {
		sectionID := sectionMap[source.sectionID]
		if sectionID == "" {
			return result, ErrConflict
		}
		id, tokenErr := token(16)
		if tokenErr != nil {
			return result, tokenErr
		}
		if _, err = tx.Exec(ctx, `INSERT INTO v3.content_activities(id,class_id,section_id,activity_type,title,position,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, id, command.ClassID, sectionID, source.kind, source.title, source.position, actor.ID); err != nil {
			return result, mapWriteError(err)
		}
		result.Activities = append(result.Activities, CloneMapping{SourceID: source.id, TargetID: id})
	}
	return result, nil
}
