// legacy-core-import plans or applies the pinned legacy core to a private rehearsal.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"lmsv3/internal/migration/legacycore"
)

func run() error {
	snapshot := flag.String("snapshot", "", "reviewed local SQL snapshot")
	proof := flag.String("target-proof", "", "isolated target receipt (empty restores proof and marker)")
	rollback := flag.Bool("prove-rollback", false, "roll back a fresh local rehearsal transaction")
	flag.Parse()
	if *snapshot == "" || flag.NArg() != 0 || (*rollback && *proof == "") {
		return legacycore.ErrSource
	}
	file, err := os.Open(*snapshot)
	if err != nil {
		return legacycore.ErrSource
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, legacycore.SnapshotSize+1))
	if err != nil {
		return legacycore.ErrSource
	}
	plan, err := legacycore.Build(raw)
	if err != nil {
		return err
	}
	status := "PASS_GO_DRY_RUN_ONLY"
	if *proof != "" {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		status, err = legacycore.Apply(ctx, plan, *proof, os.Getenv("LMS_CORE_REHEARSAL_PASSWORD"), *rollback)
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"status": status, "source_snapshot": legacycore.Snapshot, "mapping": legacycore.Mapping,
		"plan_sha256": plan.Digest(), "counts": plan.Counts,
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
}
