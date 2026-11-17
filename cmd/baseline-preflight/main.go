// baseline-preflight validates declarations and local capacity only. No env
// loading, database connection, export/import, approval or destination creation.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"lmsv3/internal/migration/backup"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "--plan-only" {
		fmt.Fprintln(os.Stderr, "BASELINE_PREFLIGHT_PLAN_ONLY_REQUIRED")
		os.Exit(2)
	}
	request, err := backup.DecodeIntake(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "BASELINE_PREFLIGHT_INVALID_INPUT")
		os.Exit(2)
	}
	report := backup.CheckIntake(request, time.Now().UTC())
	if json.NewEncoder(os.Stdout).Encode(report) != nil {
		os.Exit(2)
	}
	if report.Status != "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED" {
		os.Exit(2)
	}
}
