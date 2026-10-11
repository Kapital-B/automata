package projects

import domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"

var _ domainprojects.ProjectUseCases = (*Service)(nil)
var _ domainprojects.ProjectAssignUseCases = (*AssignService)(nil)
