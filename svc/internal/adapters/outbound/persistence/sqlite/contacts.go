package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	"github.com/Kapital-B/automata/svc/internal/domain/contacts"
	"github.com/google/uuid"
)

func (r *Repository) GetOrganisation(ctx context.Context, id uuid.UUID) (*driven.OrganisationRow, error) {
	var name, createdAt, updatedAt string
	err := r.db.QueryRowContext(ctx, `
		SELECT name, created_at, updated_at FROM organisations WHERE id = ?
	`, id.String()).Scan(&name, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
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
	return &driven.OrganisationRow{ID: id, Name: name, CreatedAt: cat, UpdatedAt: uat}, nil
}

func (r *Repository) ListContacts(ctx context.Context, organisationID uuid.UUID, filter driven.ContactListFilter) ([]driven.ContactRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(filter.Query)
	var rows *sql.Rows
	var err error
	if q == "" {
		rows, err = r.db.QueryContext(ctx, `
			SELECT id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at
			FROM contacts
			WHERE organisation_id = ? AND merged_into_contact_id IS NULL
			ORDER BY display_name COLLATE NOCASE ASC, created_at ASC
			LIMIT ? OFFSET ?
		`, organisationID.String(), limit, offset)
	} else {
		like := "%" + strings.ToLower(q) + "%"
		rows, err = r.db.QueryContext(ctx, `
			SELECT DISTINCT c.id, c.organisation_id, c.display_name, c.company, c.merged_into_contact_id, c.created_at, c.updated_at
			FROM contacts c
			LEFT JOIN contact_identities i ON i.contact_id = c.id
			WHERE c.organisation_id = ? AND c.merged_into_contact_id IS NULL
			  AND (
				LOWER(c.display_name) LIKE ? OR
				LOWER(IFNULL(c.company, '')) LIKE ? OR
				LOWER(i.value_normalized) LIKE ? OR
				LOWER(i.value_raw) LIKE ?
			  )
			ORDER BY c.display_name COLLATE NOCASE ASC, c.created_at ASC
			LIMIT ? OFFSET ?
		`, organisationID.String(), like, like, like, like, limit, offset)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContactRow, 0)
	for rows.Next() {
		row, err := scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) GetContact(ctx context.Context, organisationID, contactID uuid.UUID) (*driven.ContactRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at
		FROM contacts
		WHERE id = ? AND organisation_id = ?
	`, contactID.String(), organisationID.String())
	c, err := scanContactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Repository) CreateContact(ctx context.Context, row driven.ContactRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contacts (id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.DisplayName, nullStr(row.Company),
		formatRFC3339(row.CreatedAt.UTC()), formatRFC3339(row.UpdatedAt.UTC()))
	return err
}

func (r *Repository) UpdateContactDisplayNameIfEmpty(ctx context.Context, organisationID, contactID uuid.UUID, displayName string, at time.Time) error {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE contacts
		SET display_name = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ? AND TRIM(display_name) = ''
	`, name, formatRFC3339(at.UTC()), contactID.String(), organisationID.String())
	return err
}

func (r *Repository) ListIdentities(ctx context.Context, organisationID, contactID uuid.UUID) ([]driven.ContactIdentityRow, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, organisation_id, contact_id, kind, value_normalized, value_raw, created_at
		FROM contact_identities
		WHERE organisation_id = ? AND contact_id = ?
		ORDER BY kind ASC, value_normalized ASC
	`, organisationID.String(), contactID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContactIdentityRow, 0)
	for rows.Next() {
		ident, err := scanIdentityRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *ident)
	}
	return out, rows.Err()
}

func (r *Repository) FindContactIdentity(ctx context.Context, organisationID uuid.UUID, kind, valueNormalized string) (*driven.ContactIdentityRow, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, organisation_id, contact_id, kind, value_normalized, value_raw, created_at
		FROM contact_identities
		WHERE organisation_id = ? AND kind = ? AND value_normalized = ?
	`, organisationID.String(), kind, valueNormalized)
	ident, err := scanIdentityRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ident, nil
}

func (r *Repository) CreateIdentity(ctx context.Context, row driven.ContactIdentityRow) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO contact_identities (id, organisation_id, contact_id, kind, value_normalized, value_raw, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ContactID.String(), row.Kind,
		row.ValueNormalized, row.ValueRaw, formatRFC3339(row.CreatedAt.UTC()))
	return err
}

