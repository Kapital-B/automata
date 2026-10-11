package jobs

import (
	"fmt"
	"github.com/google/uuid"
	"strings"
)

type ScheduleInput struct {
	ID              uuid.UUID
	Name            string
	AccountID       *uuid.UUID
	Jobs            []string
	IntervalMinutes int
	Enabled         bool
}

// NormalizeScheduleInput applies the existing schedule defaults and validates its chain.
func NormalizeScheduleInput(in ScheduleInput) (ScheduleInput, error) {
	if in.IntervalMinutes <= 0 {
		in.IntervalMinutes = 10
	}
	if in.IntervalMinutes > 1440 {
		in.IntervalMinutes = 1440
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		in.Name = "Scheduled chain"
	}
	if len(in.Jobs) == 0 {
		return in, fmt.Errorf("%q needs at least one step", in.Name)
	}
	chain, err := DefaultRegistry().NormalizeScheduleChain(in.Jobs)
	if err != nil {
		return in, fmt.Errorf("%q cannot run: %w", in.Name, err)
	}
	in.Jobs = chain
	return in, nil
}
