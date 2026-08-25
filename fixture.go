package main

import "time"

// Fixture is one scheduled match. It's the common shape both formats get
// parsed into and written out of, so ics.go and csv.go never talk directly
// to each other.
type Fixture struct {
	Date        time.Time
	HomeTeam    string
	AwayTeam    string
	Venue       string
	Competition string
}