func (r *Repository) UpsertParticipant(ctx context.Context, row driven.CorrespondenceParticipantRow) error {
	if row.MessageID == nil && row.ManualItemID == nil {
		return fmt.Errorf("participant requires message_id or manual_item_id")
	}
	var messageID, manualItemID any
	if row.MessageID != nil {
		messageID = row.MessageID.String()
	}
	if row.ManualItemID != nil {
		manualItemID = row.ManualItemID.String()
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO correspondence_participants (id, organisation_id, contact_id, role, message_id, manual_item_id)
		VALUES (?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ContactID.String(), row.Role, messageID, manualItemID)
	return err
}

func (r *Repository) ListRecentMessageIDs(ctx context.Context, organisationID, contactID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT cp.message_id
		FROM correspondence_participants cp
		INNER JOIN messages m ON m.id = cp.message_id
		WHERE cp.organisation_id = ? AND cp.contact_id = ? AND cp.message_id IS NOT NULL
		ORDER BY m.received_at DESC
		LIMIT ?
	`, organisationID.String(), contactID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *Repository) SuggestMerges(ctx context.Context, organisationID, contactID uuid.UUID) ([]driven.ContactRow, error) {
	var displayName string
	err := r.db.QueryRowContext(ctx, `
		SELECT display_name FROM contacts
		WHERE id = ? AND organisation_id = ? AND merged_into_contact_id IS NULL
	`, contactID.String(), organisationID.String()).Scan(&displayName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(displayName)
	if name == "" {
		return []driven.ContactRow{}, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.organisation_id, c.display_name, c.company, c.merged_into_contact_id, c.created_at, c.updated_at
		FROM contacts c
		WHERE c.organisation_id = ?
		  AND c.id != ?
		  AND c.merged_into_contact_id IS NULL
		  AND LOWER(TRIM(c.display_name)) = LOWER(?)
		  AND NOT EXISTS (
			SELECT 1
			FROM contact_identities a
			INNER JOIN contact_identities b
				ON a.kind = b.kind AND a.value_normalized = b.value_normalized
			WHERE a.contact_id = ? AND b.contact_id = c.id AND a.kind = 'email'
		  )
		ORDER BY c.created_at ASC
	`, organisationID.String(), contactID.String(), name, contactID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContactRow, 0)
	for rows.Next() {
		row, err := scanContactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *row)
	}
	return out, rows.Err()
}

