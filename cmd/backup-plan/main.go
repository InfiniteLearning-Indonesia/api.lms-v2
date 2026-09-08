// Offline stdin JSON planner. No env loading, external process or DB connection.
package main

import (
	"encoding/json"
	"fmt"
	"lmsv3/internal/migration/backup"
	"os"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--plan-only" {
		fmt.Fprintln(os.Stderr, "explicit --plan-only required; no operation performed")
		os.Exit(2)
	}
	request, err := backup.Decode(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid offline plan; no operation performed")
		os.Exit(2)
	}
	plan := backup.Build(request)
	_ = json.NewEncoder(os.Stdout).Encode(plan)
	if plan.Status != "PLAN_VALID_NOT_AUTHORIZED" {
		os.Exit(2)
	}
}
