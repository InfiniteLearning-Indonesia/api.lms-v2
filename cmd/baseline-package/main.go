// baseline-package reads explicitly supplied local files and prints an offline
// integrity report or restore preparation plan. It never executes a restore.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"lmsv3/internal/migration/backup"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "--verify-only" && os.Args[1] != "--plan-restore" && os.Args[1] != "--authenticate-only" && os.Args[1] != "--seal-only" && os.Args[1] != "--recover-only") {
		fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_OFFLINE_MODE_REQUIRED")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var result any
	valid := false
	if os.Args[1] == "--verify-only" {
		r, err := backup.DecodePackage(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_INVALID_INPUT")
			os.Exit(2)
		}
		p := backup.CheckPackage(ctx, r, time.Now().UTC())
		result = p
		valid = p.Status == "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED"
	} else if os.Args[1] == "--seal-only" {
		r, err := backup.DecodeSealPackageArtifact(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_INVALID_INPUT")
			os.Exit(2)
		}
		p, err := backup.SealPackageArtifact(ctx, r, time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_LOCAL_TRANSFER_REJECTED")
			os.Exit(2)
		}
		result = p
		valid = p.Status == "PASS_PROTECTED_ENCRYPTED_FILE_VERIFIED_ONLY"
	} else if os.Args[1] == "--recover-only" {
		r, err := backup.DecodeRecoverPackageArtifact(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_INVALID_INPUT")
			os.Exit(2)
		}
		p, err := backup.RecoverPackageArtifact(ctx, r, time.Now().UTC())
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_LOCAL_TRANSFER_REJECTED")
			os.Exit(2)
		}
		result = p
		valid = p.Status == "OFFLINE_ARTIFACT_RECOVERED_NOT_AUTHORIZED"
	} else if os.Args[1] == "--authenticate-only" {
		r, err := backup.DecodeAuthenticatedPackage(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_INVALID_INPUT")
			os.Exit(2)
		}
		p := backup.AuthenticatePackage(ctx, r, time.Now().UTC())
		result = p
		valid = p.Status == "OFFLINE_PACKAGE_AUTHENTICATED_NOT_AUTHORIZED"
	} else {
		r, err := backup.DecodeRestorePlan(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "BASELINE_PACKAGE_INVALID_INPUT")
			os.Exit(2)
		}
		p := backup.PlanRestore(ctx, r, time.Now().UTC())
		result = p
		valid = p.Status == "OFFLINE_RESTORE_PLAN_VALID_NOT_AUTHORIZED"
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil || !valid {
		os.Exit(2)
	}
}
