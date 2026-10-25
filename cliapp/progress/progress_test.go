package progress

import (
	"bytes"
	"strings"
	"testing"
)

func TestQuietWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	q := Quiet{}
	q.Update(State{OverallDone: 5, OverallTotal: 10})
	q.Finish()
	if buf.Len() != 0 {
		t.Fatalf("quiet wrote %q", buf.String())
	}
}

func TestLineWritesPercentAndName(t *testing.T) {
	var buf bytes.Buffer
	line := &Line{Out: &buf, Bar: newBar()}
	line.Update(State{Index: 2, Count: 5, Name: "sodium.jar", OverallDone: 50, OverallTotal: 100, Speed: 2 * 1024 * 1024})
	out := buf.String()
	if !strings.Contains(out, "50%") || !strings.Contains(out, "2/5") || !strings.Contains(out, "sodium.jar") {
		t.Fatalf("progress line = %q", out)
	}
	line.Finish()
}

func TestRatioClamps(t *testing.T) {
	if ratio(-1, 10) != 0 || ratio(20, 10) != 1 || ratio(1, 0) != 0 {
		t.Fatal("ratio clamp failed")
	}
}
