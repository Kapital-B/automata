package dynamodbjobs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/joblistcursor"
	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
)

const (
	defaultPendingLockLease = 5 * time.Minute
	tableSchemaVersion      = 1

	gsiUser        = "gsi1"
	gsiAccount     = "gsi2"
	gsiActiveState = "gsi3"
	gsiAccountType = "gsi4"
	gsiUserType    = "gsi5"

	sortTimeLayout = "2006-01-02T15:04:05.000000000Z"
)

type Store struct {
	client    *dynamodb.Client
	tableName string
	cursorKey []byte
}

type jobItem struct {
	PK                string             `dynamodbav:"pk"`
	SK                string             `dynamodbav:"sk"`
	EntityType        string             `dynamodbav:"entity_type"`
	SchemaVersion     int                `dynamodbav:"schema_version"`
	JobID             string             `dynamodbav:"job_id"`
	JobType           string             `dynamodbav:"job_type"`
	Status            string             `dynamodbav:"status"`
	UserID            string             `dynamodbav:"user_id"`
	AccountID         *string            `dynamodbav:"account_id,omitempty"`
	AccountLabel      *string            `dynamodbav:"account_label,omitempty"`
	TriggerKind       string             `dynamodbav:"trigger_kind"`
	ChainID           string             `dynamodbav:"chain_id"`
	StepIndex         int                `dynamodbav:"step_index"`
	RemainingJobs     []string           `dynamodbav:"remaining_jobs,omitempty"`
	ScheduleID        *string            `dynamodbav:"schedule_id,omitempty"`
	ScheduledFor      *string            `dynamodbav:"scheduled_for,omitempty"`
	ChainStartedAt    *string            `dynamodbav:"chain_started_at,omitempty"`
	Cursor            *driven.JobCursor  `dynamodbav:"cursor,omitempty"`
	Progress          driven.JobProgress `dynamodbav:"progress"`
	Payload           jobPayloadItem     `dynamodbav:"payload"`
	ErrorMessage      *string            `dynamodbav:"error_message,omitempty"`
	ErrorCount        int                `dynamodbav:"error_count"`
	CancelRequestedAt *string            `dynamodbav:"cancel_requested_at,omitempty"`
	RetryNotBefore    *string            `dynamodbav:"retry_not_before,omitempty"`
	Revision          int64              `dynamodbav:"revision"`
	AttemptID         *string            `dynamodbav:"attempt_id,omitempty"`
	LeaseOwner        *string            `dynamodbav:"lease_owner,omitempty"`
	LeaseUntil        *string            `dynamodbav:"lease_until,omitempty"`
	WakeToken         string             `dynamodbav:"wake_token"`
	CreatedAt         string             `dynamodbav:"created_at"`
	StartedAt         *string            `dynamodbav:"started_at,omitempty"`
	UpdatedAt         string             `dynamodbav:"updated_at"`
	FinishedAt        *string            `dynamodbav:"finished_at,omitempty"`
	ExpiresAt         *int64             `dynamodbav:"expires_at,omitempty"`
	LockScope         *string            `dynamodbav:"lock_scope,omitempty"`
	LockKey           *string            `dynamodbav:"lock_key,omitempty"`
	GSI1PK            *string            `dynamodbav:"gsi1pk,omitempty"`
	GSI1SK            *string            `dynamodbav:"gsi1sk,omitempty"`
	GSI2PK            *string            `dynamodbav:"gsi2pk,omitempty"`
	GSI2SK            *string            `dynamodbav:"gsi2sk,omitempty"`
	GSI3PK            *string            `dynamodbav:"gsi3pk,omitempty"`
	GSI3SK            *string            `dynamodbav:"gsi3sk,omitempty"`
	GSI4PK            *string            `dynamodbav:"gsi4pk,omitempty"`
	GSI4SK            *string            `dynamodbav:"gsi4sk,omitempty"`
	GSI5PK            *string            `dynamodbav:"gsi5pk,omitempty"`
	GSI5SK            *string            `dynamodbav:"gsi5sk,omitempty"`
}

type jobPayloadItem struct {
	ConnectorAccountID *string `dynamodbav:"connector_account_id,omitempty"`
	MessageID          *string `dynamodbav:"message_id,omitempty"`
	ProjectID          *string `dynamodbav:"project_id,omitempty"`
	Recategorize       bool    `dynamodbav:"recategorize,omitempty"`
	Force              bool    `dynamodbav:"force,omitempty"`
	TimeWindowStart    *string `dynamodbav:"time_window_start,omitempty"`
	TimeWindowEnd      *string `dynamodbav:"time_window_end,omitempty"`
}

type lockItem struct {
	PK             string  `dynamodbav:"pk"`
	SK             string  `dynamodbav:"sk"`
	EntityType     string  `dynamodbav:"entity_type"`
	OwnerJobID     string  `dynamodbav:"owner_job_id"`
	OwnerAttemptID *string `dynamodbav:"owner_attempt_id,omitempty"`
	LeaseUntil     string  `dynamodbav:"lease_until"`
	Revision       int64   `dynamodbav:"revision"`
}

type effectItem struct {
	PK            string  `dynamodbav:"pk"`
	SK            string  `dynamodbav:"sk"`
	EntityType    string  `dynamodbav:"entity_type"`
	AccountID     string  `dynamodbav:"account_id"`
	EffectKey     string  `dynamodbav:"effect_key"`
	State         string  `dynamodbav:"state"`
	JobID         string  `dynamodbav:"job_id"`
	AttemptID     string  `dynamodbav:"attempt_id"`
	AuditJSON     string  `dynamodbav:"audit_json,omitempty"`
	CreatedAt     string  `dynamodbav:"created_at"`
	UpdatedAt     string  `dynamodbav:"updated_at"`
	Revision      int64   `dynamodbav:"revision"`
	GSI3PK        *string `dynamodbav:"gsi3pk,omitempty"`
	GSI3SK        *string `dynamodbav:"gsi3sk,omitempty"`
	SchemaVersion int     `dynamodbav:"schema_version"`
}

