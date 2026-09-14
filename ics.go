package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	icsUTCLayout   = "20060102T150405Z"
	icsLocalLayout = "20060102T150405"
)

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
			key, params, val, ok := parseICSLine(line)
			if !ok {
				continue
			}
			switch key {
			case "DTSTART":
				t, err := parseICSDateTime(val, params["TZID"])
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

// parseICSLine handles both "KEY:VALUE" and "KEY;PARAM=x;PARAM2=y:VALUE"
// forms, returning the bare key and any parameters (TZID and friends).
func parseICSLine(line string) (key string, params map[string]string, val string, ok bool) {
	colon := strings.Index(line, ":")
	if colon < 0 {
		return "", nil, "", false
	}
	head, val := line[:colon], line[colon+1:]
	parts := strings.Split(head, ";")
	key = parts[0]
	if len(parts) > 1 {
		params = make(map[string]string, len(parts)-1)
		for _, p := range parts[1:] {
			if k, v, ok := strings.Cut(p, "="); ok {
				params[k] = v
			}
		}
	}
	return key, params, val, true
}

// parseICSDateTime parses a DTSTART value per RFC 5545 §3.3.5: a trailing
// Z means UTC, a TZID parameter means the value is local wall-clock time in
// that IANA zone, and neither means "floating" time with no assigned zone,
// which we interpret as local time on this machine since that's the closest
// stand-in for "whatever timezone the viewer is in".
func parseICSDateTime(val, tzid string) (time.Time, error) {
	if strings.HasSuffix(val, "Z") {
		return time.Parse(icsUTCLayout, val)
	}
	if tzid != "" {
		loc, err := time.LoadLocation(tzid)
		if err != nil {
			return time.Time{}, fmt.Errorf("loading TZID %q: %w", tzid, err)
		}
		return time.ParseInLocation(icsLocalLayout, val, loc)
	}
	return time.ParseInLocation(icsLocalLayout, val, time.Local)
}

// formatICSDateTime is the inverse of parseICSDateTime: UTC times get the
// trailing-Z form, times in a named IANA zone get a TZID parameter, and
// anything else (e.g. the unnamed time.Local zone) falls back to UTC so the
// written value is never ambiguous. Note this doesn't emit a VTIMEZONE
// block, so a TZID here relies on the reader recognizing the zone name
// itself rather than the file being fully self-contained per RFC 5545.
func formatICSDateTime(t time.Time) (key, val string) {
	if loc := t.Location(); loc != time.UTC {
		if name := loc.String(); name != "" && name != "Local" {
			return "DTSTART;TZID=" + name, t.Format(icsLocalLayout)
		}
	}
	return "DTSTART", t.UTC().Format(icsUTCLayout)
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
		dtKey, dtVal := formatICSDateTime(f.Date)
		fmt.Fprintf(bw, "%s:%s\n", dtKey, dtVal)
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
