package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"time"
)

var csvHeader = []string{"date", "time", "home", "away", "venue", "competition"}

func parseCSV(r io.Reader) ([]Fixture, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	rows = rows[1:] // drop header

	fixtures := make([]Fixture, 0, len(rows))
	for i, row := range rows {
		if len(row) < 4 {
			return nil, fmt.Errorf("row %d: expected at least 4 columns (date,time,home,away), got %d", i+2, len(row))
		}
		t, err := time.Parse("2006-01-02 15:04", row[0]+" "+row[1])
		if err != nil {
			return nil, fmt.Errorf("row %d: parsing date/time: %w", i+2, err)
		}
		f := Fixture{Date: t, HomeTeam: row[2], AwayTeam: row[3]}
		if len(row) > 4 {
			f.Venue = row[4]
		}
		if len(row) > 5 {
			f.Competition = row[5]
		}
		fixtures = append(fixtures, f)
	}
	return fixtures, nil
}

func writeCSV(w io.Writer, fixtures []Fixture) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(csvHeader); err != nil {
		return err
	}
	for _, f := range fixtures {
		row := []string{
			f.Date.Format("2006-01-02"),
			f.Date.Format("15:04"),
			f.HomeTeam,
			f.AwayTeam,
			f.Venue,
			f.Competition,
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