func NewStore(client *dynamodb.Client, tableName string, cursorKey []byte) *Store {
	return &Store{client: client, tableName: tableName, cursorKey: append([]byte(nil), cursorKey...)}
}

func (s *Store) CreatePending(ctx context.Context, in driven.CreateJobInput) (*driven.JobRecord, error) {
	now := in.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if in.ID == uuid.Nil {
		in.ID = uuid.New()
	}
	existing, err := s.GetByID(ctx, in.ID)
	if err == nil {
		return existing, nil
	}
	job := newJobItem(in, now)
	jobAV, err := attributevalue.MarshalMap(job)
	if err != nil {
		return nil, err
	}
	if !in.AcquireLock {
		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName:           &s.tableName,
			Item:                jobAV,
			ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
		})
		if err != nil {
			if existing, getErr := s.GetByID(ctx, in.ID); getErr == nil {
				return existing, nil
			}
			return nil, mapConflict(err)
		}
		return job.record()
	}
	lock := newLockItem(in.LockScope, in.LockKey, in.ID, nil, now.Add(defaultPendingLockLease))
	lockAV, err := attributevalue.MarshalMap(lock)
	if err != nil {
		return nil, err
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:           &s.tableName,
					Item:                jobAV,
					ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
				},
			},
			{
				Put: &types.Put{
					TableName:           &s.tableName,
					Item:                lockAV,
					ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
				},
			},
		},
	})
	if err != nil {
		if existing, getErr := s.GetByID(ctx, in.ID); getErr == nil {
			return existing, nil
		}
		if holder, heldErr := s.getLock(ctx, in.LockScope, in.LockKey); heldErr == nil {
			return nil, fmt.Errorf("%w: owner=%s", driven.ErrJobLockHeld, holder.OwnerJobID)
		}
		return nil, mapConflict(err)
	}
	return job.record()
}

func (s *Store) Get(ctx context.Context, userID, jobID uuid.UUID) (*driven.JobRecord, error) {
	job, err := s.GetByID(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if job.UserID != userID {
		return nil, driven.ErrJobNotFound
	}
	return job, nil
}

func (s *Store) GetByID(ctx context.Context, jobID uuid.UUID) (*driven.JobRecord, error) {
	item, err := s.getJobItem(ctx, jobID)
	if err != nil {
		return nil, err
	}
	return item.record()
}

func (s *Store) List(ctx context.Context, filter driven.JobListFilter) (*driven.JobListPage, error) {
	if filter.Offset > 0 {
		return nil, driven.ErrOffsetNotSupported
	}
	limit := clampLimit(filter.Limit)
	spec, err := s.listQuerySpec(filter)
	if err != nil {
		return nil, err
	}
	page := &driven.JobListPage{Jobs: make([]driven.JobRecord, 0, limit)}
	startKey := spec.startKey
	for len(page.Jobs) < limit {
		out, err := s.client.Query(ctx, &dynamodb.QueryInput{
			TableName:                 &s.tableName,
			IndexName:                 strPtr(spec.index),
			KeyConditionExpression:    &spec.keyCondition,
			FilterExpression:          spec.filterExpression,
			ExpressionAttributeValues: spec.values,
			ExclusiveStartKey:         startKey,
			Limit:                     int32Ptr(int32(limit - len(page.Jobs))),
			ScanIndexForward:          boolPtr(false),
		})
		if err != nil {
			return nil, err
		}
		for _, raw := range out.Items {
			var item jobItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return nil, err
			}
			rec, err := item.record()
			if err != nil {
				return nil, err
			}
			page.Jobs = append(page.Jobs, *rec)
			if len(page.Jobs) >= limit {
				break
			}
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
		if len(page.Jobs) >= limit {
			next, err := joblistcursor.Encode(s.cursorKey, joblistcursor.Claims{
				Scope:     spec.scope,
				UserID:    filter.UserID,
				AccountID: filter.AccountID,
				JobType:   filter.JobType,
				StartKey:  encodeStartKey(startKey),
			})
			if err != nil {
				return nil, err
			}
			page.NextCursor = next
			break
		}
	}
	return page, nil
}

func (s *Store) KickPending(ctx context.Context, jobID uuid.UUID, expectedRevision int64, leaseOwner string, leaseUntil, now time.Time) (*driven.JobRecord, error) {
	current, err := s.getJobItem(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if current.Status != driven.JobStatusPending || current.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	if current.RetryNotBefore != nil {
		retryNotBefore, err := parseTimePtr(current.RetryNotBefore)
		if err != nil {
			return nil, err
		}
		if retryNotBefore != nil && retryNotBefore.After(now) {
			return nil, driven.ErrJobConflict
		}
	}
	if current.CancelRequestedAt != nil {
		return s.finishJob(ctx, current, driven.JobStatusCancelled, nil, now, 30*24*time.Hour, nil)
	}
	attemptID := uuid.New()
	next := *current
	next.Status = driven.JobStatusRunning
	next.AttemptID = stringPtr(attemptID.String())
	next.LeaseOwner = stringPtr(leaseOwner)
	next.LeaseUntil = stringPtr(formatTime(leaseUntil))
	next.Revision++
	next.UpdatedAt = formatTime(now)
	if next.StartedAt == nil {
		next.StartedAt = stringPtr(formatTime(now))
	}
	next.setActiveIndexes()
	if next.requiresLock() {
		return s.putJobWithLock(ctx, current, &next, nil, attemptID, false)
	}
	return s.replaceJob(ctx, current, &next, false)
}

func (s *Store) AdvanceRunning(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, cursor *driven.JobCursor, progress driven.JobProgress, leaseUntil, now time.Time) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, attemptID)
	if err != nil {
		return nil, err
	}
	if current.CancelRequestedAt != nil {
		return s.finishJob(ctx, current, driven.JobStatusCancelled, nil, now, 30*24*time.Hour, &attemptID)
	}
	next := *current
	if cursor != nil {
		cp := *cursor
		next.Cursor = &cp
	}
	next.Progress = progress
	next.LeaseUntil = stringPtr(formatTime(leaseUntil))
	next.Revision++
	next.UpdatedAt = formatTime(now)
	next.setActiveIndexes()
	if next.requiresLock() {
		return s.putJobWithLock(ctx, current, &next, &attemptID, attemptID, false)
	}
	return s.replaceJob(ctx, current, &next, true)
}

