package submissions

import (
	"errors"
	"testing"
)

func TestCommandBoundary(t *testing.T) {
	valid := []Command{
		{Action: "create", ClassID: "c", ActivityID: "a", AssignmentRevisionID: "r"},
		{Action: "append_revision", ClassID: "c", AggregateID: "s", Version: 1, Body: "work"},
		{Action: "lock", ClassID: "c", AggregateID: "s", AttemptVersion: 1},
		{Action: "return", ClassID: "c", AggregateID: "s", AttemptVersion: 2},
		{Action: "resubmit", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 3},
		{Action: "handin", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 1},
	}
	for _, command := range valid {
		if _, err := normalize(command); err != nil {
			t.Fatalf("valid %s: %v", command.Action, err)
		}
	}
	invalid := []Command{
		{Action: "create", ClassID: "c", ActivityID: "a", AssignmentRevisionID: "r", Body: "client owner"},
		{Action: "append_revision", ClassID: "c", AggregateID: "s", Version: 0},
		{Action: "append_revision", ClassID: "c", AggregateID: "s", Version: 1, FileAssetIDs: []string{"x", "x"}},
		{Action: "lock", ClassID: "c", AggregateID: "s", AttemptVersion: 0},
		{Action: "resubmit", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 0},
		{Action: "handin", ClassID: "c", AggregateID: "s", Version: 0, AttemptVersion: 1},
	}
	for _, command := range invalid {
		if _, err := normalize(command); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid accepted: %+v", command)
		}
	}
}

func TestHandinBoundary(t *testing.T) {
	for _, command := range []Command{
		{Action: "handin", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 1, Body: "client clock"},
		{Action: "handin", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 1, FileAssetIDs: []string{"foreign"}},
		{Action: "handin", ClassID: "c", AggregateID: "s", Version: 1, AttemptVersion: 1, AssignmentRevisionID: "foreign"},
	} {
		if _, err := normalize(command); !errors.Is(err, ErrInvalid) {
			t.Fatalf("handin accepted untrusted data: %+v", command)
		}
	}
}
