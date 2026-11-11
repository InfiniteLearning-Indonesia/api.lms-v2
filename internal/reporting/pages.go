package reporting

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
	"lmsv3/internal/identity"
)

const MaxReportRows = 200
const MaxReportBytes = 256 << 10

var ErrTooLarge = errors.New("REPORT_TOO_LARGE")
var ErrStale = errors.New("REPORT_STALE_SNAPSHOT")

type PageRequest struct {
	PageSize    int    `json:"page_size"`
	Cursor      string `json:"cursor"`
	Sort        string `json:"sort"`
	GradeItemID string `json:"grade_item_id"`
}
type ExportRequest struct {
	Format      string `json:"format"`
	Sort        string `json:"sort"`
	GradeItemID string `json:"grade_item_id"`
}
type Page struct {
	Report
	Snapshot   string `json:"snapshot"`
	NextCursor string `json:"next_cursor"`
}
type pageCursor struct {
	Session, Class, Learner, Filter, Sort, Snapshot, LastID string
	Size                                                    int
	Expires                                                 int64
}
type Export struct {
	Data              []byte
	Snapshot, AuditID string
}

func normalizeSort(value string) (string, error) {
	if value == "" {
		return "grade_item_id_asc", nil
	}
	if value != "grade_item_id_asc" {
		return "", ErrInvalid
	}
	return value, nil
}
func validFilter(value string) bool { return value == "" || identifier.MatchString(value) }
func reportDigest(report Report) string {
	b, _ := json.Marshal(report)
	digest := sha256.Sum256(b)
	return hex.EncodeToString(digest[:])
}
func boundedJSON(value any) ([]byte, error) {
	b, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxReportBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}