func (s *Store) CompleteStep(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, progress driven.JobProgress, nextInput *driven.CreateJobInput, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, attemptID)
	if err != nil {
		return nil, err
	}
	done := *current
	done.Progress = progress
	done.Status = driven.JobStatusSuccess
	done.FinishedAt = stringPtr(formatTime(now))
	done.UpdatedAt = formatTime(now)
	done.AttemptID = nil
	done.LeaseOwner = nil
	done.LeaseUntil = nil
	done.Revision++
	done.setTerminalIndexes(now, terminalTTL)
	if nextInput == nil {
		return s.finishJob(ctx, current, driven.JobStatusSuccess, nil, now, terminalTTL, &attemptID)
	}
	if nextInput.ID == uuid.Nil {
		nextInput.ID = uuid.New()
	}
	nextJob := newJobItem(*nextInput, now)
	return s.completeWithNext(ctx, current, &done, nextJob, attemptID)
}

func (s *Store) FailJob(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, errMsg string, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, attemptID)
	if err != nil {
		return nil, err
	}
	return s.finishJob(ctx, current, driven.JobStatusFailed, stringPtr(errMsg), now, terminalTTL, &attemptID)
}

func (s *Store) RequestCancel(ctx context.Context, userID, jobID uuid.UUID, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	current, err := s.getJobItem(ctx, jobID)
	if err != nil {
		return nil, err
	}
	rec, err := current.record()
	if err != nil {
		return nil, err
	}
	if rec.UserID != userID {
		return nil, driven.ErrJobNotFound
	}
	if isTerminal(current.Status) {
		return rec, nil
	}
	if current.Status == driven.JobStatusPending {
		return s.finishJob(ctx, current, driven.JobStatusCancelled, nil, now, terminalTTL, nil)
	}
	next := *current
	next.CancelRequestedAt = stringPtr(formatTime(now))
	next.UpdatedAt = formatTime(now)
	next.Revision++
	return s.replaceJob(ctx, current, &next, false)
}

func (s *Store) CancelRunning(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, now time.Time, terminalTTL time.Duration) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, attemptID)
	if err != nil {
		return nil, err
	}
	return s.finishJob(ctx, current, driven.JobStatusCancelled, nil, now, terminalTTL, &attemptID)
}

func (s *Store) DeferRetry(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID, retryNotBefore time.Time, errMsg string, now time.Time) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, attemptID)
	if err != nil {
		return nil, err
	}
	next := *current
	next.Status = driven.JobStatusPending
	next.AttemptID = nil
	next.LeaseOwner = nil
	next.LeaseUntil = nil
	next.RetryNotBefore = stringPtr(formatTime(retryNotBefore))
	next.ErrorCount++
	if errMsg != "" {
		next.ErrorMessage = stringPtr(errMsg)
	}
	next.WakeToken = uuid.NewString()
	next.Revision++
	next.UpdatedAt = formatTime(now)
	next.setActiveIndexes()
	if next.requiresLock() {
		return s.putJobWithLock(ctx, current, &next, &attemptID, uuid.Nil, true)
	}
	return s.replaceJob(ctx, current, &next, true)
}

func (s *Store) ReWakePending(ctx context.Context, jobID uuid.UUID, expectedRevision int64, now time.Time) (*driven.JobRecord, error) {
	current, err := s.getJobItem(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if current.Status != driven.JobStatusPending || current.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	next := *current
	next.WakeToken = uuid.NewString()
	next.Revision++
	next.UpdatedAt = formatTime(now)
	next.setActiveIndexes()
	if next.requiresLock() {
		return s.putJobWithLock(ctx, current, &next, nil, uuid.Nil, true)
	}
	return s.replaceJob(ctx, current, &next, false)
}

func (s *Store) RecoverExpiredLease(ctx context.Context, jobID uuid.UUID, expectedRevision int64, expectedAttemptID uuid.UUID, now time.Time) (*driven.JobRecord, error) {
	current, err := s.mustRunningAttempt(ctx, jobID, expectedRevision, expectedAttemptID)
	if err != nil {
		return nil, err
	}
	leaseUntil, err := parseTimePtr(current.LeaseUntil)
	if err != nil {
		return nil, err
	}
	if leaseUntil != nil && leaseUntil.After(now) {
		return nil, driven.ErrJobConflict
	}
	next := *current
	next.Status = driven.JobStatusPending
	next.AttemptID = nil
	next.LeaseOwner = nil
	next.LeaseUntil = nil
	next.WakeToken = uuid.NewString()
	next.ErrorCount++
	next.Revision++
	next.UpdatedAt = formatTime(now)
	next.setActiveIndexes()
	if next.requiresLock() {
		return s.putJobWithLock(ctx, current, &next, &expectedAttemptID, uuid.Nil, true)
	}
	return s.replaceJob(ctx, current, &next, true)
}

func (s *Store) ListStalePending(ctx context.Context, olderThan time.Time, limit int) ([]driven.JobRecord, error) {
	out, err := s.queryJobsByState(ctx, driven.JobStatusPending, "UPDATED#"+formatTime(olderThan), clampLimit(limit))
	if err != nil {
		return nil, err
	}
	filtered := make([]driven.JobRecord, 0, len(out))
	for _, job := range out {
		if job.RetryNotBefore != nil && job.RetryNotBefore.After(olderThan) {
			continue
		}
		filtered = append(filtered, job)
	}
	return filtered, nil
}

func (s *Store) ListExpiredLeases(ctx context.Context, now time.Time, limit int) ([]driven.JobRecord, error) {
	out, err := s.queryJobsByState(ctx, driven.JobStatusRunning, "LEASE#"+formatTime(now), clampLimit(limit))
	if err != nil {
		return nil, err
	}
	filtered := make([]driven.JobRecord, 0, len(out))
	for _, job := range out {
		if job.LeaseUntil == nil || job.LeaseUntil.After(now) {
			continue
		}
		filtered = append(filtered, job)
	}
	return filtered, nil
}

func (s *Store) ClaimEffect(ctx context.Context, in driven.ClaimEffectInput) (*driven.EffectRecord, error) {
	now := in.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	item := effectItem{
		PK:            effectPK(in.AccountID),
		SK:            effectSK(in.EffectKey),
		EntityType:    "effect",
		AccountID:     in.AccountID.String(),
		EffectKey:     in.EffectKey,
		State:         driven.EffectClaimed,
		JobID:         in.JobID.String(),
		AttemptID:     in.AttemptID.String(),
		CreatedAt:     formatTime(now),
		UpdatedAt:     formatTime(now),
		Revision:      1,
		SchemaVersion: tableSchemaVersion,
	}
	item.setStateIndex()
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return nil, err
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           &s.tableName,
		Item:                av,
		ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
	})
	if err != nil {
		existing, getErr := s.GetEffect(ctx, in.AccountID, in.EffectKey)
		if getErr == nil {
			return existing, driven.ErrEffectAlreadyClaimed
		}
		return nil, mapConflict(err)
	}
	return item.record()
}

