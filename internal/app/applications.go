package app

import (
	"context"
	"flag"
	"io"

	"github.com/bprendie/weazlback/internal/inventory"
)

func applicationsCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("applications", flag.ContinueOnError)
	flags.SetOutput(stderr)
	output := flags.String("output", "", "write private application manifest")
	if err := flags.Parse(args); err != nil {
		return err
	}
	manifest, err := inventory.CaptureApplications(ctx)
	if err != nil {
		return err
	}
	if err := inventory.ValidateApplications(manifest); err != nil {
		return err
	}
	if *output != "" {
		if err := inventory.WriteApplications(*output, manifest); err != nil {
			return err
		}
	}
	return writeJSON(stdout, manifest)
}
