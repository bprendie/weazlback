package app

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestReadTuneValueAcceptsDefaultAndManualChoice(t *testing.T) {
	var output bytes.Buffer
	got, err := readTuneValue(bufio.NewReader(strings.NewReader("\n")), &output, "Connections", 4, 1, 64)
	if err != nil || got != 4 {
		t.Fatalf("default=%d err=%v", got, err)
	}
	output.Reset()
	got, err = readTuneValue(bufio.NewReader(strings.NewReader("10\n")), &output, "Connections", 4, 1, 64)
	if err != nil || got != 10 {
		t.Fatalf("manual=%d err=%v", got, err)
	}
}

func TestReadTuneValueRepromptsAfterInvalidChoice(t *testing.T) {
	var output bytes.Buffer
	got, err := readTuneValue(bufio.NewReader(strings.NewReader("100\n2\n")), &output, "Connections", 4, 1, 64)
	if err != nil || got != 2 {
		t.Fatalf("choice=%d err=%v", got, err)
	}
	if !strings.Contains(output.String(), "Enter a value from 1 to 64") {
		t.Fatalf("output=%q", output.String())
	}
}
