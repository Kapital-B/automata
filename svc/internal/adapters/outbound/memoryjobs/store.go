package memoryjobs

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/joblistcursor"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/google/uuid"
)

// Store is an in-memory JobStore with the same fencing semantics as DynamoDB.
type Store struct {
	mu        sync.Mutex
	jobs      map[uuid.UUID]*driven.JobRecord
	locks     map[string]*lockItem
	effects   map[string]*driven.EffectRecord
	cursorKey []byte
}

type lockItem struct {
	OwnerJobID     uuid.UUID
	OwnerAttemptID *uuid.UUID
	LeaseUntil     time.Time
	Revision       int64
}

func NewStore() *Store {
	return &Store{
		jobs:      make(map[uuid.UUID]*driven.JobRecord),
		locks:     make(map[string]*lockItem),
		effects:   make(map[string]*driven.EffectRecord),
		cursorKey: []byte("memoryjobs-default-cursor-key"),
	}
}

func cloneJob(j *driven.JobRecord) *driven.JobRecord {
	if j == nil {
		return nil
	}
	cp := *j
	cp.AccountID = cloneUUIDPtr(j.AccountID)
	cp.AccountLabel = cloneStringPtr(j.AccountLabel)
	if j.RemainingJobs != nil {
		cp.RemainingJobs = append([]string(nil), j.RemainingJobs...)
	}
	cp.ScheduleID = cloneUUIDPtr(j.ScheduleID)
	cp.ScheduledFor = cloneTimePtr(j.ScheduledFor)
	cp.ChainStartedAt = cloneTimePtr(j.ChainStartedAt)
	if j.Cursor != nil {
		c := *j.Cursor
		cp.Cursor = &c
	}
	cp.Payload = clonePayload(j.Payload)
	if j.Progress.Detail != nil {
		cp.Progress.Detail = map[string]interface{}{}
		for k, v := range j.Progress.Detail {
			cp.Progress.Detail[k] = v
		}
	}
	cp.ErrorMessage = cloneStringPtr(j.ErrorMessage)
	cp.CancelRequestedAt = cloneTimePtr(j.CancelRequestedAt)
	cp.RetryNotBefore = cloneTimePtr(j.RetryNotBefore)
	cp.AttemptID = cloneUUIDPtr(j.AttemptID)
	cp.LeaseOwner = cloneStringPtr(j.LeaseOwner)
	cp.LeaseUntil = cloneTimePtr(j.LeaseUntil)
	cp.StartedAt = cloneTimePtr(j.StartedAt)
	cp.FinishedAt = cloneTimePtr(j.FinishedAt)
	cp.ExpiresAt = cloneTimePtr(j.ExpiresAt)
	return &cp
}

func cloneUUIDPtr(v *uuid.UUID) *uuid.UUID {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneStringPtr(v *string) *string {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneTimePtr(v *time.Time) *time.Time {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func clonePayload(p driven.JobPayload) driven.JobPayload {
	return driven.JobPayload{
		ConnectorAccountID: cloneUUIDPtr(p.ConnectorAccountID),
		MessageID:          cloneUUIDPtr(p.MessageID),
		ProjectID:          cloneUUIDPtr(p.ProjectID),
		Recategorize:       p.Recategorize,
		Force:              p.Force,
		TimeWindowStart:    cloneTimePtr(p.TimeWindowStart),
		TimeWindowEnd:      cloneTimePtr(p.TimeWindowEnd),
	}
}

func (s *Store) CreatePending(_ context.Context, in driven.CreateJobInput) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	if existing, ok := s.jobs[in.ID]; ok {
		return cloneJob(existing), nil
	}
	if in.AcquireLock {
		key := lockKey(in.LockScope, in.LockKey)
		if lk, ok := s.locks[key]; ok {
			if owner, exists := s.jobs[lk.OwnerJobID]; exists && !isTerminal(owner.Status) {
				return nil, fmt.Errorf("%w: owner=%s", driven.ErrJobLockHeld, lk.OwnerJobID)
			}
		}
		s.locks[key] = &lockItem{OwnerJobID: in.ID, LeaseUntil: now.Add(5 * time.Minute), Revision: 1}
	}
	rec := &driven.JobRecord{
		ID:             in.ID,
		JobType:        in.JobType,
		Status:         driven.JobStatusPending,
		UserID:         in.UserID,
		AccountID:      in.AccountID,
		TriggerKind:    in.TriggerKind,
		ChainID:        in.ChainID,
		StepIndex:      in.StepIndex,
		RemainingJobs:  append([]string(nil), in.RemainingJobs...),
		ScheduleID:     in.ScheduleID,
		ScheduledFor:   in.ScheduledFor,
		ChainStartedAt: in.ChainStartedAt,
		Payload:        in.Payload,
		Revision:       1,
		WakeToken:      uuid.New(),
		CreatedAt:      now,
		UpdatedAt:      now,
		SchemaVersion:  1,
	}
	s.jobs[in.ID] = rec
	return cloneJob(rec), nil
}

func (s *Store) Get(_ context.Context, userID, jobID uuid.UUID) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[jobID]
	if !ok || j.UserID != userID {
		return nil, driven.ErrJobNotFound
	}
	return cloneJob(j), nil
}

func (s *Store) GetByID(_ context.Context, jobID uuid.UUID) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[jobID]
	if !ok {
		return nil, driven.ErrJobNotFound
	}
	return cloneJob(j), nil
}

