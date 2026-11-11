// report-load is a fixed local-synthetic QA client, never a production load tool.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"lmsv3/internal/reportbench"
	"os"
	"time"
)

func main() {
	if os.Getenv("QA_PG_ACK") != "new-local-synthetic-cluster" {
		fmt.Fprintln(os.Stderr, "SYNTHETIC_ACK_REQUIRED")
		os.Exit(1)
	}
	c, e := reportbench.Decode(os.Stdin)
	if e != nil {
		fmt.Fprintln(os.Stderr, "SYNTHETIC_CONFIG_REJECTED")
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	r, e := reportbench.Run(ctx, c)
	if e != nil {
		fmt.Fprintln(os.Stderr, "SYNTHETIC_LOAD_FAILED")
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
