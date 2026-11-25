package datadis

import (
	"testing"
	"time"
)

func TestParseMeasurementTime(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skipf("Europe/Madrid time zone not available: %v", err)
	}

	tests := []struct {
		name       string
		date       string
		label      string
		occurrence int
		want       string // RFC 3339, UTC
		wantErr    bool
	}{
		{name: "first hour", date: "2026/11/19", label: "01:00", occurrence: 1, want: "2026-11-18T23:00:00Z"},
		{name: "afternoon", date: "2026/11/19", label: "17:00", occurrence: 1, want: "2026-11-19T15:00:00Z"},
		{name: "last hour is 24:00", date: "2026/11/19", label: "24:00", occurrence: 1, want: "2026-11-19T22:00:00Z"},
		{name: "bare hour", date: "2026/11/19", label: "17", occurrence: 1, want: "2026-11-19T15:00:00Z"},
		{name: "summer time", date: "2026/07/01", label: "13:00", occurrence: 1, want: "2026-07-01T10:00:00Z"},
		{name: "month end rolls over", date: "2026/11/30", label: "24:00", occurrence: 1, want: "2026-11-30T22:00:00Z"},
		{name: "year end rolls over", date: "2026/12/31", label: "24:00", occurrence: 1, want: "2026-12-31T22:00:00Z"},

		// Clocks go forward on 2026-03-29: 02:00 CET becomes 03:00 CEST.
		{name: "spring forward 01:00", date: "2026/03/29", label: "01:00", occurrence: 1, want: "2026-03-28T23:00:00Z"},
		{name: "spring forward 03:00", date: "2026/03/29", label: "03:00", occurrence: 1, want: "2026-03-29T00:00:00Z"},
		{name: "spring forward 04:00", date: "2026/03/29", label: "04:00", occurrence: 1, want: "2026-03-29T01:00:00Z"},

		// Clocks go back on 2026-10-25: 03:00 CEST becomes 02:00 CET, so
		// 02:00 happens twice.
		{name: "fall back 01:00", date: "2026/10/25", label: "01:00", occurrence: 1, want: "2026-10-24T22:00:00Z"},
		{name: "fall back first 02:00", date: "2026/10/25", label: "02:00", occurrence: 1, want: "2026-10-24T23:00:00Z"},
		{name: "fall back second 02:00", date: "2026/10/25", label: "02:00", occurrence: 2, want: "2026-10-25T00:00:00Z"},
		{name: "fall back 03:00", date: "2026/10/25", label: "03:00", occurrence: 1, want: "2026-10-25T01:00:00Z"},
		{name: "fall back third 02:00", date: "2026/10/25", label: "02:00", occurrence: 3, wantErr: true},
		{name: "repeated label on normal day", date: "2026/11/19", label: "17:00", occurrence: 2, wantErr: true},

		{name: "the bug: HH:MM:SS", date: "2026/11/19", label: "17:00:00", occurrence: 1, wantErr: true},
		{name: "hour 25", date: "2026/11/19", label: "25:00", occurrence: 1, wantErr: true},
		{name: "24 with minutes", date: "2026/11/19", label: "24:30", occurrence: 1, wantErr: true},
		{name: "empty time", date: "2026/11/19", label: "", occurrence: 1, wantErr: true},
		{name: "bad date", date: "2026-11-19", label: "17:00", occurrence: 1, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseMeasurementTime(tt.date, tt.label, tt.occurrence, madrid)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got %s", got.UTC().Format(time.RFC3339))
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if g := got.UTC().Format(time.RFC3339); g != tt.want {
				t.Errorf("got %s, want %s", g, tt.want)
			}
		})
	}
}

// A fall-back day has 25 hourly measurements; they must map to 25 distinct,
// consecutive hours.
func TestParseMeasurementTimeFallBackDay(t *testing.T) {
	madrid, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		t.Skipf("Europe/Madrid time zone not available: %v", err)
	}

	labels := []string{"01:00", "02:00", "02:00"}
	for h := 3; h <= 24; h++ {
		labels = append(labels, time.Date(0, 1, 1, h%24, 0, 0, 0, time.UTC).Format("15:04"))
	}
	labels[len(labels)-1] = "24:00"

	seen := map[string]int{}
	var prev time.Time
	for i, label := range labels {
		seen[label]++
		got, err := parseMeasurementTime("2026/10/25", label, seen[label], madrid)
		if err != nil {
			t.Fatalf("label %d (%s): %v", i, label, err)
		}
		if i > 0 && got.Sub(prev) != time.Hour {
			t.Fatalf("label %d (%s): %s is not one hour after %s", i, label, got.UTC(), prev.UTC())
		}
		prev = got
	}
	if len(labels) != 25 {
		t.Fatalf("expected 25 labels, got %d", len(labels))
	}
}

func TestHourRange(t *testing.T) {
	tests := map[string]string{
		"01:00": "00:00–01:00",
		"17:00": "16:00–17:00",
		"24:00": "23:00–24:00",
		"17":    "16:00–17:00",
		"00:00": "00:00",
		"bogus": "bogus",
		"":      "",
	}
	for in, want := range tests {
		if got := HourRange(in); got != want {
			t.Errorf("HourRange(%q) = %q, want %q", in, got, want)
		}
	}
}
