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
	first, err := ID()
	if err != nil || !idPattern.MatchString(first) {
		t.Fatal("unsafe ID")
	}
	second, _ := ID()
	if first == second {
		t.Fatal("duplicate ID")
	}
}
