package reporting

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"lmsv3/internal/identity"
)

var ErrInvalid = errors.New("REPORT_INVALID")
var ErrForbidden = errors.New("REPORT_FORBIDDEN")
var ErrNotFound = errors.New("REPORT_NOT_FOUND")
var identifier = regexp.MustCompile(`^[a-f0-9]{32}$`)

type Grade struct {
	GradeItemID  string    `json:"grade_item_id"`
	Title        string    `json:"title"`
	ValueMilli   int64     `json:"value_milli"`
	GradeVersion int64     `json:"grade_version"`
	ReleaseID    string    `json:"release_id"`
	ReleasedAt   time.Time `json:"released_at"`
}

type Watermark struct {
	ReleaseID  string    `json:"release_id"`
	ReleasedAt time.Time `json:"released_at"`
}

type Report struct {
	ContractVersion int        `json:"contract_version"`
	ClassID         string     `json:"class_id"`
	LearnerUserID   string     `json:"learner_user_id"`
	Watermark       *Watermark `json:"watermark"`
	Grades          []Grade    `json:"grades"`
}

type Store struct {
	Pool      *pgxpool.Pool
	cursorKey [32]byte
}

// Shared with the synthetic EXPLAIN probe so evidence cannot benchmark a substitute query.
const releasedGradesSQL = `SELECT DISTINCT ON (l.grade_item_id) l.grade_item_id,gi.title,r.value_milli,l.grade_version,l.id,l.released_at
	 FROM v3.grade_releases l JOIN v3.grade_revisions r ON r.id=l.revision_id AND r.class_id=l.class_id
	 JOIN v3.grade_items gi ON gi.id=l.grade_item_id AND gi.class_id=l.class_id
	 WHERE l.class_id=$1 AND l.learner_user_id=$2
	 ORDER BY l.grade_item_id,l.released_at DESC,l.id DESC LIMIT 201`

// ReleasedGrades reads canonical release history in one read-only snapshot.
// No worker or projection lag can make a committed release invisible.
func (s Store) ReleasedGrades(ctx context.Context, session, classID, learnerID string) (Report, error) {
	if s.Pool == nil {
		return Report{}, ErrInvalid
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return Report{}, err
	}
	defer tx.Rollback(ctx)
	report, err := s.releasedGradesTx(ctx, tx, session, classID, learnerID)
	if err != nil {
		return Report{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Report{}, err
	}
	return report, nil
}

func (s Store) releasedGradesTx(ctx context.Context, tx pgx.Tx, session, classID, learnerID string) (Report, error) {
	if !identifier.MatchString(classID) || !identifier.MatchString(learnerID) || s.Pool == nil {
		return Report{}, ErrInvalid
	}
	actor, err := identity.ResolveBridge(ctx, tx, session)
	if err != nil {
		return Report{}, err
	}
	var active bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.classes c JOIN v3.enrollments e ON e.class_id=c.id
	 WHERE c.id=$1 AND c.state='PUBLISHED' AND e.user_id=$2 AND e.status='ACTIVE'
	 AND NOT EXISTS(SELECT 1 FROM v3.class_access ca WHERE ca.class_id=c.id AND ca.closed_at IS NOT NULL)
	 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access ea WHERE ea.enrollment_id=e.id AND ea.suspended_at IS NOT NULL))`, classID, learnerID).Scan(&active)
	if err != nil {
		return Report{}, err
	}
	if !active {
		return Report{}, ErrNotFound
	}
	if !actor.Admin && actor.ID != learnerID {
		var teacher bool
		err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM v3.enrollments e JOIN v3.roles r ON r.enrollment_id=e.id AND r.class_id=e.class_id
		 WHERE e.class_id=$1 AND e.user_id=$2 AND e.status='ACTIVE' AND r.role='teacher' AND r.revoked_at IS NULL
		 AND NOT EXISTS(SELECT 1 FROM v3.enrollment_access ea WHERE ea.enrollment_id=e.id AND ea.suspended_at IS NOT NULL))`, classID, actor.ID).Scan(&teacher)
		if err != nil {
			return Report{}, err
		}
		if !teacher {
			return Report{}, ErrForbidden
		}
	}
	report := Report{ContractVersion: 1, ClassID: classID, LearnerUserID: learnerID, Grades: []Grade{}}
	rows, err := tx.Query(ctx, releasedGradesSQL, classID, learnerID)
	if err != nil {
		return Report{}, err
	}
	for rows.Next() {
		var grade Grade
		if err = rows.Scan(&grade.GradeItemID, &grade.Title, &grade.ValueMilli, &grade.GradeVersion, &grade.ReleaseID, &grade.ReleasedAt); err != nil {
			rows.Close()
			return Report{}, err
		}
		report.Grades = append(report.Grades, grade)
		if len(report.Grades) > MaxReportRows {
			rows.Close()
			return Report{}, ErrTooLarge
		}
		if report.Watermark == nil || grade.ReleasedAt.After(report.Watermark.ReleasedAt) || grade.ReleasedAt.Equal(report.Watermark.ReleasedAt) && grade.ReleaseID > report.Watermark.ReleaseID {
			report.Watermark = &Watermark{ReleaseID: grade.ReleaseID, ReleasedAt: grade.ReleasedAt}
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Report{}, err
	}
	return report, nil
}
