package app

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/bprendie/weazlback/internal/browserrepair"
)

func browserCommand(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 || args[0] != "repair" {
		return fmt.Errorf("usage: weazlback browser repair [--apply] [--home PATH]")
	}
	flags := flag.NewFlagSet("browser repair", flag.ContinueOnError)
	flags.SetOutput(stderr)
	home, _ := os.UserHomeDir()
	target := flags.String("home", home, "target home directory")
	apply := flags.Bool("apply", false, "remove validated stale browser locks")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	options := browserrepair.Options{Home: *target, UID: os.Getuid(), Processes: browserrepair.ProcFS{}}
	plan := browserrepair.Detect(options)
	result := summarizeBrowserPlan(plan)
	if _, err := fmt.Fprintf(stdout, "browser repair plan: %d removable, %d live, %d ambiguous, %d boundary\n", countRemovable(plan), result.Live, result.Ambiguous, result.Boundary); err != nil {
		return err
	}
	if *apply {
		result = browserrepair.Apply(options, plan)
		_, err := fmt.Fprintf(stdout, "browser repair complete: %d removed, %d live, %d ambiguous, %d boundary, %d failed\n", result.Removed, result.Live, result.Ambiguous, result.Boundary, result.Failed)
		if result.Failed > 0 {
			return fmt.Errorf("browser repair incomplete: %d transient entries failed", result.Failed)
		}
		return err
	}
	return nil
}

func summarizeBrowserPlan(plan browserrepair.Plan) browserrepair.Result {
	var result browserrepair.Result
	for _, entry := range plan.Entries {
		switch entry.Action {
		case browserrepair.SkipLive:
			result.Live++
		case browserrepair.SkipAmbiguous:
			result.Ambiguous++
		case browserrepair.SkipBoundary:
			result.Boundary++
		}
	}
	return result
}

func countRemovable(plan browserrepair.Plan) int {
	count := 0
	for _, entry := range plan.Entries {
		if entry.Action == browserrepair.Remove {
			count++
		}
	}
	return count
}