func (s *Store) List(_ context.Context, filter driven.JobListFilter) (*driven.JobListPage, error) {
	if filter.Offset > 0 {
		return nil, driven.ErrOffsetNotSupported
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	var matched []*driven.JobRecord
	for _, j := range s.jobs {
		if j.UserID != filter.UserID {
			continue
		}
		if filter.AccountID != nil {
			if j.AccountID == nil || *j.AccountID != *filter.AccountID {
				continue
			}
		}
		if filter.JobType != "" && j.JobType != filter.JobType {
			continue
		}
		matched = append(matched, j)
	}
	sort.Slice(matched, func(i, k int) bool {
		if matched[i].CreatedAt.Equal(matched[k].CreatedAt) {
			return matched[i].ID.String() > matched[k].ID.String()
		}
		return matched[i].CreatedAt.After(matched[k].CreatedAt)
	})
	start := 0
	if filter.Cursor != "" {
		dec, err := joblistcursor.Decode(s.cursorKey, filter.Cursor)
		if err != nil {
			return nil, err
		}
		if dec.Scope != cursorScope(filter) || dec.UserID != filter.UserID || dec.JobType != filter.JobType || !sameUUIDPtr(dec.AccountID, filter.AccountID) {
			return nil, fmt.Errorf("invalid cursor")
		}
		id, err := uuid.Parse(dec.StartKey["id"])
		if err != nil {
			return nil, fmt.Errorf("invalid cursor")
		}
		createdAt, err := time.Parse(time.RFC3339Nano, dec.StartKey["created_at"])
		if err != nil {
			return nil, fmt.Errorf("invalid cursor")
		}
		for i, m := range matched {
			if m.ID == id && m.CreatedAt.Equal(createdAt) {
				start = i + 1
				break
			}
		}
	}
	page := &driven.JobListPage{Jobs: make([]driven.JobRecord, 0, limit)}
	for i := start; i < len(matched) && len(page.Jobs) < limit; i++ {
		page.Jobs = append(page.Jobs, *cloneJob(matched[i]))
	}
	if start+len(page.Jobs) < len(matched) && len(page.Jobs) > 0 {
		last := page.Jobs[len(page.Jobs)-1]
		next, err := joblistcursor.Encode(s.cursorKey, joblistcursor.Claims{
			Scope:     cursorScope(filter),
			UserID:    filter.UserID,
			AccountID: filter.AccountID,
			JobType:   filter.JobType,
			StartKey: map[string]string{
				"id":         last.ID.String(),
				"created_at": last.CreatedAt.UTC().Format(time.RFC3339Nano),
			},
		})
		if err != nil {
			return nil, err
		}
		page.NextCursor = next
	}
	return page, nil
}

func (s *Store) KickPending(_ context.Context, jobID uuid.UUID, expectedRevision int64, leaseOwner string, leaseUntil, now time.Time) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.Status != driven.JobStatusPending || j.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	if j.RetryNotBefore != nil && j.RetryNotBefore.After(now) {
		return nil, driven.ErrJobConflict
	}
	if j.CancelRequestedAt != nil {
		return s.terminalLocked(j, driven.JobStatusCancelled, nil, now, 30*24*time.Hour)
	}
	attempt := uuid.New()
	j.Status = driven.JobStatusRunning
	j.AttemptID = &attempt
	j.LeaseOwner = &leaseOwner
	j.LeaseUntil = &leaseUntil
	j.Revision++
	j.UpdatedAt = now
	if j.StartedAt == nil {
		t := now
		j.StartedAt = &t
	}
	s.renewLockLocked(j, attempt, leaseUntil)
	return cloneJob(j), nil
}

