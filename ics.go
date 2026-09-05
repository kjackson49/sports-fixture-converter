package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// icsTimeLayout assumes UTC (a trailing Z), which is what most published
// fixture calendars use. Local/floating times are a roadmap item.
const icsTimeLayout = "20060102T150405Z"

func parseICS(r io.Reader) ([]Fixture, error) {
	lines, err := unfoldICS(r)
	if err != nil {
		return nil, err
	}

	var fixtures []Fixture
	var cur *Fixture

	for _, line := range lines {
		switch {
		case line == "BEGIN:VEVENT":
			cur = &Fixture{}
		case line == "END:VEVENT":
			if cur != nil {
				fixtures = append(fixtures, *cur)
				cur = nil
			}
		case cur != nil:
			key, val, ok := splitICSLine(line)
			if !ok {
				continue
			}
			switch key {
			case "DTSTART":
				t, err := time.Parse(icsTimeLayout, val)
				if err != nil {
					return nil, fmt.Errorf("parsing DTSTART %q: %w", val, err)
				}
				cur.Date = t
			case "SUMMARY":
				if home, away, ok := strings.Cut(unescapeICS(val), " v "); ok {
					cur.HomeTeam = home
					cur.AwayTeam = away
				}
			case "LOCATION":
				cur.Venue = unescapeICS(val)
			case "CATEGORIES":
				cur.Competition = unescapeICS(val)
			}
		}
	}
	return fixtures, nil
}

// unfoldICS rejoins folded content lines. RFC 5545 lets a generator split
// any line by inserting a CRLF followed by a single space or tab; readers
// are required to undo that by dropping the CRLF and the leading whitespace
// before parsing the line as KEY;PARAMS:VALUE.
func unfoldICS(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	var lines []string
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if len(line) > 0 && (line[0] == ' ' || line[0] == '\t') && len(lines) > 0 {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return lines, nil
}

// splitICSLine handles both "KEY:VALUE" and "KEY;PARAM=x:VALUE" forms.
// Parameters (TZID and friends) are dropped rather than interpreted.
func splitICSLine(line string) (key, val string, ok bool) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", "", false
	}
	key, val = line[:colon], line[colon+1:]
	if semi := strings.Index(key, ";"); semi >= 0 {
		key = key[:semi]
	}
	return key, val, true
}

func unescapeICS(s string) string {
	r := strings.NewReplacer(`\,`, ",", `\;`, ";", `\n`, " ", `\N`, " ", `\\`, `\`)
	return r.Replace(s)
}

func escapeICS(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `,`, `\,`, `;`, `\;`, "\n", `\n`)
	return r.Replace(s)
}

func writeICS(w io.Writer, fixtures []Fixture) error {
	bw := bufio.NewWriter(w)
	fmt.Fprintln(bw, "BEGIN:VCALENDAR")
	fmt.Fprintln(bw, "VERSION:2.0")
	fmt.Fprintln(bw, "PRODID:-//fixconv//EN")
	for i, f := range fixtures {
		fmt.Fprintln(bw, "BEGIN:VEVENT")
		fmt.Fprintf(bw, "UID:fixconv-%d@local\n", i)
		fmt.Fprintf(bw, "DTSTART:%s\n", f.Date.UTC().Format(icsTimeLayout))
		fmt.Fprintf(bw, "SUMMARY:%s\n", escapeICS(f.HomeTeam+" v "+f.AwayTeam))
		if f.Venue != "" {
			fmt.Fprintf(bw, "LOCATION:%s\n", escapeICS(f.Venue))
		}
		if f.Competition != "" {
			fmt.Fprintf(bw, "CATEGORIES:%s\n", escapeICS(f.Competition))
		}
		fmt.Fprintln(bw, "END:VEVENT")
	}
	fmt.Fprintln(bw, "END:VCALENDAR")
	return bw.Flush()
}
