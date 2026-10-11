package jobs

import "github.com/google/uuid"

type ScheduleInput struct {
	ID              uuid.UUID
	Name            string
	AccountID       *uuid.UUID
	Jobs            []string
	IntervalMinutes int
	Enabled         bool
}
