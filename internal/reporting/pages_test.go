package reporting

import (
	"encoding/csv"
	"errors"
	"strings"
	"testing"
	"time"
)

func sampleReport() Report {
	return Report{ContractVersion: 1, ClassID: strings.Repeat("a", 32), LearnerUserID: strings.Repeat("b", 32), Grades: []Grade{
		{GradeItemID: strings.Repeat("1", 32), Title: "=HYPERLINK(\"bad\")", ValueMilli: 70000, GradeVersion: 1, ReleaseID: strings.Repeat("c", 32)},
		{GradeItemID: strings.Repeat("2", 32), Title: "ordinary", ValueMilli: 80000, GradeVersion: 1, ReleaseID: strings.Repeat("d", 32)},
	}}
}

func TestReportPageCursorSnapshot(t *testing.T) {
	store := Store{cursorKey: [32]byte{1, 2, 3}}
	report := sampleReport()
	now := time.Unix(1700000000, 0)
	request := PageRequest{PageSize: 1}
	first, err := store.pageReport(report, "session-a", request, now)
	if err != nil || len(first.Grades) != 1 || first.NextCursor == "" {
		t.Fatal("first page", err)
	}
	request.Cursor = first.NextCursor
	second, err := store.pageReport(report, "session-a", request, now)
	if err != nil || len(second.Grades) != 1 || second.Grades[0].GradeItemID == first.Grades[0].GradeItemID || second.NextCursor != "" || second.Snapshot != first.Snapshot {
		t.Fatal("stable continuation", err)
	}
	if _, err = store.pageReport(report, "session-b", request, now); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign session accepted", err)
	}
	foreign := report
	foreign.ClassID = strings.Repeat("e", 32)
	if _, err = store.pageReport(foreign, "session-a", request, now); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign class accepted", err)
	}
	foreign = report
	foreign.LearnerUserID = strings.Repeat("f", 32)
	if _, err = store.pageReport(foreign, "session-a", request, now); !errors.Is(err, ErrForbidden) {
		t.Fatal("foreign learner accepted", err)
	}
	if _, err = store.pageReport(report, "session-a", request, now.Add(11*time.Minute)); !errors.Is(err, ErrStale) {
		t.Fatal("expired cursor accepted", err)
	}
	changed := sampleReport()
	changed.Grades[0].ValueMilli++
	if _, err = store.pageReport(changed, "session-a", request, now); !errors.Is(err, ErrStale) {
		t.Fatal("changed snapshot accepted", err)
	}
	for _, bad := range []PageRequest{{PageSize: 0}, {PageSize: 51}, {PageSize: 1, Sort: "title"}, {PageSize: 1, GradeItemID: "invalid"}, {PageSize: 1, Cursor: "x" + request.Cursor[1:]}, {PageSize: 2, Cursor: request.Cursor}, {PageSize: 1, Cursor: request.Cursor, GradeItemID: report.Grades[0].GradeItemID}} {
		if _, err = store.pageReport(report, "session-a", bad, now); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid page accepted", bad, err)
		}
	}
	report.Grades = make([]Grade, MaxReportRows+1)
	if _, err = store.pageReport(report, "session-a", PageRequest{PageSize: 1}, now); !errors.Is(err, ErrTooLarge) {
		t.Fatal("row limit bypassed", err)
	}
}

func TestReportExportEncodingBounds(t *testing.T) {
	report := sampleReport()
	data, err := encodeExport(report, ExportRequest{Format: "csv"})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(rows) != 3 || len(rows[0]) != 8 || rows[1][3] != "'"+report.Grades[0].Title || rows[1][4] != "70000" {
		t.Fatal("unsafe export", rows, err)
	}
	for _, value := range []string{"=SUM(1,2)", " +SUM(1,2)", "-cmd", "@cmd", "\tcmd", "\n=SUM(1)", "\r=SUM(1)"} {
		if !strings.HasPrefix(safeCell(value), "'") {
			t.Fatal("formula prefix not escaped")
		}
	}
	filtered, err := encodeExport(report, ExportRequest{Format: "csv", GradeItemID: report.Grades[1].GradeItemID})
	if err != nil || strings.Contains(string(filtered), "HYPERLINK") {
		t.Fatal("filter failed", err)
	}
	if _, err = encodeExport(report, ExportRequest{Format: "xlsx"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("unsupported format", err)
	}
	report.Grades[0].Title = strings.Repeat("x", MaxReportBytes+1)
	if _, err = encodeExport(report, ExportRequest{Format: "csv"}); !errors.Is(err, ErrTooLarge) {
		t.Fatal("byte limit bypassed", err)
	}
	report.Grades = make([]Grade, MaxReportRows+1)
	if _, err = encodeExport(report, ExportRequest{Format: "csv"}); !errors.Is(err, ErrTooLarge) {
		t.Fatal("row limit bypassed", err)
	}
}
