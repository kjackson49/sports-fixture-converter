package main

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
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
	var rrule string

	for _, line := range lines {
		switch {
		case line == "BEGIN:VEVENT":
			cur = &Fixture{}
			rrule = ""
		case line == "END:VEVENT":
			if cur != nil {
				if rrule == "" {
					fixtures = append(fixtures, *cur)
				} else {
					occurrences, err := expandRRULE(cur.Date, rrule)
					if err != nil {
						return nil, fmt.Errorf("expanding RRULE %q: %w", rrule, err)
					}
					for _, t := range occurrences {
						f := *cur
						f.Date = t
						fixtures = append(fixtures, f)
					}
				}
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
			case "RRULE":
				rrule = val
			}
		}
	}
	return fixtures, nil
}

// icsRRULEWeekday maps the two-letter BYDAY codes from RFC 5545 §3.3.10 to
// their time.Weekday. Ordinal prefixes (e.g. "1MO", "-1FR") aren't
// supported since they only mean something for monthly/yearly rules.
var icsRRULEWeekday = map[string]time.Weekday{
	"SU": time.Sunday, "MO": time.Monday, "TU": time.Tuesday, "WE": time.Wednesday,
	"TH": time.Thursday, "FR": time.Friday, "SA": time.Saturday,
}

// expandRRULE turns an RRULE value into the concrete occurrence times it
// describes, starting from dtstart. Only FREQ=DAILY and FREQ=WEEKLY are
// supported (optionally with BYDAY for weekly), which is what recurring
// fixtures actually use in practice — a match every N days, or on set
// weekdays each week. The rule must be bounded by COUNT or UNTIL; RFC 5545
// allows open-ended recurrence but we can't emit an infinite CSV.
func expandRRULE(dtstart time.Time, rrule string) ([]time.Time, error) {
	const maxOccurrences = 1000

	var freq string
	interval := 1
	count := 0
	var until time.Time
	hasUntil := false
	var byday []time.Weekday

	for _, part := range strings.Split(rrule, ";") {
		k, v, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		switch k {
		case "FREQ":
			freq = v
		case "INTERVAL":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid INTERVAL %q", v)
			}
			interval = n
		case "COUNT":
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 {
				return nil, fmt.Errorf("invalid COUNT %q", v)
			}
			count = n
		case "UNTIL":
			t, err := parseRRULEUntil(v)
			if err != nil {
				return nil, fmt.Errorf("invalid UNTIL %q: %w", v, err)
			}
			until = t
			hasUntil = true
		case "BYDAY":
			for _, d := range strings.Split(v, ",") {
				wd, ok := icsRRULEWeekday[d]
				if !ok {
					return nil, fmt.Errorf("unsupported BYDAY value %q", d)
				}
				byday = append(byday, wd)
			}
		}
	}

	switch freq {
	case "DAILY", "WEEKLY":
	case "":
		return nil, fmt.Errorf("RRULE is missing FREQ")
	default:
		return nil, fmt.Errorf("unsupported FREQ %q (only DAILY and WEEKLY are)", freq)
	}
	if freq == "DAILY" && len(byday) > 0 {
		return nil, fmt.Errorf("BYDAY is only supported with FREQ=WEEKLY")
	}
	if count == 0 && !hasUntil {
		return nil, fmt.Errorf("RRULE needs COUNT or UNTIL, open-ended recurrence isn't supported")
	}

	if len(byday) == 0 {
		step := 24 * time.Hour
		if freq == "WEEKLY" {
			step = 7 * 24 * time.Hour
		}
		var times []time.Time
		for t := dtstart; ; t = t.Add(step * time.Duration(interval)) {
			if hasUntil && t.After(until) {
				break
			}
			times = append(times, t)
			if count > 0 && len(times) >= count {
				break
			}
			if len(times) >= maxOccurrences {
				return nil, fmt.Errorf("RRULE expands past %d occurrences", maxOccurrences)
			}
		}
		return times, nil
	}

	// FREQ=WEEKLY with BYDAY: walk week by week from the Sunday on or
	// before dtstart, emitting one occurrence per matching weekday that
	// falls on or after dtstart.
	weekStart := dtstart.AddDate(0, 0, -int(dtstart.Weekday()))
	var times []time.Time
	for week := 0; ; week += interval {
		base := weekStart.AddDate(0, 0, week*7)
		for _, wd := range byday {
			occDay := base.AddDate(0, 0, int(wd))
			occ := time.Date(occDay.Year(), occDay.Month(), occDay.Day(),
				dtstart.Hour(), dtstart.Minute(), dtstart.Second(), 0, dtstart.Location())
			if occ.Before(dtstart) {
				continue
			}
			if hasUntil && occ.After(until) {
				continue
			}
			times = append(times, occ)
		}
		if len(times) >= maxOccurrences {
			return nil, fmt.Errorf("RRULE expands past %d occurrences", maxOccurrences)
		}
		if hasUntil && base.After(until) {
			break
		}
		if count > 0 && len(times) >= count {
			break
		}
	}

	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	if count > 0 && len(times) > count {
		times = times[:count]
	}
	return times, nil
}

// parseRRULEUntil parses an RRULE UNTIL value, which per RFC 5545 is
// either a UTC date-time (trailing Z) or, less commonly, a bare date.
func parseRRULEUntil(val string) (time.Time, error) {
	if strings.HasSuffix(val, "Z") {
		return time.Parse(icsUTCLayout, val)
	}
	if len(val) == 8 {
		return time.Parse("20060102", val)
	}
	return time.ParseInLocation(icsLocalLayout, val, time.Local)
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
