package jobs

import (
	"errors"
	"testing"
)

func TestSchedulePolicyDefaultsLimitsAndCanonicalChain(t *testing.T) {
	for _, interval := range []int{-1, 0, 15, 1441} {
		in, err := NormalizeScheduleInput(ScheduleInput{Name: "  ", IntervalMinutes: interval, Jobs: []string{"sync", " categorize "}})
		if err != nil {
			t.Fatal(err)
		}
		want := interval
		if want <= 0 {
			want = 10
		}
		if want > 1440 {
			want = 1440
		}
		if in.Name != "Scheduled chain" || in.IntervalMinutes != want || in.Jobs[1] != TypeCategorize {
			t.Fatalf("%+v", in)
		}
	}
	if _, err := NormalizeScheduleInput(ScheduleInput{Jobs: []string{TypeInterpretProject}}); !errors.Is(err, ErrNotSchedulable) {
		t.Fatalf("%v", err)
	}
	if _, err := NormalizeScheduleInput(ScheduleInput{}); err == nil {
		t.Fatal("empty chain accepted")
	}
}
