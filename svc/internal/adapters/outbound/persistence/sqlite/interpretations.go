package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlkit "github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlkit"
	domaininterpretations "github.com/Kapital-B/automata/svc/internal/domain/interpretations"
	"github.com/google/uuid"
)

func (r *Repository) CreateInterpretation(ctx context.Context, row domaininterpretations.InterpretationRow) (portResult0 error) {
	defer func() {
		portResult0 = sqlkit.DomainError(portResult0)
	}()

	var accountID, runID, conf any
	if row.AccountID != nil {
		accountID = row.AccountID.String()
	}
	if row.RunID != nil {
		runID = row.RunID.String()
	}
	if row.Confidence != nil {
		conf = *row.Confidence
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO interpretations (
			id, organisation_id, project_id, account_id, run_id, status,
			payload_json, confidence, reason, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), accountID, runID,
		row.Status, row.PayloadJSON, conf, row.Reason,
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) AddInterpretationSource(ctx context.Context, row domaininterpretations.InterpretationSourceRow) (portResult0 error) {
	defer func() {
		portResult0 = sqlkit.DomainError(portResult0)
	}()

	var msgID, manualID, connectorMessageID any
	if row.MessageID != nil {
		msgID = row.MessageID.String()
	}
	if row.ManualItemID != nil {
		manualID = row.ManualItemID.String()
	}
	if row.ConnectorMessageID != nil {
		connectorMessageID = row.ConnectorMessageID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO interpretation_sources (id, interpretation_id, message_id, manual_item_id, connector_message_id)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.InterpretationID.String(), msgID, manualID, connectorMessageID)
	return err
}

func (r *Repository) GetInterpretation(ctx context.Context, organisationID, id uuid.UUID) (portResult0 *domaininterpretations.InterpretationRow, portResult1 error) {
	defer func() {
		portResult1 = sqlkit.DomainError(portResult1)
	}()

	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, project_id, account_id, run_id, status,
			payload_json, confidence, reason, created_at, updated_at
		FROM interpretations WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	out, err := scanInterpretationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListPendingInterpretations(ctx context.Context, organisationID, projectID uuid.UUID) (portResult0 []domaininterpretations.InterpretationRow, portResult1 error) {
	defer func() {
		portResult1 = sqlkit.DomainError(portResult1)
	}()

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, project_id, account_id, run_id, status,
			payload_json, confidence, reason, created_at, updated_at
		FROM interpretations
		WHERE organisation_id = ? AND project_id = ? AND status = 'pending'
		ORDER BY created_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domaininterpretations.InterpretationRow, 0)
	for rows.Next() {
		row, err := scanInterpretationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateInterpretationStatus(ctx context.Context, organisationID, id uuid.UUID, status string, updatedAt time.Time) (portResult0 error) {
	defer func() {
		portResult0 = sqlkit.DomainError(portResult0)
	}()

	res, err := r.db.ExecContext(ctx, `
		UPDATE interpretations SET status = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, status, formatRFC3339(updatedAt.UTC()), id.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListInterpretationSources(ctx context.Context, interpretationID uuid.UUID) (portResult0 []domaininterpretations.InterpretationSourceRow, portResult1 error) {
	defer func() {
		portResult1 = sqlkit.DomainError(portResult1)
	}()

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, interpretation_id, message_id, manual_item_id, connector_message_id
		FROM interpretation_sources WHERE interpretation_id = ?
	`, interpretationID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domaininterpretations.InterpretationSourceRow, 0)
	for rows.Next() {
		src, err := scanInterpretationSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *src)
	}
	return out, rows.Err()
}

func scanInterpretationRow(s rowScanner) (*domaininterpretations.InterpretationRow, error) {
	var idStr, orgStr, projStr, status, payload, reason, createdAt, updatedAt string
	var accountID, runID sql.NullString
	var conf sql.NullFloat64
	if err := s.Scan(
		&idStr, &orgStr, &projStr, &accountID, &runID, &status,
		&payload, &conf, &reason, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	orgID, err := uuid.Parse(orgStr)
	if err != nil {
		return nil, err
	}
	projID, err := uuid.Parse(projStr)
	if err != nil {
		return nil, err
	}
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	uat, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	row := &domaininterpretations.InterpretationRow{
		ID: id, OrganisationID: orgID, ProjectID: projID, Status: status,
		PayloadJSON: payload, Reason: reason, CreatedAt: cat, UpdatedAt: uat,
	}
	if accountID.Valid && accountID.String != "" {
		aid, err := uuid.Parse(accountID.String)
		if err != nil {
			return nil, err
		}
		row.AccountID = &aid
	}
	if runID.Valid && runID.String != "" {
		rid, err := uuid.Parse(runID.String)
		if err != nil {
			return nil, err
		}
		row.RunID = &rid
	}
	if conf.Valid {
		c := conf.Float64
		row.Confidence = &c
	}
	return row, nil
}

func scanInterpretationSource(s rowScanner) (*domaininterpretations.InterpretationSourceRow, error) {
	var idStr, interpStr string
	var msg, manual, connectorMessage sql.NullString
	if err := s.Scan(&idStr, &interpStr, &msg, &manual, &connectorMessage); err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	interpID, err := uuid.Parse(interpStr)
	if err != nil {
		return nil, err
	}
	row := &domaininterpretations.InterpretationSourceRow{ID: id, InterpretationID: interpID}
	if msg.Valid && msg.String != "" {
		mid, err := uuid.Parse(msg.String)
		if err != nil {
			return nil, err
		}
		row.MessageID = &mid
	}
	if manual.Valid && manual.String != "" {
		mid, err := uuid.Parse(manual.String)
		if err != nil {
			return nil, err
		}
		row.ManualItemID = &mid
	}
	if connectorMessage.Valid && connectorMessage.String != "" {
		id, err := uuid.Parse(connectorMessage.String)
		if err != nil {
			return nil, err
		}
		row.ConnectorMessageID = &id
	}
	return row, nil
}
