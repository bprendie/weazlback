package browserrepair

import "os"

type Family string

const (
	Chromium Family = "chromium"
	Mozilla  Family = "mozilla"
)

type Action string

const (
	Remove        Action = "remove"
	SkipLive      Action = "skip-live"
	SkipAmbiguous Action = "skip-ambiguous"
	SkipBoundary  Action = "skip-boundary"
)

type Entry struct {
	Family Family
	Action Action
	Root   string
	Path   string
}

type Plan struct {
	Entries []Entry
}

type Result struct {
	Removed, Live, Ambiguous, Boundary, Failed int
}

type ProcessChecker interface {
	Running(Family, int) bool
}

type Options struct {
	Home       string
	ConfigHome string
	UID        int
	Processes  ProcessChecker
	Lstat      func(string) (os.FileInfo, error)
	Remove     func(string) error
}

func (r Result) Clean() bool { return r.Failed == 0 }