func (s *Store) AdvanceRunning(_ context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, cursor *driven.JobCursor, progress driven.JobProgress, leaseUntil, now time.Time) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAttempt(j, expectedRevision, attemptID); err != nil {
		return nil, err
	}
	if j.CancelRequestedAt != nil {
		return s.terminalLocked(j, driven.JobStatusCancelled, nil, now, 30*24*time.Hour)
	}
	if cursor != nil {
		c := *cursor
		j.Cursor = &c
	}
	j.Progress = progress
	j.LeaseUntil = &leaseUntil
	j.Revision++
	j.UpdatedAt = now
	s.renewLockLocked(j, attemptID, leaseUntil)
	return cloneJob(j), nil
}

func (s *Store) CompleteStep(_ context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, progress driven.JobProgress, next *driven.CreateJobInput, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAttempt(j, expectedRevision, attemptID); err != nil {
		return nil, err
	}
	j.Progress = progress
	if next != nil {
		if next.ID == uuid.Nil {
			next.ID = uuid.New()
		}
		if _, exists := s.jobs[next.ID]; !exists {
			n := &driven.JobRecord{
				ID:             next.ID,
				JobType:        next.JobType,
				Status:         driven.JobStatusPending,
				UserID:         next.UserID,
				AccountID:      next.AccountID,
				TriggerKind:    next.TriggerKind,
				ChainID:        next.ChainID,
				StepIndex:      next.StepIndex,
				RemainingJobs:  append([]string(nil), next.RemainingJobs...),
				ScheduleID:     next.ScheduleID,
				ScheduledFor:   next.ScheduledFor,
				ChainStartedAt: next.ChainStartedAt,
				Payload:        next.Payload,
				Revision:       1,
				WakeToken:      uuid.New(),
				CreatedAt:      now,
				UpdatedAt:      now,
				SchemaVersion:  1,
			}
			s.jobs[next.ID] = n
		}
		s.transferLockLocked(j, next.ID, now)
	} else {
		s.releaseLockLocked(j)
	}
	return s.terminalLocked(j, driven.JobStatusSuccess, nil, now, terminalTTL)
}

func (s *Store) FailJob(_ context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, errMsg string, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAttempt(j, expectedRevision, attemptID); err != nil {
		return nil, err
	}
	s.releaseLockLocked(j)
	msg := errMsg
	return s.terminalLocked(j, driven.JobStatusFailed, &msg, now, terminalTTL)
}

func (s *Store) RequestCancel(_ context.Context, userID, jobID uuid.UUID, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[jobID]
	if !ok || j.UserID != userID {
		return nil, driven.ErrJobNotFound
	}
	if isTerminal(j.Status) {
		return cloneJob(j), nil
	}
	t := now
	j.CancelRequestedAt = &t
	j.UpdatedAt = now
	if j.Status == driven.JobStatusPending {
		s.releaseLockLocked(j)
		return s.terminalLocked(j, driven.JobStatusCancelled, nil, now, terminalTTL)
	}
	j.Revision++
	return cloneJob(j), nil
}

func (s *Store) CancelRunning(_ context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if err := s.requireAttempt(j, expectedRevision, attemptID); err != nil {
		return nil, err
	}
	s.releaseLockLocked(j)
	return s.terminalLocked(j, driven.JobStatusCancelled, nil, now, terminalTTL)
}

