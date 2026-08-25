package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "fixconv:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("fixconv", flag.ContinueOnError)
	in := fs.String("in", "", "input file (.ics or .csv)")
	out := fs.String("out", "", "output file (.ics or .csv)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *in == "" || *out == "" {
		fs.Usage()
		return fmt.Errorf("both -in and -out are required")
	}

	inFile, err := os.Open(*in)
	if err != nil {
		return err
	}
	defer inFile.Close()

	var fixtures []Fixture
	switch ext := strings.ToLower(filepath.Ext(*in)); ext {
	case ".ics":
		fixtures, err = parseICS(inFile)
	case ".csv":
		fixtures, err = parseCSV(inFile)
	default:
		return fmt.Errorf("unrecognized input extension %q (want .ics or .csv)", ext)
	}
	if err != nil {
		return fmt.Errorf("reading %s: %w", *in, err)
	}

	outFile, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer outFile.Close()

	switch ext := strings.ToLower(filepath.Ext(*out)); ext {
	case ".ics":
		err = writeICS(outFile, fixtures)
	case ".csv":
		err = writeCSV(outFile, fixtures)
	default:
		return fmt.Errorf("unrecognized output extension %q (want .ics or .csv)", ext)
	}
	if err != nil {
		return fmt.Errorf("writing %s: %w", *out, err)
	}

	fmt.Printf("converted %d fixtures\n", len(fixtures))
	return nil
}
