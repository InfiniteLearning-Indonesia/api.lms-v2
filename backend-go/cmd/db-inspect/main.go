// db-inspect is an explicit operator CLI; never invoked by API startup or CI.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"lmsv3/internal/platform/dbinspect"
)

func main() {
	path := flag.String("env-file", "", "absolute path to explicitly selected staging env")
	ack := flag.Bool("staging-readonly", false, "confirm user-selected staging metadata inspection, no row reads or mutations")
	flag.Parse()
	if !*ack {
		fmt.Fprintln(os.Stderr, "STAGING_READONLY_ACK_REQUIRED")
		os.Exit(2)
	}
	s, err := dbinspect.Load(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	r := dbinspect.Inspect(context.Background(), s)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(r); err != nil {
		os.Exit(1)
	}
	if r.Status != "READ_ONLY_OBSERVATION_COMPLETE" {
		os.Exit(2)
	}
}