func (s *Store) DeferRetry(_ context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, retryNotBefore time.Time, errMsg string, now time.Time) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.AttemptID == nil || *j.AttemptID != attemptID || j.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	j.Status = driven.JobStatusPending
	j.AttemptID = nil
	j.LeaseOwner = nil
	j.LeaseUntil = nil
	j.RetryNotBefore = &retryNotBefore
	j.ErrorCount++
	if errMsg != "" {
		j.ErrorMessage = &errMsg
	}
	j.WakeToken = uuid.New()
	j.Revision++
	j.UpdatedAt = now
	s.resetLockLeaseLocked(j, retryNotBefore)
	return cloneJob(j), nil
}

func (s *Store) ReWakePending(_ context.Context, jobID uuid.UUID, expectedRevision int64, now time.Time) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.Status != driven.JobStatusPending || j.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	j.WakeToken = uuid.New()
	j.Revision++
	j.UpdatedAt = now
	s.resetPendingLockLeaseLocked(j, now.Add(5*time.Minute))
	return cloneJob(j), nil
}

func (s *Store) RecoverExpiredLease(_ context.Context, jobID uuid.UUID, expectedRevision int64, expectedAttemptID uuid.UUID, now time.Time) (*driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, err := s.must(jobID)
	if err != nil {
		return nil, err
	}
	if j.Status != driven.JobStatusRunning || j.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	if j.AttemptID == nil || *j.AttemptID != expectedAttemptID {
		return nil, driven.ErrJobConflict
	}
	if j.LeaseUntil != nil && j.LeaseUntil.After(now) {
		return nil, driven.ErrJobConflict
	}
	j.Status = driven.JobStatusPending
	j.AttemptID = nil
	j.LeaseOwner = nil
	j.LeaseUntil = nil
	j.WakeToken = uuid.New()
	j.Revision++
	j.UpdatedAt = now
	j.ErrorCount++
	s.resetLockLeaseLocked(j, now)
	return cloneJob(j), nil
}

