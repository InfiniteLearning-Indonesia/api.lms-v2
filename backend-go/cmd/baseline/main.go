// baseline performs a local read-only declaration inventory. It never connects
// to a database; exported values are sanitized by internal/baseline.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"lmsv3/internal/baseline"
)

func main() {
	workspace := flag.String("workspace", "", "absolute workspace containing backend-live and backend")
	flag.Parse()
	if !filepath.IsAbs(*workspace) {
		fmt.Fprintln(os.Stderr, "absolute --workspace is required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	report, err := baseline.Collect(ctx, *workspace)
	if err != nil {
		fmt.Fprintln(os.Stderr, "baseline inventory failed; no database operation performed")
		os.Exit(1)
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		os.Exit(1)
	}
}