func (s *Store) UpdateEffect(ctx context.Context, accountID uuid.UUID, effectKey string, expectedRevision int64, state string, auditJSON string, now time.Time) (*driven.EffectRecord, error) {
	current, err := s.getEffectItem(ctx, accountID, effectKey)
	if err != nil {
		return nil, err
	}
	if current.Revision != expectedRevision {
		return nil, driven.ErrJobConflict
	}
	next := *current
	next.State = state
	next.AuditJSON = auditJSON
	next.Revision++
	next.UpdatedAt = formatTime(now)
	next.setStateIndex()
	av, err := attributevalue.MarshalMap(next)
	if err != nil {
		return nil, err
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: &s.tableName,
		Item:      av,
		ConditionExpression: strPtr(
			"entity_type = :entity AND revision = :revision",
		),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":entity":   &types.AttributeValueMemberS{Value: "effect"},
			":revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", expectedRevision)},
		},
	})
	if err != nil {
		return nil, mapConflict(err)
	}
	return next.record()
}

func (s *Store) GetEffect(ctx context.Context, accountID uuid.UUID, effectKey string) (*driven.EffectRecord, error) {
	item, err := s.getEffectItem(ctx, accountID, effectKey)
	if err != nil {
		return nil, err
	}
	return item.record()
}

func (s *Store) ListEffectsByState(ctx context.Context, state string, updatedBefore time.Time, limit int) ([]driven.EffectRecord, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &s.tableName,
		IndexName:              strPtr(gsiActiveState),
		KeyConditionExpression: strPtr("gsi3pk = :pk AND gsi3sk <= :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "EFFECTSTATE#" + state},
			":sk": &types.AttributeValueMemberS{Value: "UPDATED#" + formatTime(updatedBefore) + "#EFFECT#~"},
		},
		Limit:            int32Ptr(int32(clampLimit(limit))),
		ScanIndexForward: boolPtr(true),
	})
	if err != nil {
		return nil, err
	}
	items := make([]driven.EffectRecord, 0, len(out.Items))
	for _, raw := range out.Items {
		var item effectItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return nil, err
		}
		rec, err := item.record()
		if err != nil {
			return nil, err
		}
		items = append(items, *rec)
	}
	return items, nil
}

func (s *Store) finishJob(ctx context.Context, current *jobItem, status string, errMsg *string, now time.Time, ttl time.Duration, attemptID *uuid.UUID) (*driven.JobRecord, error) {
	next := *current
	next.Status = status
	next.FinishedAt = stringPtr(formatTime(now))
	next.UpdatedAt = formatTime(now)
	next.AttemptID = nil
	next.LeaseOwner = nil
	next.LeaseUntil = nil
	next.Revision++
	next.ErrorMessage = errMsg
	next.setTerminalIndexes(now, ttl)
	if next.requiresLock() {
		return s.replaceJobAndDeleteLock(ctx, current, &next, attemptID)
	}
	if attemptID != nil {
		return s.replaceJob(ctx, current, &next, true)
	}
	return s.replaceJob(ctx, current, &next, false)
}