func (s *Store) ListStalePending(_ context.Context, olderThan time.Time, limit int) ([]driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	var out []driven.JobRecord
	for _, j := range s.jobs {
		if j.Status != driven.JobStatusPending {
			continue
		}
		if j.RetryNotBefore != nil && j.RetryNotBefore.After(olderThan) {
			continue
		}
		if j.UpdatedAt.After(olderThan) {
			continue
		}
		out = append(out, *cloneJob(j))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) ListExpiredLeases(_ context.Context, now time.Time, limit int) ([]driven.JobRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	var out []driven.JobRecord
	for _, j := range s.jobs {
		if j.Status != driven.JobStatusRunning {
			continue
		}
		if j.LeaseUntil == nil || j.LeaseUntil.After(now) {
			continue
		}
		out = append(out, *cloneJob(j))
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) ClaimEffect(_ context.Context, in driven.ClaimEffectInput) (*driven.EffectRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := in.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	key := effectLedgerKey(in.AccountID, in.EffectKey)
	if existing, ok := s.effects[key]; ok {
		cp := *existing
		return &cp, driven.ErrEffectAlreadyClaimed
	}
	rec := &driven.EffectRecord{
		AccountID: in.AccountID,
		EffectKey: in.EffectKey,
		State:     driven.EffectClaimed,
		JobID:     in.JobID,
		AttemptID: in.AttemptID,
		CreatedAt: now,
		UpdatedAt: now,
		Revision:  1,
	}
	s.effects[key] = rec
	cp := *rec
	return &cp, nil
}

func (s *Store) UpdateEffect(_ context.Context, accountID uuid.UUID, effectKey string, expectedRevision int64, state string, auditJSON string, now time.Time) (*driven.EffectRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := effectLedgerKey(accountID, effectKey)
	rec, ok := s.effects[key]
	if !ok {
		return nil, driven.ErrJobNotFound
	}
	if rec.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	rec.State = state
	rec.AuditJSON = auditJSON
	rec.Revision++
	rec.UpdatedAt = now
	cp := *rec
	return &cp, nil
}

func (s *Store) GetEffect(_ context.Context, accountID uuid.UUID, effectKey string) (*driven.EffectRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rec, ok := s.effects[effectLedgerKey(accountID, effectKey)]
	if !ok {
		return nil, driven.ErrJobNotFound
	}
	cp := *rec
	return &cp, nil
}

func (s *Store) ListEffectsByState(_ context.Context, state string, updatedBefore time.Time, limit int) ([]driven.EffectRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 {
		limit = 100
	}
	var out []driven.EffectRecord
	for _, e := range s.effects {
		if e.State != state {
			continue
		}
		if e.UpdatedAt.After(updatedBefore) {
			continue
		}
		out = append(out, *e)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (s *Store) must(jobID uuid.UUID) (*driven.JobRecord, error) {
	j, ok := s.jobs[jobID]
	if !ok {
		return nil, driven.ErrJobNotFound
	}
	return j, nil
}

func (s *Store) requireAttempt(j *driven.JobRecord, expectedRevision int64, attemptID uuid.UUID) error {
	if j.Status != driven.JobStatusRunning || j.Revision != expectedRevision {
		return driven.ErrJobConflict
	}
	if j.AttemptID == nil || *j.AttemptID != attemptID {
		return driven.ErrJobConflict
	}
	return nil
}

func (s *Store) terminalLocked(j *driven.JobRecord, status string, errMsg *string, now time.Time, ttl time.Duration) (*driven.JobRecord, error) {
	j.Status = status
	j.FinishedAt = &now
	j.UpdatedAt = now
	j.AttemptID = nil
	j.LeaseOwner = nil
	j.LeaseUntil = nil
	if errMsg != nil {
		j.ErrorMessage = errMsg
	}
	j.Revision++
	if ttl > 0 {
		exp := now.Add(ttl)
		j.ExpiresAt = &exp
	}
	return cloneJob(j), nil
}

func (s *Store) renewLockLocked(j *driven.JobRecord, attemptID uuid.UUID, leaseUntil time.Time) {
	for _, lk := range s.locks {
		if lk.OwnerJobID == j.ID {
			lk.OwnerAttemptID = &attemptID
			lk.LeaseUntil = leaseUntil
			lk.Revision++
		}
	}
}

func (s *Store) transferLockLocked(from *driven.JobRecord, toID uuid.UUID, now time.Time) {
	for key, lk := range s.locks {
		if lk.OwnerJobID == from.ID {
			s.locks[key] = &lockItem{OwnerJobID: toID, LeaseUntil: now, Revision: lk.Revision + 1}
		}
	}
}

func (s *Store) releaseLockLocked(j *driven.JobRecord) {
	for key, lk := range s.locks {
		if lk.OwnerJobID == j.ID {
			delete(s.locks, key)
		}
	}
}

func (s *Store) resetLockLeaseLocked(j *driven.JobRecord, leaseUntil time.Time) {
	for _, lk := range s.locks {
		if lk.OwnerJobID == j.ID {
			lk.OwnerAttemptID = nil
			lk.LeaseUntil = leaseUntil
			lk.Revision++
		}
	}
}

func (s *Store) resetPendingLockLeaseLocked(j *driven.JobRecord, leaseUntil time.Time) {
	for _, lk := range s.locks {
		if lk.OwnerJobID == j.ID {
			lk.LeaseUntil = leaseUntil
			lk.Revision++
		}
	}
}

func isTerminal(status string) bool {
	return status == driven.JobStatusSuccess || status == driven.JobStatusFailed || status == driven.JobStatusCancelled
}

func lockKey(scope, key string) string { return scope + "#" + key }

func effectLedgerKey(accountID uuid.UUID, key string) string {
	return accountID.String() + "#" + key
}

func cursorScope(filter driven.JobListFilter) string {
	switch {
	case filter.AccountID != nil && filter.JobType != "":
		return "account_type"
	case filter.AccountID != nil:
		return "account"
	case filter.JobType != "":
		return "user_type"
	default:
		return "user"
	}
}

func sameUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

var _ driven.JobStore = (*Store)(nil)
