package classes

import (
	"strings"
	"testing"
)

func TestClassInputBoundary(t *testing.T) {
	if validate(Command{Action: "create", Name: "Name only"}) != nil {
		t.Fatal("name-only class rejected")
	}
	for _, cmd := range []Command{{Action: "create", Name: " "}, {Action: "create", Name: strings.Repeat("x", 201)}, {Action: "delete"}, {Action: "add", ClassID: strings.Repeat("a", 32), UserID: strings.Repeat("b", 32), Role: "admin"}, {Action: "edit", ClassID: strings.Repeat("a", 32), Name: "valid", Version: 0}} {
		if validate(cmd) == nil {
			t.Fatal("invalid command accepted")
		}
	}
	classID, userID, otherID := strings.Repeat("a", 32), strings.Repeat("b", 32), strings.Repeat("c", 32)
	valid := []Command{
		{Action: "create_group", ClassID: classID, Name: "Group A"},
		{Action: "assign_group", ClassID: classID, GroupID: otherID, UserID: userID},
		{Action: "assign_mentor", ClassID: classID, UserID: userID, MentorID: otherID},
		{Action: "invite", ClassID: classID, UserID: userID, Role: "student"},
		{Action: "accept_invitation", ClassID: classID, InvitationID: otherID},
		{Action: "start_episode", ClassID: classID, UserID: userID, EpisodeKind: "RETAKE", OriginEpisode: otherID, Policy: "SEPARATE_HISTORY"},
	}
	for _, cmd := range valid {
		if validate(cmd) != nil || !participationAction(cmd.Action) {
			t.Fatalf("valid participation command rejected: %s", cmd.Action)
		}
	}
	for _, cmd := range []Command{
		{Action: "create_group", ClassID: classID, Name: " "},
		{Action: "assign_mentor", ClassID: classID, UserID: userID, MentorID: userID},
		{Action: "invite", ClassID: classID, UserID: userID, Role: "owner"},
		{Action: "start_episode", ClassID: classID, UserID: userID, EpisodeKind: "TRANSFER", OriginEpisode: otherID, Policy: "SEPARATE_HISTORY"},
	} {
		if validate(cmd) == nil {
			t.Fatalf("invalid participation command accepted: %s", cmd.Action)
		}
	}
	first, err := ID()
	if err != nil || !idPattern.MatchString(first) {
		t.Fatal("unsafe ID")
	}
	second, _ := ID()
	if first == second {
		t.Fatal("duplicate ID")
	}
}