func (r *Repository) MergeContacts(ctx context.Context, organisationID, survivorID, sourceID uuid.UUID, at time.Time) error {
	if survivorID == sourceID {
		return fmt.Errorf("cannot merge a contact into itself")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var survOrg, srcOrg string
	var survMerged, srcMerged sql.NullString
	err = tx.QueryRowContext(ctx, `
		SELECT organisation_id, merged_into_contact_id FROM contacts WHERE id = ?
	`, survivorID.String()).Scan(&survOrg, &survMerged)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `
		SELECT organisation_id, merged_into_contact_id FROM contacts WHERE id = ?
	`, sourceID.String()).Scan(&srcOrg, &srcMerged)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.ErrNoRows
	}
	if err != nil {
		return err
	}
	if survOrg != organisationID.String() || srcOrg != organisationID.String() {
		return fmt.Errorf("contacts not in organisation")
	}
	if survMerged.Valid || srcMerged.Valid {
		return fmt.Errorf("contact already merged")
	}

	// Move identities; unique conflicts become errors.
	identRows, err := tx.QueryContext(ctx, `
		SELECT id, kind, value_normalized, value_raw, created_at
		FROM contact_identities WHERE contact_id = ?
	`, sourceID.String())
	if err != nil {
		return err
	}
	type identMove struct {
		id, kind, norm, raw, created string
	}
	var toMove []identMove
	for identRows.Next() {
		var m identMove
		if err := identRows.Scan(&m.id, &m.kind, &m.norm, &m.raw, &m.created); err != nil {
			identRows.Close()
			return err
		}
		toMove = append(toMove, m)
	}
	identRows.Close()
	if err := identRows.Err(); err != nil {
		return err
	}
	for _, m := range toMove {
		var existing sql.NullString
		err := tx.QueryRowContext(ctx, `
			SELECT contact_id FROM contact_identities
			WHERE organisation_id = ? AND kind = ? AND value_normalized = ? AND contact_id != ?
		`, organisationID.String(), m.kind, m.norm, sourceID.String()).Scan(&existing)
		if err == nil && existing.Valid {
			if existing.String == survivorID.String() {
				if _, err := tx.ExecContext(ctx, `DELETE FROM contact_identities WHERE id = ?`, m.id); err != nil {
					return err
				}
				continue
			}
			return fmt.Errorf("identity conflict: unique constraint")
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE contact_identities SET contact_id = ? WHERE id = ?
		`, survivorID.String(), m.id); err != nil {
			return err
		}
	}

	// Move participants; drop duplicates that already exist on survivor.
	partRows, err := tx.QueryContext(ctx, `
		SELECT id, role, message_id, manual_item_id
		FROM correspondence_participants WHERE contact_id = ?
	`, sourceID.String())
	if err != nil {
		return err
	}
	type partMove struct {
		id, role          string
		messageID, manual sql.NullString
	}
	var parts []partMove
	for partRows.Next() {
		var p partMove
		if err := partRows.Scan(&p.id, &p.role, &p.messageID, &p.manual); err != nil {
			partRows.Close()
			return err
		}
		parts = append(parts, p)
	}
	partRows.Close()
	if err := partRows.Err(); err != nil {
		return err
	}
	for _, p := range parts {
		if p.messageID.Valid {
			var exists int
			_ = tx.QueryRowContext(ctx, `
				SELECT COUNT(1) FROM correspondence_participants
				WHERE contact_id = ? AND role = ? AND message_id = ?
			`, survivorID.String(), p.role, p.messageID.String).Scan(&exists)
			if exists > 0 {
				if _, err := tx.ExecContext(ctx, `DELETE FROM correspondence_participants WHERE id = ?`, p.id); err != nil {
					return err
				}
				continue
			}
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE correspondence_participants SET contact_id = ? WHERE id = ?
		`, survivorID.String(), p.id); err != nil {
			return err
		}
	}

	if _, err := tx.ExecContext(ctx, `
		UPDATE contacts
		SET merged_into_contact_id = ?, updated_at = ?
		WHERE id = ?
	`, survivorID.String(), formatRFC3339(at.UTC()), sourceID.String()); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) ResolveEmailContact(ctx context.Context, organisationID uuid.UUID, email, displayName string, now time.Time) (uuid.UUID, error) {
	norm := contacts.NormalizeEmail(email)
	if norm == "" {
		return uuid.Nil, fmt.Errorf("empty email")
	}
	existing, err := r.FindContactIdentity(ctx, organisationID, string(contacts.KindEmail), norm)
	if err != nil {
		return uuid.Nil, err
	}
	if existing != nil {
		_ = r.UpdateContactDisplayNameIfEmpty(ctx, organisationID, existing.ContactID, displayName, now)
		return existing.ContactID, nil
	}
	contactID := uuid.New()
	name := strings.TrimSpace(displayName)
	row := driven.ContactRow{
		ID:             contactID,
		OrganisationID: organisationID,
		DisplayName:    name,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := r.CreateContact(ctx, row); err != nil {
		return uuid.Nil, err
	}
	ident := driven.ContactIdentityRow{
		ID:              uuid.New(),
		OrganisationID:  organisationID,
		ContactID:       contactID,
		Kind:            string(contacts.KindEmail),
		ValueNormalized: norm,
		ValueRaw:        strings.TrimSpace(email),
		CreatedAt:       now,
	}
	if err := r.CreateIdentity(ctx, ident); err != nil {
		// Race: another resolve created the same email.
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			again, findErr := r.FindContactIdentity(ctx, organisationID, string(contacts.KindEmail), norm)
			if findErr != nil {
				return uuid.Nil, findErr
			}
			if again != nil {
				return again.ContactID, nil
			}
		}
		return uuid.Nil, err
	}
	return contactID, nil
}

func (r *Repository) ListMessageIDsForAccount(ctx context.Context, accountID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 5000
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM messages WHERE account_id = ? ORDER BY received_at DESC LIMIT ?
	`, accountID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]uuid.UUID, 0)
	for rows.Next() {
		var idStr string
		if err := rows.Scan(&idStr); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func scanContactRow(s rowScanner) (*driven.ContactRow, error) {
	var idStr, orgStr, displayName, createdAt, updatedAt string
	var company, merged sql.NullString
	if err := s.Scan(&idStr, &orgStr, &displayName, &company, &merged, &createdAt, &updatedAt); err != nil {
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
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	uat, err := parseTime(updatedAt)
	if err != nil {
		return nil, err
	}
	row := &driven.ContactRow{
		ID:             id,
		OrganisationID: orgID,
		DisplayName:    displayName,
		Company:        nullStringPtr(company),
		CreatedAt:      cat,
		UpdatedAt:      uat,
	}
	if merged.Valid && merged.String != "" {
		mid, err := uuid.Parse(merged.String)
		if err != nil {
			return nil, err
		}
		row.MergedIntoContactID = &mid
	}
	return row, nil
}

func scanIdentityRow(s rowScanner) (*driven.ContactIdentityRow, error) {
	var idStr, orgStr, contactStr, kind, norm, raw, createdAt string
	if err := s.Scan(&idStr, &orgStr, &contactStr, &kind, &norm, &raw, &createdAt); err != nil {
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
	contactID, err := uuid.Parse(contactStr)
	if err != nil {
		return nil, err
	}
	cat, err := parseTime(createdAt)
	if err != nil {
		return nil, err
	}
	return &driven.ContactIdentityRow{
		ID:              id,
		OrganisationID:  orgID,
		ContactID:       contactID,
		Kind:            kind,
		ValueNormalized: norm,
		ValueRaw:        raw,
		CreatedAt:       cat,
	}, nil
}