func (s *Store) completeWithNext(ctx context.Context, current, done *jobItem, nextJob *jobItem, attemptID uuid.UUID) (*driven.JobRecord, error) {
	doneAV, err := attributevalue.MarshalMap(done)
	if err != nil {
		return nil, err
	}
	nextAV, err := attributevalue.MarshalMap(nextJob)
	if err != nil {
		return nil, err
	}
	items := []types.TransactWriteItem{
		{
			Put: &types.Put{
				TableName:           &s.tableName,
				Item:                doneAV,
				ConditionExpression: strPtr("entity_type = :entity AND status = :status AND revision = :revision AND attempt_id = :attempt"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":entity":   &types.AttributeValueMemberS{Value: "job"},
					":status":   &types.AttributeValueMemberS{Value: driven.JobStatusRunning},
					":revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", current.Revision)},
					":attempt":  &types.AttributeValueMemberS{Value: attemptID.String()},
				},
			},
		},
		{
			Put: &types.Put{
				TableName:           &s.tableName,
				Item:                nextAV,
				ConditionExpression: strPtr("attribute_not_exists(pk) AND attribute_not_exists(sk)"),
			},
		},
	}
	if current.requiresLock() {
		lock, err := s.getLock(ctx, derefString(current.LockScope), derefString(current.LockKey))
		if err != nil {
			return nil, err
		}
		transferred := *lock
		transferred.OwnerJobID = nextJob.JobID
		transferred.OwnerAttemptID = nil
		transferred.LeaseUntil = formatTime(timeFromString(done.UpdatedAt))
		transferred.Revision++
		lockAV, err := attributevalue.MarshalMap(transferred)
		if err != nil {
			return nil, err
		}
		items = append(items, types.TransactWriteItem{
			Put: &types.Put{
				TableName:           &s.tableName,
				Item:                lockAV,
				ConditionExpression: strPtr("entity_type = :entity AND owner_job_id = :job AND owner_attempt_id = :attempt AND revision = :revision"),
				ExpressionAttributeValues: map[string]types.AttributeValue{
					":entity":   &types.AttributeValueMemberS{Value: "lock"},
					":job":      &types.AttributeValueMemberS{Value: current.JobID},
					":attempt":  &types.AttributeValueMemberS{Value: attemptID.String()},
					":revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", lock.Revision)},
				},
			},
		})
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{TransactItems: items})
	if err != nil {
		return nil, mapConflict(err)
	}
	return done.record()
}

func (s *Store) putJobWithLock(ctx context.Context, current, next *jobItem, expectedAttempt *uuid.UUID, newAttempt uuid.UUID, resetPendingLease bool) (*driven.JobRecord, error) {
	lock, err := s.getLock(ctx, derefString(current.LockScope), derefString(current.LockKey))
	if err != nil {
		return nil, err
	}
	nextLock := *lock
	if newAttempt != uuid.Nil {
		nextLock.OwnerAttemptID = stringPtr(newAttempt.String())
	} else {
		nextLock.OwnerAttemptID = nil
	}
	if resetPendingLease {
		leaseUntil := defaultPendingLockLease
		if next.RetryNotBefore != nil {
			parsed, err := parseTimePtr(next.RetryNotBefore)
			if err != nil {
				return nil, err
			}
			if parsed != nil {
				nextLock.LeaseUntil = formatTime(*parsed)
			}
		} else {
			nextLock.LeaseUntil = formatTime(timeFromString(next.UpdatedAt).Add(leaseUntil))
		}
	} else if next.LeaseUntil != nil {
		nextLock.LeaseUntil = *next.LeaseUntil
	}
	nextLock.Revision++
	jobAV, err := attributevalue.MarshalMap(next)
	if err != nil {
		return nil, err
	}
	lockAV, err := attributevalue.MarshalMap(nextLock)
	if err != nil {
		return nil, err
	}
	lockCondition := "entity_type = :lock_entity AND owner_job_id = :job AND revision = :lock_revision"
	values := map[string]types.AttributeValue{
		":lock_entity":   &types.AttributeValueMemberS{Value: "lock"},
		":job":           &types.AttributeValueMemberS{Value: current.JobID},
		":lock_revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", lock.Revision)},
	}
	if expectedAttempt != nil {
		lockCondition += " AND owner_attempt_id = :lock_attempt"
		values[":lock_attempt"] = &types.AttributeValueMemberS{Value: expectedAttempt.String()}
	}
	jobCondition, jobValues := s.jobCondition(current, expectedAttempt != nil)
	for k, v := range jobValues {
		values[k] = v
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 &s.tableName,
					Item:                      jobAV,
					ConditionExpression:       &jobCondition,
					ExpressionAttributeValues: jobValues,
				},
			},
			{
				Put: &types.Put{
					TableName:                 &s.tableName,
					Item:                      lockAV,
					ConditionExpression:       &lockCondition,
					ExpressionAttributeValues: values,
				},
			},
		},
	})
	if err != nil {
		return nil, mapConflict(err)
	}
	return next.record()
}

func (s *Store) replaceJobAndDeleteLock(ctx context.Context, current, next *jobItem, expectedAttempt *uuid.UUID) (*driven.JobRecord, error) {
	lock, err := s.getLock(ctx, derefString(current.LockScope), derefString(current.LockKey))
	if err != nil {
		return nil, err
	}
	jobAV, err := attributevalue.MarshalMap(next)
	if err != nil {
		return nil, err
	}
	jobCondition, jobValues := s.jobCondition(current, expectedAttempt != nil)
	lockCondition := "entity_type = :lock_entity AND owner_job_id = :job AND revision = :lock_revision"
	lockValues := map[string]types.AttributeValue{
		":lock_entity":   &types.AttributeValueMemberS{Value: "lock"},
		":job":           &types.AttributeValueMemberS{Value: current.JobID},
		":lock_revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", lock.Revision)},
	}
	if expectedAttempt != nil {
		lockCondition += " AND owner_attempt_id = :lock_attempt"
		lockValues[":lock_attempt"] = &types.AttributeValueMemberS{Value: expectedAttempt.String()}
	}
	_, err = s.client.TransactWriteItems(ctx, &dynamodb.TransactWriteItemsInput{
		TransactItems: []types.TransactWriteItem{
			{
				Put: &types.Put{
					TableName:                 &s.tableName,
					Item:                      jobAV,
					ConditionExpression:       &jobCondition,
					ExpressionAttributeValues: jobValues,
				},
			},
			{
				Delete: &types.Delete{
					TableName:                 &s.tableName,
					Key:                       map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: lock.PK}, "sk": &types.AttributeValueMemberS{Value: lock.SK}},
					ConditionExpression:       &lockCondition,
					ExpressionAttributeValues: lockValues,
				},
			},
		},
	})
	if err != nil {
		return nil, mapConflict(err)
	}
	return next.record()
}

