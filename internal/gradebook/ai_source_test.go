package gradebook

import "testing"

func TestNormalizeGradeAISourceRequiresHumanReason(t *testing.T) {
	base := GradeCommand{ClassID: "class", GradeItemID: "item", AggregateID: "aggregate", SubmissionRevisionID: "revision", ExpectedVersion: 0, ValueMilli: 81000}
	legacy, err := normalizeGrade(base)
	if err != nil || legacy.Reason != "INITIAL" {
		t.Fatal("ordinary initial grade changed", legacy, err)
	}
	base.AISuggestionID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if _, err := normalizeGrade(base); err == nil {
		t.Fatal("AI-linked grade accepted without human reason")
	}
	base.Reason = "Teacher reviewed the evidence and overrode the suggested value"
	linked, err := normalizeGrade(base)
	if err != nil || linked.Reason != base.Reason || linked.AISuggestionID != base.AISuggestionID {
		t.Fatal("human-reviewed grade rejected", linked, err)
	}
	base.AISuggestionID = "foreign-id"
	if _, err := normalizeGrade(base); err == nil {
		t.Fatal("malformed suggestion ID accepted")
	}
}
