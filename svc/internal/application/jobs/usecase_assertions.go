package jobs

import domainjobs "github.com/Kapital-B/automata/svc/internal/domain/jobs"

var _ domainjobs.JobExecutionUseCases = (*ExecutionService)(nil)
var _ domainjobs.JobSchedulerUseCases = (*SchedulerService)(nil)