func (s *Store) replaceJob(ctx context.Context, current, next *jobItem, requireAttempt bool) (*driven.JobRecord, error) {
	item, err := attributevalue.MarshalMap(next)
	if err != nil {
		return nil, err
	}
	condition, values := s.jobCondition(current, requireAttempt)
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:                 &s.tableName,
		Item:                      item,
		ConditionExpression:       &condition,
		ExpressionAttributeValues: values,
	})
	if err != nil {
		return nil, mapConflict(err)
	}
	return next.record()
}

func (s *Store) jobCondition(current *jobItem, requireAttempt bool) (string, map[string]types.AttributeValue) {
	condition := "entity_type = :entity AND status = :status AND revision = :revision"
	values := map[string]types.AttributeValue{
		":entity":   &types.AttributeValueMemberS{Value: "job"},
		":status":   &types.AttributeValueMemberS{Value: current.Status},
		":revision": &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", current.Revision)},
	}
	if requireAttempt && current.AttemptID != nil {
		condition += " AND attempt_id = :attempt"
		values[":attempt"] = &types.AttributeValueMemberS{Value: *current.AttemptID}
	}
	return condition, values
}

func (s *Store) mustRunningAttempt(ctx context.Context, jobID uuid.UUID, expectedRevision int64, attemptID uuid.UUID) (*jobItem, error) {
	current, err := s.getJobItem(ctx, jobID)
	if err != nil {
		return nil, err
	}
	if current.Status != driven.JobStatusRunning || current.Revision != expectedRevision || current.AttemptID == nil || *current.AttemptID != attemptID.String() {
		return nil, driven.ErrJobConflict
	}
	return current, nil
}

func (s *Store) queryJobsByState(ctx context.Context, state, upperBound string, limit int) ([]driven.JobRecord, error) {
	out, err := s.client.Query(ctx, &dynamodb.QueryInput{
		TableName:              &s.tableName,
		IndexName:              strPtr(gsiActiveState),
		KeyConditionExpression: strPtr("gsi3pk = :pk AND gsi3sk <= :sk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "STATUS#" + state},
			":sk": &types.AttributeValueMemberS{Value: upperBound + "#JOB#~"},
		},
		Limit:            int32Ptr(int32(limit)),
		ScanIndexForward: boolPtr(true),
	})
	if err != nil {
		return nil, err
	}
	records := make([]driven.JobRecord, 0, len(out.Items))
	for _, raw := range out.Items {
		var item jobItem
		if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
			return nil, err
		}
		rec, err := item.record()
		if err != nil {
			return nil, err
		}
		records = append(records, *rec)
	}
	return records, nil
}

func (s *Store) listQuerySpec(filter driven.JobListFilter) (*listSpec, error) {
	spec := &listSpec{
		values: map[string]types.AttributeValue{
			":user": &types.AttributeValueMemberS{Value: "USER#" + filter.UserID.String()},
		},
	}
	switch {
	case filter.AccountID != nil && filter.JobType != "":
		spec.scope = "account_type"
		spec.index = gsiAccountType
		spec.keyCondition = "gsi4pk = :pk"
		spec.values = map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: "ACCOUNT#" + filter.AccountID.String() + "#TYPE#" + filter.JobType},
			":user": &types.AttributeValueMemberS{Value: filter.UserID.String()},
		}
		spec.filterExpression = strPtr("user_id = :user")
	case filter.AccountID != nil:
		spec.scope = "account"
		spec.index = gsiAccount
		spec.keyCondition = "gsi2pk = :pk"
		spec.values = map[string]types.AttributeValue{
			":pk":   &types.AttributeValueMemberS{Value: "ACCOUNT#" + filter.AccountID.String()},
			":user": &types.AttributeValueMemberS{Value: filter.UserID.String()},
		}
		spec.filterExpression = strPtr("user_id = :user")
	case filter.JobType != "":
		spec.scope = "user_type"
		spec.index = gsiUserType
		spec.keyCondition = "gsi5pk = :pk"
		spec.values = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + filter.UserID.String() + "#TYPE#" + filter.JobType},
		}
	default:
		spec.scope = "user"
		spec.index = gsiUser
		spec.keyCondition = "gsi1pk = :pk"
		spec.values = map[string]types.AttributeValue{
			":pk": &types.AttributeValueMemberS{Value: "USER#" + filter.UserID.String()},
		}
	}
	if filter.Cursor != "" {
		claims, err := joblistcursor.Decode(s.cursorKey, filter.Cursor)
		if err != nil {
			return nil, err
		}
		if claims.Scope != spec.scope || claims.UserID != filter.UserID || claims.JobType != filter.JobType || !sameUUIDPtr(claims.AccountID, filter.AccountID) {
			return nil, fmt.Errorf("invalid cursor")
		}
		spec.startKey = decodeStartKey(claims.StartKey)
	}
	return spec, nil
}

type listSpec struct {
	scope            string
	index            string
	keyCondition     string
	values           map[string]types.AttributeValue
	filterExpression *string
	startKey         map[string]types.AttributeValue
}

func (s *Store) getJobItem(ctx context.Context, jobID uuid.UUID) (*jobItem, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: jobPK(jobID)},
			"sk": &types.AttributeValueMemberS{Value: "METADATA"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, driven.ErrJobNotFound
	}
	var item jobItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, err
	}
	if item.EntityType != "job" {
		return nil, driven.ErrJobNotFound
	}
	return &item, nil
}

