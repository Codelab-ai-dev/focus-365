package finance

import (
	"testing"
	"time"
)

func d(y int, m time.Month, day int) time.Time {
	return time.Date(y, m, day, 0, 0, 0, 0, time.UTC)
}

func TestPaydayAjustaFinDeSemana(t *testing.T) {
	// 31 may 2026 cae domingo → retrocede al viernes 29.
	if got := payday(2026, time.May); !got.Equal(d(2026, time.May, 29)) {
		t.Errorf("payday(may) = %v, want 2026-05-29", got)
	}
	// 28 feb 2026 cae sábado → retrocede al viernes 27.
	if got := payday(2026, time.February); !got.Equal(d(2026, time.February, 27)) {
		t.Errorf("payday(feb) = %v, want 2026-02-27", got)
	}
	// 30 jun 2026 cae martes → sin ajuste.
	if got := payday(2026, time.June); !got.Equal(d(2026, time.June, 30)) {
		t.Errorf("payday(jun) = %v, want 2026-06-30", got)
	}
}

func TestCycle(t *testing.T) {
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"justo el payday va al mes siguiente", d(2026, time.May, 29), d(2026, time.June, 1)},
		{"un día antes del payday queda en el mes", d(2026, time.May, 28), d(2026, time.May, 1)},
		{"mediados de mes queda en el mes", d(2026, time.June, 10), d(2026, time.June, 1)},
		{"el payday de junio va a julio", d(2026, time.June, 30), d(2026, time.July, 1)},
		{"cruce de año diciembre→enero", d(2026, time.December, 31), d(2027, time.January, 1)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Cycle(c.in); !got.Equal(c.want) {
				t.Errorf("Cycle(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestParseFormatCycle(t *testing.T) {
	c, err := ParseCycle("2026-06")
	if err != nil {
		t.Fatalf("ParseCycle: %v", err)
	}
	if !c.Equal(d(2026, time.June, 1)) {
		t.Errorf("ParseCycle = %v, want 2026-06-01", c)
	}
	if got := FormatCycle(d(2026, time.June, 1)); got != "2026-06" {
		t.Errorf("FormatCycle = %q, want 2026-06", got)
	}
}
