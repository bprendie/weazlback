package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bprendie/weazlback/internal/restic"
)

func restoreCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	snapshot := flags.String("snapshot", "latest", "restore point ID")
	target := flags.String("target", "", "private staging directory")
	var includes stringList
	flags.Var(&includes, "include", "path to restore; repeatable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *target == "" {
		return errors.New("--target is required")
	}
	_, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	if err := restic.NewService(stderr).Restore(ctx, repo, *snapshot, *target, includes); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "restore point %s restored into %s\n", *snapshot, *target)
	return err
}

func filesCommand(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("files", flag.ContinueOnError)
	flags.SetOutput(stderr)
	destinationID := flags.String("destination", "", "destination ID")
	snapshot := flags.String("snapshot", "latest", "restore point ID")
	query := flags.String("query", "", "case-insensitive path filter")
	if err := flags.Parse(args); err != nil {
		return err
	}
	_, destination, v, err := loadRuntime(*destinationID, stderr)
	if err != nil {
		return err
	}
	defer v.Lock()
	if err := authorizeDestination(*destination, stdout, stderr); err != nil {
		return err
	}
	repo, err := repositoryFrom(v, *destination)
	if err != nil {
		return err
	}
	files, err := restic.NewService(stderr).Files(ctx, repo, *snapshot)
	if err != nil {
		return err
	}
	if needle := strings.ToLower(strings.TrimSpace(*query)); needle != "" {
		filtered := files[:0]
		for _, file := range files {
			if strings.Contains(strings.ToLower(file.Path), needle) {
				filtered = append(filtered, file)
			}
		}
		files = filtered
	}
	return writeJSON(stdout, files)
}

type stringList []string

func (s *stringList) String() string         { return strings.Join(*s, ",") }
func (s *stringList) Set(value string) error { *s = append(*s, value); return nil }