func (s *Store) getLock(ctx context.Context, scope, key string) (*lockItem, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: lockPK(scope, key)},
			"sk": &types.AttributeValueMemberS{Value: "LOCK#SYNC"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, driven.ErrJobNotFound
	}
	var item lockItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) getEffectItem(ctx context.Context, accountID uuid.UUID, effectKey string) (*effectItem, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{
		TableName: &s.tableName,
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: effectPK(accountID)},
			"sk": &types.AttributeValueMemberS{Value: effectSK(effectKey)},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(out.Item) == 0 {
		return nil, driven.ErrJobNotFound
	}
	var item effectItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

func newJobItem(in driven.CreateJobInput, now time.Time) *jobItem {
	item := &jobItem{
		PK:             jobPK(in.ID),
		SK:             "METADATA",
		EntityType:     "job",
		SchemaVersion:  tableSchemaVersion,
		JobID:          in.ID.String(),
		JobType:        in.JobType,
		Status:         driven.JobStatusPending,
		UserID:         in.UserID.String(),
		AccountID:      uuidPtrString(in.AccountID),
		TriggerKind:    in.TriggerKind,
		ChainID:        in.ChainID.String(),
		StepIndex:      in.StepIndex,
		RemainingJobs:  append([]string(nil), in.RemainingJobs...),
		ScheduleID:     uuidPtrString(in.ScheduleID),
		ScheduledFor:   timePtrString(in.ScheduledFor),
		ChainStartedAt: timePtrString(in.ChainStartedAt),
		Progress:       driven.JobProgress{},
		Payload:        payloadToItem(in.Payload),
		Revision:       1,
		WakeToken:      uuid.NewString(),
		CreatedAt:      formatTime(now),
		UpdatedAt:      formatTime(now),
	}
	if in.AcquireLock {
		item.LockScope = stringPtr(in.LockScope)
		item.LockKey = stringPtr(in.LockKey)
	}
	item.setActiveIndexes()
	return item
}

func newLockItem(scope, key string, jobID uuid.UUID, attemptID *uuid.UUID, leaseUntil time.Time) *lockItem {
	item := &lockItem{
		PK:         lockPK(scope, key),
		SK:         "LOCK#SYNC",
		EntityType: "lock",
		OwnerJobID: jobID.String(),
		LeaseUntil: formatTime(leaseUntil),
		Revision:   1,
	}
	if attemptID != nil {
		item.OwnerAttemptID = stringPtr(attemptID.String())
	}
	return item
}

func (i *jobItem) setActiveIndexes() {
	userPK := "USER#" + i.UserID
	created := "CREATED#" + i.CreatedAt + "#JOB#" + i.JobID
	i.GSI1PK = stringPtr(userPK)
	i.GSI1SK = stringPtr(created)
	if i.AccountID != nil {
		i.GSI2PK = stringPtr("ACCOUNT#" + *i.AccountID)
		i.GSI2SK = stringPtr(created)
		i.GSI4PK = stringPtr("ACCOUNT#" + *i.AccountID + "#TYPE#" + i.JobType)
		i.GSI4SK = stringPtr(created)
	}
	i.GSI5PK = stringPtr(userPK + "#TYPE#" + i.JobType)
	i.GSI5SK = stringPtr(created)
	switch i.Status {
	case driven.JobStatusPending:
		i.GSI3PK = stringPtr("STATUS#" + i.Status)
		i.GSI3SK = stringPtr("UPDATED#" + i.UpdatedAt + "#JOB#" + i.JobID)
	case driven.JobStatusRunning:
		lease := ""
		if i.LeaseUntil != nil {
			lease = *i.LeaseUntil
		}
		i.GSI3PK = stringPtr("STATUS#" + i.Status)
		i.GSI3SK = stringPtr("LEASE#" + lease + "#JOB#" + i.JobID)
	default:
		i.GSI3PK = nil
		i.GSI3SK = nil
	}
}

func (i *jobItem) setTerminalIndexes(now time.Time, ttl time.Duration) {
	i.setActiveIndexes()
	i.GSI3PK = nil
	i.GSI3SK = nil
	if ttl > 0 {
		expiresAt := now.Add(ttl).Unix()
		i.ExpiresAt = &expiresAt
	}
}

func (i *jobItem) requiresLock() bool {
	return i.LockScope != nil && i.LockKey != nil && *i.LockScope != "" && *i.LockKey != ""
}

func (i *jobItem) record() (*driven.JobRecord, error) {
	id, err := uuid.Parse(i.JobID)
	if err != nil {
		return nil, err
	}
	userID, err := uuid.Parse(i.UserID)
	if err != nil {
		return nil, err
	}
	chainID, err := uuid.Parse(i.ChainID)
	if err != nil {
		return nil, err
	}
	createdAt, err := parseTime(i.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := parseTime(i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	rec := &driven.JobRecord{
		ID:            id,
		JobType:       i.JobType,
		Status:        i.Status,
		UserID:        userID,
		AccountID:     parseUUIDPtr(i.AccountID),
		AccountLabel:  i.AccountLabel,
		TriggerKind:   i.TriggerKind,
		ChainID:       chainID,
		StepIndex:     i.StepIndex,
		RemainingJobs: append([]string(nil), i.RemainingJobs...),
		ScheduleID:    parseUUIDPtr(i.ScheduleID),
		Cursor:        cloneCursor(i.Cursor),
		Progress:      i.Progress,
		Payload:       i.Payload.record(),
		ErrorMessage:  cloneString(i.ErrorMessage),
		ErrorCount:    i.ErrorCount,
		Revision:      i.Revision,
		LeaseOwner:    cloneString(i.LeaseOwner),
		WakeToken:     uuid.MustParse(i.WakeToken),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		SchemaVersion: i.SchemaVersion,
	}
	rec.ScheduledFor, err = parseTimePtr(i.ScheduledFor)
	if err != nil {
		return nil, err
	}
	rec.ChainStartedAt, err = parseTimePtr(i.ChainStartedAt)
	if err != nil {
		return nil, err
	}
	rec.CancelRequestedAt, err = parseTimePtr(i.CancelRequestedAt)
	if err != nil {
		return nil, err
	}
	rec.RetryNotBefore, err = parseTimePtr(i.RetryNotBefore)
	if err != nil {
		return nil, err
	}
	rec.LeaseUntil, err = parseTimePtr(i.LeaseUntil)
	if err != nil {
		return nil, err
	}
	rec.StartedAt, err = parseTimePtr(i.StartedAt)
	if err != nil {
		return nil, err
	}
	rec.FinishedAt, err = parseTimePtr(i.FinishedAt)
	if err != nil {
		return nil, err
	}
	rec.ExpiresAt, err = parseExpiryPtr(i.ExpiresAt)
	if err != nil {
		return nil, err
	}
	rec.AttemptID = parseUUIDPtr(i.AttemptID)
	return rec, nil
}

func (p jobPayloadItem) record() driven.JobPayload {
	return driven.JobPayload{
		ConnectorAccountID: parseUUIDPtr(p.ConnectorAccountID),
		MessageID:          parseUUIDPtr(p.MessageID),
		ProjectID:          parseUUIDPtr(p.ProjectID),
		Recategorize:       p.Recategorize,
		Force:              p.Force,
		TimeWindowStart:    mustParseTimePtr(p.TimeWindowStart),
		TimeWindowEnd:      mustParseTimePtr(p.TimeWindowEnd),
	}
}

func payloadToItem(p driven.JobPayload) jobPayloadItem {
	return jobPayloadItem{
		ConnectorAccountID: uuidPtrString(p.ConnectorAccountID),
		MessageID:          uuidPtrString(p.MessageID),
		ProjectID:          uuidPtrString(p.ProjectID),
		Recategorize:       p.Recategorize,
		Force:              p.Force,
		TimeWindowStart:    timePtrString(p.TimeWindowStart),
		TimeWindowEnd:      timePtrString(p.TimeWindowEnd),
	}
}

func (i *effectItem) setStateIndex() {
	switch i.State {
	case driven.EffectClaimed, driven.EffectUnknown, driven.EffectSucceededPendingAudit:
		i.GSI3PK = stringPtr("EFFECTSTATE#" + i.State)
		i.GSI3SK = stringPtr("UPDATED#" + i.UpdatedAt + "#EFFECT#" + i.EffectKey)
	default:
		i.GSI3PK = nil
		i.GSI3SK = nil
	}
}

func (i *effectItem) record() (*driven.EffectRecord, error) {
	accountID, err := uuid.Parse(i.AccountID)
	if err != nil {
		return nil, err
	}
	jobID, err := uuid.Parse(i.JobID)
	if err != nil {
		return nil, err
	}
	attemptID, err := uuid.Parse(i.AttemptID)
	if err != nil {
		return nil, err
	}
	createdAt, err := parseTime(i.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := parseTime(i.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &driven.EffectRecord{
		AccountID: accountID,
		EffectKey: i.EffectKey,
		State:     i.State,
		JobID:     jobID,
		AttemptID: attemptID,
		AuditJSON: i.AuditJSON,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Revision:  i.Revision,
	}, nil
}

func mapConflict(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "conditionalcheckfailed") || strings.Contains(msg, "transactioncanceled") {
		return driven.ErrJobConflict
	}
	return err
}

func isTerminal(status string) bool {
	return status == driven.JobStatusSuccess || status == driven.JobStatusFailed || status == driven.JobStatusCancelled
}

func jobPK(id uuid.UUID) string { return "JOB#" + id.String() }

func lockPK(scope, key string) string { return strings.ToUpper(scope) + "#" + key }

func effectPK(accountID uuid.UUID) string { return "ACCOUNT#" + accountID.String() }

func effectSK(key string) string { return "EFFECT#" + key }

func formatTime(t time.Time) string { return t.UTC().Format(sortTimeLayout) }

func parseTime(raw string) (time.Time, error) { return time.Parse(sortTimeLayout, raw) }

func parseTimePtr(raw *string) (*time.Time, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	t, err := parseTime(*raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func mustParseTimePtr(raw *string) *time.Time {
	t, err := parseTimePtr(raw)
	if err != nil {
		return nil
	}
	return t
}

func parseExpiryPtr(raw *int64) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	t := time.Unix(*raw, 0).UTC()
	return &t, nil
}

func parseUUIDPtr(raw *string) *uuid.UUID {
	if raw == nil || *raw == "" {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	return &id
}

func uuidPtrString(v *uuid.UUID) *string {
	if v == nil {
		return nil
	}
	return stringPtr(v.String())
}

func timePtrString(v *time.Time) *string {
	if v == nil {
		return nil
	}
	return stringPtr(formatTime(*v))
}

func timeFromString(v string) time.Time {
	t, _ := parseTime(v)
	return t
}

func cloneCursor(v *driven.JobCursor) *driven.JobCursor {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func cloneString(v *string) *string {
	if v == nil {
		return nil
	}
	cp := *v
	return &cp
}

func encodeStartKey(key map[string]types.AttributeValue) map[string]string {
	out := make(map[string]string, len(key))
	for k, v := range key {
		switch av := v.(type) {
		case *types.AttributeValueMemberS:
			out[k] = av.Value
		case *types.AttributeValueMemberN:
			out[k] = av.Value
		}
	}
	return out
}

func decodeStartKey(key map[string]string) map[string]types.AttributeValue {
	out := make(map[string]types.AttributeValue, len(key))
	for k, v := range key {
		out[k] = &types.AttributeValueMemberS{Value: v}
	}
	return out
}

func clampLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func sameUUIDPtr(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func derefString(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func strPtr(v string) *string { return &v }

func stringPtr(v string) *string { return &v }

func int32Ptr(v int32) *int32 { return &v }

func boolPtr(v bool) *bool { return &v }

var _ driven.JobStore = (*Store)(nil)