func filtered(report Report, filter string) (Report, error) {
	if len(report.Grades) > MaxReportRows {
		return Report{}, ErrTooLarge
	}
	if !validFilter(filter) {
		return Report{}, ErrInvalid
	}
	result := report
	result.Grades = []Grade{}
	for _, grade := range report.Grades {
		if filter == "" || grade.GradeItemID == filter {
			result.Grades = append(result.Grades, grade)
		}
	}
	sort.Slice(result.Grades, func(i, j int) bool { return result.Grades[i].GradeItemID < result.Grades[j].GradeItemID })
	return result, nil
}
func (s Store) signCursor(value pageCursor) string {
	b, _ := json.Marshal(value)
	mac := hmac.New(sha256.New, s.cursorKey[:])
	mac.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s Store) decodeCursor(value string) (pageCursor, error) {
	var cursor pageCursor
	if len(value) > 2048 || s.cursorKey == [32]byte{} {
		return cursor, ErrInvalid
	}
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return cursor, ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return cursor, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return cursor, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.cursorKey[:])
	mac.Write(b)
	if !hmac.Equal(signature, mac.Sum(nil)) || json.Unmarshal(b, &cursor) != nil {
		return cursor, ErrInvalid
	}
	return cursor, nil
}
func (s Store) pageReport(report Report, session string, request PageRequest, now time.Time) (Page, error) {
	order, err := normalizeSort(request.Sort)
	if err != nil || request.PageSize < 1 || request.PageSize > 50 || !validFilter(request.GradeItemID) || s.cursorKey == [32]byte{} {
		return Page{}, ErrInvalid
	}
	selected, err := filtered(report, request.GradeItemID)
	if err != nil {
		return Page{}, err
	}
	snapshot := reportDigest(report)
	last := ""
	expires := now.Add(10 * time.Minute).Unix()
	if request.Cursor != "" {
		cursor, e := s.decodeCursor(request.Cursor)
		if e != nil {
			return Page{}, e
		}
		if cursor.Session != identity.TokenHash(session) || cursor.Class != report.ClassID || cursor.Learner != report.LearnerUserID {
			return Page{}, ErrForbidden
		}
		if cursor.Size != request.PageSize || cursor.Filter != request.GradeItemID || cursor.Sort != order {
			return Page{}, ErrInvalid
		}
		if cursor.Expires <= now.Unix() || cursor.Snapshot != snapshot {
			return Page{}, ErrStale
		}
		last, expires = cursor.LastID, cursor.Expires
	}
	remaining := []Grade{}
	for _, grade := range selected.Grades {
		if grade.GradeItemID > last {
			remaining = append(remaining, grade)
		}
	}
	page := Page{Report: selected, Snapshot: snapshot}
	page.Grades = remaining
	if len(remaining) > request.PageSize {
		page.Grades = remaining[:request.PageSize]
		page.NextCursor = s.signCursor(pageCursor{Session: identity.TokenHash(session), Class: report.ClassID, Learner: report.LearnerUserID,
			Filter: request.GradeItemID, Sort: order, Snapshot: snapshot, LastID: page.Grades[len(page.Grades)-1].GradeItemID, Size: request.PageSize, Expires: expires})
	}
	if _, err = boundedJSON(page); err != nil {
		return Page{}, err
	}
	return page, nil
}
func (s Store) ReadPage(ctx context.Context, session, classID, learnerID string, request PageRequest) (Page, error) {
	if request.PageSize < 1 || request.PageSize > 50 || len(request.Cursor) > 2048 || !validFilter(request.GradeItemID) {
		return Page{}, ErrInvalid
	}
	if _, err := normalizeSort(request.Sort); err != nil {
		return Page{}, err
	}
	report, err := s.ReleasedGrades(ctx, session, classID, learnerID)
	if err != nil {
		return Page{}, err
	}
	return s.pageReport(report, session, request, time.Now())
}
func safeCell(value string) string {
	trimmed := strings.TrimLeftFunc(value, unicode.IsSpace)
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) || strings.HasPrefix(value, "\t") || strings.HasPrefix(value, "\r") || strings.HasPrefix(value, "\n") {
		return "'" + value
	}
	return value
}
func encodeExport(report Report, request ExportRequest) ([]byte, error) {
	if request.Format != "csv" {
		return nil, ErrInvalid
	}
	if _, err := normalizeSort(request.Sort); err != nil {
		return nil, err
	}
	selected, err := filtered(report, request.GradeItemID)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	if err = writer.Write([]string{"class_id", "learner_user_id", "grade_item_id", "title", "value_milli", "grade_version", "release_id", "released_at"}); err != nil {
		return nil, err
	}
	for _, grade := range selected.Grades {
		err = writer.Write([]string{report.ClassID, report.LearnerUserID, grade.GradeItemID, safeCell(grade.Title), strconv.FormatInt(grade.ValueMilli, 10), strconv.FormatInt(grade.GradeVersion, 10), grade.ReleaseID, grade.ReleasedAt.Format(time.RFC3339Nano)})
		if err != nil {
			return nil, err
		}
		writer.Flush()
		if writer.Error() != nil {
			return nil, writer.Error()
		}
		if buffer.Len() > MaxReportBytes {
			return nil, ErrTooLarge
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		return nil, writer.Error()
	}
	return buffer.Bytes(), nil
}
func (s Store) ExportCSV(ctx context.Context, session, classID, learnerID string, request ExportRequest) (Export, error) {
	if s.Pool == nil {
		return Export{}, ErrInvalid
	}
	tx, err := s.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)
	report, err := s.releasedGradesTx(ctx, tx, session, classID, learnerID)
	if err != nil {
		return Export{}, err
	}
	data, err := encodeExport(report, request)
	if err != nil {
		return Export{}, err
	}
	actor, err := identity.ResolveBridge(ctx, tx, session)
	if err != nil {
		return Export{}, err
	}
	id, err := identity.RandomToken()
	if err != nil {
		return Export{}, err
	}
	snapshot := reportDigest(report)
	_, err = tx.Exec(ctx, `INSERT INTO v3.audit(id,actor_id,class_id,action,request_id) VALUES($1,$2,$3,$4,$1)`, id, actor.ID, classID, "reporting.grades.export/"+learnerID+"/"+snapshot)
	if err != nil {
		return Export{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO v3.outbox(event_id,topic) VALUES($1,'reporting.audit.v1')`, id)
	if err != nil {
		return Export{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Export{}, err
	}
	return Export{Data: data, Snapshot: snapshot, AuditID: id}, nil
}
