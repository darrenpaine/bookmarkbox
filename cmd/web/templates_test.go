package main

import (
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

func TestHumanDate(t *testing.T) {
	tests := []struct {
		name string
		tm   time.Time
		want string
	}{
		{
			name: "UTC",
			tm:   time.Date(2022, 3, 17, 10, 15, 0, 0, time.UTC),
			want: "17 Mar 2022 at 10:15",
		},
		{
			name: "Empty",
			tm:   time.Time{},
			want: "",
		},
		{
			name: "CET",
			tm:   time.Date(2022, 3, 17, 10, 15, 0, 0, time.FixedZone("CET", 1*60*60)),
			want: "17 Mar 2022 at 09:15",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hd := humanDate(tt.tm)
			assert.Equal(t, hd, tt.want)
		})
	}
}

func TestShortDate(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	assert.Equal(t, shortDate(time.Date(2026, 3, 6, 9, 0, 0, 0, time.UTC), now), "6 Mar")
	assert.Equal(t, shortDate(time.Date(2025, 12, 31, 9, 0, 0, 0, time.UTC), now), "31 Dec 2025")
	assert.Equal(t, shortDate(time.Time{}, now), "")
}

// The same name always gets the same one of the 8 colours.
func TestColour(t *testing.T) {
	assert.Equal(t, colour("go"), colour("go"))
	seen := map[string]bool{}
	for _, name := range []string{"go", "docs", "sqlite", "css", "htmx", "reading", "books", "self-hosting", "a", "b", "c", "d"} {
		c := colour(name)
		if len(c) != 2 || c[0] != 'c' || c[1] < '0' || c[1] > '7' {
			t.Errorf("colour(%q) = %q", name, c)
		}
		seen[c] = true
	}
	if len(seen) < 4 {
		t.Errorf("only %d colours used for 12 names", len(seen))
	}
}

func TestInitial(t *testing.T) {
	assert.Equal(t, initial("go.dev"), "G")
	assert.Equal(t, initial("ärzte.de"), "Ä")
	assert.Equal(t, initial(""), "?")
}
