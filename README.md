# fixconv

Most amateur league and club sites publish their fixture list as an
iCalendar feed (`.ics`) meant for subscribing in a calendar app. That's
fine until you want to do anything else with the data: put it in a
spreadsheet, feed it into a script, diff two seasons against each other.
CSV is a lot easier to work with for that, and going back the other way
(CSV to ICS) is useful when the source data is a spreadsheet someone
maintains by hand but you want it to show up in a calendar app.

`fixconv` converts fixture lists between the two formats. It's a single
static binary, no network access, no dependencies.

## Usage

```
go build -o fixconv .

./fixconv -in season.ics -out season.csv
./fixconv -in season.csv -out season.ics
```

Direction is inferred from the file extensions, so `-in` and `-out` can
be either format in either order.

## CSV shape

```
date,time,home,away,venue,competition
2026-08-29,15:00,Riverside FC,Oak Park United,Riverside Ground,County League Div 1
2026-09-05,19:45,Oak Park United,Riverside FC,Memorial Stadium,County League Div 1
```

`date` is `YYYY-MM-DD`, `time` is 24-hour `HH:MM`. `venue` and
`competition` are optional; a row is valid with just the first four
columns.

## ICS mapping

Each fixture becomes one `VEVENT`. `SUMMARY` is `<home> v <away>`,
`LOCATION` is the venue, `CATEGORIES` carries the competition name.
Times are read and written in UTC (a trailing `Z` on `DTSTART`) — feeds
that use `TZID` or floating local time aren't handled yet.

## Current limits

- No line folding on parse: an ICS file with wrapped long lines (some
  calendar exports do this) won't parse correctly yet.
- No `RRULE` (recurring event) support — every fixture needs its own
  `VEVENT`.
- Timezone handling is UTC-only in both directions.

None of this is architecturally hard, it just hasn't been needed for
the calendars I've fed it so far.
