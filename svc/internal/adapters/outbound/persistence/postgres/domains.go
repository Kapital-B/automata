package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Kapital-B/automata/svc/internal/application/ports/driven"
	domaincontacts "github.com/Kapital-B/automata/svc/internal/domain/contacts"
	"github.com/google/uuid"
)

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
	var (
		rows *sql.Rows
		err  error
	)
	if q == "" {
		rows, err = r.queryContext(ctx, `
			SELECT id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at
			FROM contacts
			WHERE organisation_id = ? AND merged_into_contact_id IS NULL
			ORDER BY lower(display_name) ASC, created_at ASC
			LIMIT ? OFFSET ?
		`, organisationID.String(), limit, offset)
	} else {
		like := "%" + strings.ToLower(q) + "%"
		rows, err = r.queryContext(ctx, `
			SELECT DISTINCT c.id, c.organisation_id, c.display_name, c.company, c.merged_into_contact_id, c.created_at, c.updated_at
			FROM contacts c
			LEFT JOIN contact_identities i ON i.contact_id = c.id
			WHERE c.organisation_id = ? AND c.merged_into_contact_id IS NULL
			  AND (
				LOWER(c.display_name) LIKE ? OR
				LOWER(COALESCE(c.company, '')) LIKE ? OR
				LOWER(i.value_normalized) LIKE ? OR
				LOWER(i.value_raw) LIKE ?
			  )
			ORDER BY lower(c.display_name) ASC, c.created_at ASC
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
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at
		FROM contacts
		WHERE id = ? AND organisation_id = ?
	`, contactID.String(), organisationID.String())
	out, err := scanContactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) CreateContact(ctx context.Context, row driven.ContactRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO contacts (id, organisation_id, display_name, company, merged_into_contact_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULL, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.DisplayName, nullStr(row.Company), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) UpdateContactDisplayNameIfEmpty(ctx context.Context, organisationID, contactID uuid.UUID, displayName string, at time.Time) error {
	name := strings.TrimSpace(displayName)
	if name == "" {
		return nil
	}
	_, err := r.execContext(ctx, `
		UPDATE contacts
		SET display_name = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ? AND btrim(display_name) = ''
	`, name, at.UTC(), contactID.String(), organisationID.String())
	return err
}

func (r *Repository) ListIdentities(ctx context.Context, organisationID, contactID uuid.UUID) ([]driven.ContactIdentityRow, error) {
	rows, err := r.queryContext(ctx, `
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
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, contact_id, kind, value_normalized, value_raw, created_at
		FROM contact_identities
		WHERE organisation_id = ? AND kind = ? AND value_normalized = ?
	`, organisationID.String(), kind, valueNormalized)
	out, err := scanIdentityRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) CreateIdentity(ctx context.Context, row driven.ContactIdentityRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO contact_identities (id, organisation_id, contact_id, kind, value_normalized, value_raw, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ContactID.String(), row.Kind, row.ValueNormalized, row.ValueRaw, row.CreatedAt.UTC())
	return err
}

func (r *Repository) UpsertParticipant(ctx context.Context, row driven.CorrespondenceParticipantRow) error {
	if row.MessageID == nil && row.ManualItemID == nil {
		return fmt.Errorf("participant requires message_id or manual_item_id")
	}
	_, err := r.execContext(ctx, `
		INSERT INTO correspondence_participants (id, organisation_id, contact_id, role, message_id, manual_item_id)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`, row.ID.String(), row.OrganisationID.String(), row.ContactID.String(), row.Role, nullUUID(row.MessageID), nullUUID(row.ManualItemID))
	return err
}

func (r *Repository) ListRecentMessageIDs(ctx context.Context, organisationID, contactID uuid.UUID, limit int) ([]uuid.UUID, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.queryContext(ctx, `
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
	return scanUUIDColumn(rows)
}

func (r *Repository) SuggestMerges(ctx context.Context, organisationID, contactID uuid.UUID) ([]driven.ContactRow, error) {
	var displayName string
	err := r.queryRowContext(ctx, `
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
	rows, err := r.queryContext(ctx, `
		SELECT c.id, c.organisation_id, c.display_name, c.company, c.merged_into_contact_id, c.created_at, c.updated_at
		FROM contacts c
		WHERE c.organisation_id = ?
		  AND c.id != ?
		  AND c.merged_into_contact_id IS NULL
		  AND LOWER(btrim(c.display_name)) = LOWER(?)
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
	return r.withTx(ctx, func(tx *sql.Tx) error {
		var survOrg, srcOrg string
		var survMerged, srcMerged sql.NullString
		err := txQueryRowContext(ctx, tx, `SELECT organisation_id, merged_into_contact_id FROM contacts WHERE id = ?`, survivorID.String()).Scan(&survOrg, &survMerged)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return err
		}
		err = txQueryRowContext(ctx, tx, `SELECT organisation_id, merged_into_contact_id FROM contacts WHERE id = ?`, sourceID.String()).Scan(&srcOrg, &srcMerged)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return sql.ErrNoRows
			}
			return err
		}
		if survOrg != organisationID.String() || srcOrg != organisationID.String() {
			return fmt.Errorf("contacts not in organisation")
		}
		if survMerged.Valid || srcMerged.Valid {
			return fmt.Errorf("contact already merged")
		}

		identRows, err := txQueryContext(ctx, tx, `
			SELECT id, kind, value_normalized, value_raw, created_at
			FROM contact_identities WHERE contact_id = ?
		`, sourceID.String())
		if err != nil {
			return err
		}
		type identMove struct {
			id, kind, norm, raw string
			created             time.Time
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
			err := txQueryRowContext(ctx, tx, `
				SELECT contact_id FROM contact_identities
				WHERE organisation_id = ? AND kind = ? AND value_normalized = ? AND contact_id != ?
			`, organisationID.String(), m.kind, m.norm, sourceID.String()).Scan(&existing)
			if err == nil && existing.Valid {
				if existing.String == survivorID.String() {
					if _, err := txExecContext(ctx, tx, `DELETE FROM contact_identities WHERE id = ?`, m.id); err != nil {
						return err
					}
					continue
				}
				return fmt.Errorf("identity conflict: unique constraint")
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			if _, err := txExecContext(ctx, tx, `UPDATE contact_identities SET contact_id = ? WHERE id = ?`, survivorID.String(), m.id); err != nil {
				return err
			}
		}

		partRows, err := txQueryContext(ctx, tx, `
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
				_ = txQueryRowContext(ctx, tx, `
					SELECT COUNT(1) FROM correspondence_participants
					WHERE contact_id = ? AND role = ? AND message_id = ?
				`, survivorID.String(), p.role, p.messageID.String).Scan(&exists)
				if exists > 0 {
					if _, err := txExecContext(ctx, tx, `DELETE FROM correspondence_participants WHERE id = ?`, p.id); err != nil {
						return err
					}
					continue
				}
			}
			if p.manual.Valid {
				var exists int
				_ = txQueryRowContext(ctx, tx, `
					SELECT COUNT(1) FROM correspondence_participants
					WHERE contact_id = ? AND role = ? AND manual_item_id = ?
				`, survivorID.String(), p.role, p.manual.String).Scan(&exists)
				if exists > 0 {
					if _, err := txExecContext(ctx, tx, `DELETE FROM correspondence_participants WHERE id = ?`, p.id); err != nil {
						return err
					}
					continue
				}
			}
			if _, err := txExecContext(ctx, tx, `UPDATE correspondence_participants SET contact_id = ? WHERE id = ?`, survivorID.String(), p.id); err != nil {
				return err
			}
		}

		_, err = txExecContext(ctx, tx, `
			UPDATE contacts
			SET merged_into_contact_id = ?, updated_at = ?
			WHERE id = ?
		`, survivorID.String(), at.UTC(), sourceID.String())
		return err
	})
}

func (r *Repository) ResolveEmailContact(ctx context.Context, organisationID uuid.UUID, email, displayName string, now time.Time) (uuid.UUID, error) {
	norm := domaincontacts.NormalizeEmail(email)
	if norm == "" {
		return uuid.Nil, fmt.Errorf("empty email")
	}
	existing, err := r.FindContactIdentity(ctx, organisationID, string(domaincontacts.KindEmail), norm)
	if err != nil {
		return uuid.Nil, err
	}
	if existing != nil {
		_ = r.UpdateContactDisplayNameIfEmpty(ctx, organisationID, existing.ContactID, displayName, now)
		return existing.ContactID, nil
	}
	contactID := uuid.New()
	row := driven.ContactRow{
		ID:             contactID,
		OrganisationID: organisationID,
		DisplayName:    strings.TrimSpace(displayName),
		CreatedAt:      now.UTC(),
		UpdatedAt:      now.UTC(),
	}
	if err := r.CreateContact(ctx, row); err != nil {
		return uuid.Nil, err
	}
	ident := driven.ContactIdentityRow{
		ID:              uuid.New(),
		OrganisationID:  organisationID,
		ContactID:       contactID,
		Kind:            string(domaincontacts.KindEmail),
		ValueNormalized: norm,
		ValueRaw:        strings.TrimSpace(email),
		CreatedAt:       now.UTC(),
	}
	if err := r.CreateIdentity(ctx, ident); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			again, findErr := r.FindContactIdentity(ctx, organisationID, string(domaincontacts.KindEmail), norm)
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
	rows, err := r.queryContext(ctx, `SELECT id FROM messages WHERE account_id = ? ORDER BY received_at DESC LIMIT ?`, accountID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func (r *Repository) ListContactIDsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queryContext(ctx, `
		SELECT DISTINCT contact_id FROM correspondence_participants
		WHERE organisation_id = ? AND message_id = ?
	`, organisationID.String(), messageID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func (r *Repository) ListContactIDsForThread(ctx context.Context, organisationID, accountID uuid.UUID, conversationID string) ([]uuid.UUID, error) {
	rows, err := r.queryContext(ctx, `
		SELECT DISTINCT cp.contact_id
		FROM correspondence_participants cp
		INNER JOIN messages m ON m.id = cp.message_id
		WHERE cp.organisation_id = ? AND m.account_id = ? AND m.conversation_id = ?
	`, organisationID.String(), accountID.String(), conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func (r *Repository) ListContactIDsForManualItem(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.queryContext(ctx, `
		SELECT DISTINCT contact_id FROM correspondence_participants
		WHERE organisation_id = ? AND manual_item_id = ?
	`, organisationID.String(), manualItemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanUUIDColumn(rows)
}

func scanContactRow(s rowScanner) (*driven.ContactRow, error) {
	var idStr, orgStr, displayName string
	var company, merged sql.NullString
	var createdAt, updatedAt time.Time
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
	row := &driven.ContactRow{
		ID:             id,
		OrganisationID: orgID,
		DisplayName:    displayName,
		Company:        nullStringPtr(company),
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      updatedAt.UTC(),
	}
	if merged.Valid && merged.String != "" {
		id, err := uuid.Parse(merged.String)
		if err != nil {
			return nil, err
		}
		row.MergedIntoContactID = &id
	}
	return row, nil
}

func scanIdentityRow(s rowScanner) (*driven.ContactIdentityRow, error) {
	var idStr, orgStr, contactStr, kind, norm, raw string
	var createdAt time.Time
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
	return &driven.ContactIdentityRow{
		ID:              id,
		OrganisationID:  orgID,
		ContactID:       contactID,
		Kind:            kind,
		ValueNormalized: norm,
		ValueRaw:        raw,
		CreatedAt:       createdAt.UTC(),
	}, nil
}

func (r *Repository) ListProjects(ctx context.Context, organisationID uuid.UUID, filter driven.ProjectListFilter) ([]driven.ProjectRow, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	q := `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE organisation_id = ?`
	args := []any{organisationID.String()}
	if !filter.IncludeArchived {
		q += ` AND archived_at IS NULL`
	}
	q += ` ORDER BY code ASC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ProjectRow, 0)
	for rows.Next() {
		p, err := scanProjectRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, organisationID, projectID uuid.UUID) (*driven.ProjectRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE id = ? AND organisation_id = ?
	`, projectID.String(), organisationID.String())
	p, err := scanProjectRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) GetProjectByCode(ctx context.Context, organisationID uuid.UUID, code string) (*driven.ProjectRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at
		FROM projects WHERE organisation_id = ? AND code = ?
	`, organisationID.String(), code)
	p, err := scanProjectRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return p, err
}

func (r *Repository) CreateProject(ctx context.Context, project driven.ProjectRow, member driven.ProjectMemberRow) error {
	return r.withTx(ctx, func(tx *sql.Tx) error {
		kw, _ := json.Marshal(project.Keywords)
		if project.Keywords == nil {
			kw = []byte("[]")
		}
		if _, err := txExecContext(ctx, tx, `
			INSERT INTO projects (id, organisation_id, name, code, description, client, keywords_json, archived_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, NULL, ?, ?)
		`, project.ID.String(), project.OrganisationID.String(), project.Name, project.Code, nullStr(project.Description), nullStr(project.Client), string(kw), project.CreatedAt.UTC(), project.UpdatedAt.UTC()); err != nil {
			return err
		}
		_, err := txExecContext(ctx, tx, `
			INSERT INTO project_members (
				id, project_id, user_id, role, discipline, responsibilities, current_scope, approval_authority, out_of_scope, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, member.ID.String(), member.ProjectID.String(), member.UserID.String(), member.Role,
			nullStr(member.Discipline), nullStr(member.Responsibilities), nullStr(member.CurrentScope), nullStr(member.ApprovalAuthority), nullStr(member.OutOfScope),
			member.CreatedAt.UTC(), member.UpdatedAt.UTC())
		return err
	})
}

func (r *Repository) UpdateProject(ctx context.Context, project driven.ProjectRow) error {
	kw, _ := json.Marshal(project.Keywords)
	if project.Keywords == nil {
		kw = []byte("[]")
	}
	_, err := r.execContext(ctx, `
		UPDATE projects SET name = ?, description = ?, client = ?, keywords_json = ?, archived_at = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, project.Name, nullStr(project.Description), nullStr(project.Client), string(kw), nullTime(project.ArchivedAt), project.UpdatedAt.UTC(), project.ID.String(), project.OrganisationID.String())
	return err
}

func (r *Repository) GetProjectMember(ctx context.Context, projectID, userID uuid.UUID) (*driven.ProjectMemberRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, project_id, user_id, role, discipline, responsibilities, current_scope, approval_authority, out_of_scope, created_at, updated_at
		FROM project_members WHERE project_id = ? AND user_id = ?
	`, projectID.String(), userID.String())
	m, err := scanMemberRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (r *Repository) UpdateProjectMember(ctx context.Context, member driven.ProjectMemberRow) error {
	_, err := r.execContext(ctx, `
		UPDATE project_members
		SET role = ?, discipline = ?, responsibilities = ?, current_scope = ?, approval_authority = ?, out_of_scope = ?, updated_at = ?
		WHERE project_id = ? AND user_id = ?
	`, member.Role, nullStr(member.Discipline), nullStr(member.Responsibilities), nullStr(member.CurrentScope), nullStr(member.ApprovalAuthority), nullStr(member.OutOfScope), member.UpdatedAt.UTC(), member.ProjectID.String(), member.UserID.String())
	return err
}

func (r *Repository) UpsertProjectParticipant(ctx context.Context, projectID, contactID uuid.UUID, firstSeenAt time.Time) error {
	_, err := r.execContext(ctx, `
		INSERT INTO project_participants (project_id, contact_id, first_seen_at)
		VALUES (?, ?, ?)
		ON CONFLICT(project_id, contact_id) DO NOTHING
	`, projectID.String(), contactID.String(), firstSeenAt.UTC())
	return err
}

func (r *Repository) CountProjectMembers(ctx context.Context, projectID uuid.UUID) (int, error) {
	var n int
	err := r.queryRowContext(ctx, `SELECT COUNT(1) FROM project_members WHERE project_id = ?`, projectID.String()).Scan(&n)
	return n, err
}

func scanProjectRow(s rowScanner) (*driven.ProjectRow, error) {
	var idStr, orgStr, name, code, kwJSON string
	var desc, client sql.NullString
	var archived sql.NullTime
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &name, &code, &desc, &client, &kwJSON, &archived, &createdAt, &updatedAt); err != nil {
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
	var keywords []string
	if strings.TrimSpace(kwJSON) != "" {
		_ = json.Unmarshal([]byte(kwJSON), &keywords)
	}
	if keywords == nil {
		keywords = []string{}
	}
	p := &driven.ProjectRow{
		ID:             id,
		OrganisationID: orgID,
		Name:           name,
		Code:           code,
		Description:    nullStringPtr(desc),
		Client:         nullStringPtr(client),
		Keywords:       keywords,
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      updatedAt.UTC(),
	}
	p.ArchivedAt = nullTimePtr(archived)
	return p, nil
}

func scanMemberRow(s rowScanner) (*driven.ProjectMemberRow, error) {
	var idStr, projectStr, userStr, role string
	var discipline, responsibilities, currentScope, approvalAuthority, outOfScope sql.NullString
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &projectStr, &userStr, &role, &discipline, &responsibilities, &currentScope, &approvalAuthority, &outOfScope, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	projectID, _ := uuid.Parse(projectStr)
	userID, _ := uuid.Parse(userStr)
	return &driven.ProjectMemberRow{
		ID:                id,
		ProjectID:         projectID,
		UserID:            userID,
		Role:              role,
		Discipline:        nullStringPtr(discipline),
		Responsibilities:  nullStringPtr(responsibilities),
		CurrentScope:      nullStringPtr(currentScope),
		ApprovalAuthority: nullStringPtr(approvalAuthority),
		OutOfScope:        nullStringPtr(outOfScope),
		CreatedAt:         createdAt.UTC(),
		UpdatedAt:         updatedAt.UTC(),
	}, nil
}

func (r *Repository) UpsertThreadAssignment(ctx context.Context, row driven.AssignmentRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO thread_assignments (
			id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, conversation_id) DO UPDATE SET
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.OrganisationID.String(), row.AccountID.String(), row.ConversationID, nullUUID(row.ProjectID), row.Status, nullFloat(row.Confidence), row.Reason, row.Source, nullUUID(row.RunID), nullUUID(row.AssignedByUserID), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) GetThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) (*driven.AssignmentRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, account_id, conversation_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		FROM thread_assignments WHERE account_id = ? AND conversation_id = ?
	`, accountID.String(), conversationID)
	out, err := scanThreadAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) DeleteThreadAssignment(ctx context.Context, accountID uuid.UUID, conversationID string) error {
	_, err := r.execContext(ctx, `DELETE FROM thread_assignments WHERE account_id = ? AND conversation_id = ?`, accountID.String(), conversationID)
	return err
}

func (r *Repository) UpsertMessageOverride(ctx context.Context, row driven.AssignmentRow) error {
	if row.MessageID == nil {
		return fmt.Errorf("message override requires message_id")
	}
	_, err := r.execContext(ctx, `
		INSERT INTO message_assignment_overrides (
			message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(message_id) DO UPDATE SET
			project_id = excluded.project_id,
			status = excluded.status,
			confidence = excluded.confidence,
			reason = excluded.reason,
			source = excluded.source,
			run_id = excluded.run_id,
			assigned_by_user_id = excluded.assigned_by_user_id,
			updated_at = excluded.updated_at
	`, row.MessageID.String(), row.OrganisationID.String(), row.AccountID.String(), nullUUID(row.ProjectID), row.Status, nullFloat(row.Confidence), row.Reason, row.Source, nullUUID(row.RunID), nullUUID(row.AssignedByUserID), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) GetMessageOverride(ctx context.Context, messageID uuid.UUID) (*driven.AssignmentRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT message_id, organisation_id, account_id, project_id, status, confidence, reason, source,
			run_id, assigned_by_user_id, created_at, updated_at
		FROM message_assignment_overrides WHERE message_id = ?
	`, messageID.String())
	out, err := scanOverrideAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) DeleteMessageOverride(ctx context.Context, messageID uuid.UUID) error {
	_, err := r.execContext(ctx, `DELETE FROM message_assignment_overrides WHERE message_id = ?`, messageID.String())
	return err
}

func (r *Repository) EffectiveAssignment(ctx context.Context, userID, messageID uuid.UUID) (*driven.EffectiveAssignment, error) {
	msg, err := r.GetMessage(ctx, userID, messageID)
	if err != nil || msg == nil {
		return msgEffectiveNil(msg, err)
	}
	out := &driven.EffectiveAssignment{Status: "unassigned", Scope: "none", AccountID: msg.AccountID, MessageID: msg.ID}
	if msg.ConversationID != nil {
		out.ConversationID = msg.ConversationID
	}
	ov, err := r.GetMessageOverride(ctx, messageID)
	if err != nil {
		return nil, err
	}
	if ov != nil {
		out.ProjectID = ov.ProjectID
		out.Status = ov.Status
		if ov.ProjectID == nil {
			out.Status = "unassigned"
		}
		out.Reason = ov.Reason
		out.Source = ov.Source
		out.Scope = "message"
		return out, nil
	}
	if msg.ConversationID != nil && strings.TrimSpace(*msg.ConversationID) != "" {
		th, err := r.GetThreadAssignment(ctx, msg.AccountID, *msg.ConversationID)
		if err != nil {
			return nil, err
		}
		if th != nil {
			out.ProjectID = th.ProjectID
			out.Status = th.Status
			if th.ProjectID == nil {
				out.Status = "unassigned"
			}
			out.Reason = th.Reason
			out.Source = th.Source
			out.Scope = "thread"
		}
	}
	return out, nil
}

func msgEffectiveNil(msg *driven.MessageRow, err error) (*driven.EffectiveAssignment, error) {
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}
	return nil, nil
}

func (r *Repository) ListUnassigned(ctx context.Context, userID uuid.UUID, filter driven.UnassignedListFilter) ([]driven.UnassignedItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	status := strings.TrimSpace(strings.ToLower(filter.Status))
	if status == "" {
		status = "all"
	}
	out := make([]driven.UnassignedItem, 0)
	rows, err := r.queryContext(ctx, `
		SELECT m.id, m.account_id, a.label, m.subject, m.from_json, m.conversation_id, m.received_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		ORDER BY m.received_at DESC
		LIMIT ?
	`, userID.String(), defaultTimelineCandidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var idStr, accountStr, label, subject, fromJSON string
		var conversation sql.NullString
		var receivedAt time.Time
		if err := rows.Scan(&idStr, &accountStr, &label, &subject, &fromJSON, &conversation, &receivedAt); err != nil {
			return nil, err
		}
		id, _ := uuid.Parse(idStr)
		accountID, _ := uuid.Parse(accountStr)
		eff, err := r.EffectiveAssignment(ctx, userID, id)
		if err != nil || eff == nil {
			continue
		}
		itemStatus := "unassigned"
		if eff.ProjectID != nil && eff.Status == "provisional" {
			itemStatus = "provisional"
		} else if eff.ProjectID != nil && eff.Status == "committed" {
			continue
		}
		if status == "unassigned" && itemStatus != "unassigned" {
			continue
		}
		if status == "provisional" && itemStatus != "provisional" {
			continue
		}
		msgID, accID := id, accountID
		out = append(out, driven.UnassignedItem{
			Kind: "message", MessageID: &msgID, AccountID: &accID, AccountLabel: label,
			Subject: subject, FromJSON: fromJSON, ConversationID: nullStringPtr(conversation),
			OccurredAt: receivedAt.UTC(), Status: itemStatus, Reason: eff.Reason, ProjectID: eff.ProjectID, Source: eff.Source,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	orgID, err := r.GetHomeOrganisationID(ctx, userID)
	if err == nil {
		manuals, err := r.ListUnassignedManualItems(ctx, orgID, 500)
		if err == nil {
			for _, m := range manuals {
				itemStatus := "unassigned"
				if m.AssignmentStatus == "provisional" && m.ProjectID != nil {
					itemStatus = "provisional"
				} else if m.AssignmentStatus == "committed" && m.ProjectID != nil {
					continue
				}
				if status == "unassigned" && itemStatus != "unassigned" {
					continue
				}
				if status == "provisional" && itemStatus != "provisional" {
					continue
				}
				mid := m.ID
				reason := ""
				if m.AssignmentReason != nil {
					reason = *m.AssignmentReason
				}
				source := ""
				if m.AssignmentSource != nil {
					source = *m.AssignmentSource
				}
				out = append(out, driven.UnassignedItem{
					Kind: "manual", ManualItemID: &mid, Subject: m.Title, Channel: m.Channel, OccurredAt: m.OccurredAt,
					Status: itemStatus, Reason: reason, ProjectID: m.ProjectID, Source: source,
				})
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.After(out[j].OccurredAt) })
	if offset >= len(out) {
		return []driven.UnassignedItem{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *Repository) CountUnassignedSummary(ctx context.Context, userID uuid.UUID) (driven.UnassignedSummary, error) {
	items, err := r.ListUnassigned(ctx, userID, driven.UnassignedListFilter{Status: "all", Limit: 2000})
	if err != nil {
		return driven.UnassignedSummary{}, err
	}
	var sum driven.UnassignedSummary
	for _, it := range items {
		if it.Status == "provisional" {
			sum.Provisional++
		} else {
			sum.Unassigned++
		}
	}
	return sum, nil
}

func (r *Repository) ListMessagesNeedingAssign(ctx context.Context, userID, accountID uuid.UUID, limit int) ([]driven.MessageRow, error) {
	if limit <= 0 {
		limit = 500
	}
	msgs, err := r.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]driven.MessageRow, 0)
	for _, m := range msgs {
		eff, err := r.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil || eff == nil {
			continue
		}
		if eff.ProjectID != nil || eff.Scope != "none" {
			continue
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *Repository) FindCommittedSiblingProject(ctx context.Context, userID, accountID uuid.UUID, conversationID string, excludeMessageID uuid.UUID) (*uuid.UUID, error) {
	if strings.TrimSpace(conversationID) == "" {
		return nil, nil
	}
	msgs, err := r.ListMessages(ctx, userID, driven.MessageListFilter{AccountID: &accountID, Limit: 200})
	if err != nil {
		return nil, err
	}
	for _, m := range msgs {
		if m.ID == excludeMessageID || m.ConversationID == nil || *m.ConversationID != conversationID {
			continue
		}
		eff, err := r.EffectiveAssignment(ctx, userID, m.ID)
		if err != nil || eff == nil {
			continue
		}
		if eff.ProjectID != nil && eff.Status == "committed" {
			return eff.ProjectID, nil
		}
	}
	return nil, nil
}

func scanThreadAssignment(s rowScanner) (*driven.AssignmentRow, error) {
	var idStr, orgStr, accountStr, conversationID, status, reason, source string
	var projectID, runID, assignedBy sql.NullString
	var confidence sql.NullFloat64
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &accountStr, &conversationID, &projectID, &status, &confidence, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return buildAssignmentRow(idStr, orgStr, accountStr, conversationID, nil, projectID, status, confidence, reason, source, runID, assignedBy, createdAt, updatedAt)
}

func scanOverrideAssignment(s rowScanner) (*driven.AssignmentRow, error) {
	var messageStr, orgStr, accountStr, status, reason, source string
	var projectID, runID, assignedBy sql.NullString
	var confidence sql.NullFloat64
	var createdAt, updatedAt time.Time
	if err := s.Scan(&messageStr, &orgStr, &accountStr, &projectID, &status, &confidence, &reason, &source, &runID, &assignedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	messageID, err := uuid.Parse(messageStr)
	if err != nil {
		return nil, err
	}
	return buildAssignmentRow(uuid.Nil.String(), orgStr, accountStr, "", &messageID, projectID, status, confidence, reason, source, runID, assignedBy, createdAt, updatedAt)
}

func buildAssignmentRow(idStr, orgStr, accountStr, conversationID string, messageID *uuid.UUID, projectID sql.NullString, status string, confidence sql.NullFloat64, reason, source string, runID, assignedBy sql.NullString, createdAt, updatedAt time.Time) (*driven.AssignmentRow, error) {
	var (
		id  uuid.UUID
		err error
	)
	if idStr != "" && idStr != uuid.Nil.String() {
		if id, err = uuid.Parse(idStr); err != nil {
			return nil, err
		}
	} else if messageID != nil {
		id = *messageID
	}
	orgID, err := uuid.Parse(orgStr)
	if err != nil {
		return nil, err
	}
	accountID, err := uuid.Parse(accountStr)
	if err != nil {
		return nil, err
	}
	row := &driven.AssignmentRow{
		ID:             id,
		OrganisationID: orgID,
		AccountID:      accountID,
		ConversationID: conversationID,
		MessageID:      messageID,
		Status:         status,
		Reason:         reason,
		Source:         source,
		CreatedAt:      createdAt.UTC(),
		UpdatedAt:      updatedAt.UTC(),
	}
	if projectID.Valid && projectID.String != "" {
		id, err := uuid.Parse(projectID.String)
		if err != nil {
			return nil, err
		}
		row.ProjectID = &id
	}
	if confidence.Valid {
		v := confidence.Float64
		row.Confidence = &v
	}
	if runID.Valid && runID.String != "" {
		id, err := uuid.Parse(runID.String)
		if err != nil {
			return nil, err
		}
		row.RunID = &id
	}
	if assignedBy.Valid && assignedBy.String != "" {
		id, err := uuid.Parse(assignedBy.String)
		if err != nil {
			return nil, err
		}
		row.AssignedByUserID = &id
	}
	return row, nil
}

func (r *Repository) CreateManualItem(ctx context.Context, row driven.ManualItemRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO manual_items (
			id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.Channel, row.OccurredAt.UTC(), row.Title, row.BodyText, nullUUID(row.ProjectID), row.AssignmentStatus, nullStr(row.AssignmentReason), nullStr(row.AssignmentSource), row.CreatedByUserID.String(), row.CreatedAt.UTC())
	return err
}

func (r *Repository) GetManualItem(ctx context.Context, organisationID, id uuid.UUID) (*driven.ManualItemRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	item, err := scanManualItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) UpdateManualItemAssignment(ctx context.Context, organisationID, id uuid.UUID, projectID *uuid.UUID, status, reason, source string) error {
	res, err := r.execContext(ctx, `
		UPDATE manual_items
		SET project_id = ?, assignment_status = ?, assignment_reason = ?, assignment_source = ?
		WHERE id = ? AND organisation_id = ?
	`, nullUUID(projectID), status, reason, source, id.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListManualItemsForProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.ManualItemRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items
		WHERE organisation_id = ? AND project_id = ?
		ORDER BY occurred_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManualItemRows(rows)
}

func (r *Repository) ListUnassignedManualItems(ctx context.Context, organisationID uuid.UUID, limit int) ([]driven.ManualItemRow, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := r.queryContext(ctx, `
		SELECT id, organisation_id, channel, occurred_at, title, body_text, project_id,
			assignment_status, assignment_reason, assignment_source, created_by_user_id, created_at
		FROM manual_items
		WHERE organisation_id = ?
		  AND (
			assignment_status = 'unassigned'
			OR project_id IS NULL
			OR assignment_status = 'provisional'
		  )
		ORDER BY occurred_at DESC
		LIMIT ?
	`, organisationID.String(), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanManualItemRows(rows)
}

func scanManualItemRows(rows *sql.Rows) ([]driven.ManualItemRow, error) {
	out := make([]driven.ManualItemRow, 0)
	for rows.Next() {
		item, err := scanManualItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanManualItem(s rowScanner) (*driven.ManualItemRow, error) {
	var idStr, orgStr, channel, title, body, status, createdBy string
	var occurredAt, createdAt time.Time
	var projectID, reason, source sql.NullString
	if err := s.Scan(&idStr, &orgStr, &channel, &occurredAt, &title, &body, &projectID, &status, &reason, &source, &createdBy, &createdAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	userID, _ := uuid.Parse(createdBy)
	row := &driven.ManualItemRow{
		ID:               id,
		OrganisationID:   orgID,
		Channel:          channel,
		OccurredAt:       occurredAt.UTC(),
		Title:            title,
		BodyText:         body,
		AssignmentStatus: status,
		AssignmentReason: nullStringPtr(reason),
		AssignmentSource: nullStringPtr(source),
		CreatedByUserID:  userID,
		CreatedAt:        createdAt.UTC(),
	}
	if projectID.Valid && projectID.String != "" {
		id, err := uuid.Parse(projectID.String)
		if err != nil {
			return nil, err
		}
		row.ProjectID = &id
	}
	return row, nil
}

func (r *Repository) ListProjectTimeline(ctx context.Context, userID, organisationID, projectID uuid.UUID, filter driven.TimelineFilter) ([]driven.TimelineItem, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	source := strings.TrimSpace(strings.ToLower(filter.Source))
	if source == "" {
		source = "all"
	}
	out := make([]driven.TimelineItem, 0)
	if source == "all" || source == "mail" {
		items, err := r.listMailTimelineItems(ctx, userID, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if source == "all" || source == "manual" {
		manuals, err := r.ListManualItemsForProject(ctx, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		for _, m := range manuals {
			item := driven.TimelineItem{
				Source: "manual", OccurredAt: m.OccurredAt, Title: m.Title, Snippet: snippetText(m.BodyText, 160), Channel: m.Channel, BodyText: m.BodyText,
			}
			id := m.ID
			item.ManualItemID = &id
			contacts, _ := r.timelineContactsForManual(ctx, organisationID, m.ID)
			item.Contacts = contacts
			if issueID, err := r.FindIssueIDByManualItem(ctx, m.ID); err == nil {
				item.IssueID = issueID
			}
			out = append(out, item)
		}
	}
	if source == "all" || source == "slack" {
		messages, err := r.ListConnectorMessagesForProject(ctx, userID, organisationID, projectID)
		if err != nil {
			return nil, err
		}
		accountLabels := map[uuid.UUID]string{}
		for _, message := range messages {
			label, ok := accountLabels[message.ConnectorAccountID]
			if !ok {
				account, err := r.GetConnectorAccount(ctx, userID, message.ConnectorAccountID)
				if err != nil {
					return nil, err
				}
				if account != nil {
					label = account.Label
				}
				accountLabels[message.ConnectorAccountID] = label
			}
			messageID := message.ID
			accountID := message.ConnectorAccountID
			out = append(out, driven.TimelineItem{
				Source: "slack", OccurredAt: message.OccurredAt, Title: message.Title, Snippet: snippetText(message.BodyText, 160),
				BodyText: message.BodyText, AccountLabel: label, ConnectorMessageID: &messageID, ConnectorAccountID: &accountID, Channel: message.ExternalChannelID,
			})
		}
	}
	if filter.UnassignedToIssue {
		filtered := make([]driven.TimelineItem, 0, len(out))
		for _, item := range out {
			if item.IssueID == nil {
				filtered = append(filtered, item)
			}
		}
		out = filtered
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OccurredAt.After(out[j].OccurredAt) })
	if offset >= len(out) {
		return []driven.TimelineItem{}, nil
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], nil
}

func (r *Repository) listMailTimelineItems(ctx context.Context, userID, organisationID, projectID uuid.UUID) ([]driven.TimelineItem, error) {
	rows, err := r.queryContext(ctx, `
		SELECT m.id, m.account_id, a.label, m.subject, m.body_text, m.received_at
		FROM messages m
		INNER JOIN accounts a ON a.id = m.account_id AND a.user_id = ?
		LEFT JOIN message_assignment_overrides o ON o.message_id = m.id
		LEFT JOIN thread_assignments t
			ON t.account_id = m.account_id
			AND m.conversation_id IS NOT NULL
			AND m.conversation_id != ''
			AND t.conversation_id = m.conversation_id
		WHERE
			CASE
				WHEN o.message_id IS NOT NULL THEN o.project_id = ?
				ELSE t.project_id = ?
			END
		ORDER BY m.received_at DESC
		LIMIT 500
	`, userID.String(), projectID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.TimelineItem, 0)
	for rows.Next() {
		var idStr, accountStr, label, subject string
		var body sql.NullString
		var receivedAt time.Time
		if err := rows.Scan(&idStr, &accountStr, &label, &subject, &body, &receivedAt); err != nil {
			return nil, err
		}
		msgID, _ := uuid.Parse(idStr)
		accountID, _ := uuid.Parse(accountStr)
		bodyText := ""
		if body.Valid {
			bodyText = body.String
		}
		item := driven.TimelineItem{Source: "mail", OccurredAt: receivedAt.UTC(), Title: subject, Snippet: snippetText(bodyText, 160), AccountLabel: label}
		item.AccountID = &accountID
		item.MessageID = &msgID
		contacts, _ := r.timelineContactsForMessage(ctx, organisationID, msgID)
		item.Contacts = contacts
		if issueID, err := r.FindIssueIDByMessage(ctx, msgID); err == nil {
			item.IssueID = issueID
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (r *Repository) timelineContactsForMessage(ctx context.Context, organisationID, messageID uuid.UUID) ([]driven.TimelineContact, error) {
	rows, err := r.queryContext(ctx, `
		SELECT c.id, c.display_name, cp.role
		FROM correspondence_participants cp
		INNER JOIN contacts c ON c.id = cp.contact_id
		WHERE cp.organisation_id = ? AND cp.message_id = ?
		ORDER BY cp.role ASC
	`, organisationID.String(), messageID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTimelineContacts(rows)
}

func (r *Repository) timelineContactsForManual(ctx context.Context, organisationID, manualItemID uuid.UUID) ([]driven.TimelineContact, error) {
	rows, err := r.queryContext(ctx, `
		SELECT c.id, c.display_name, cp.role
		FROM correspondence_participants cp
		INNER JOIN contacts c ON c.id = cp.contact_id
		WHERE cp.organisation_id = ? AND cp.manual_item_id = ?
		ORDER BY cp.role ASC
	`, organisationID.String(), manualItemID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTimelineContacts(rows)
}

func scanTimelineContacts(rows *sql.Rows) ([]driven.TimelineContact, error) {
	out := make([]driven.TimelineContact, 0)
	for rows.Next() {
		var idStr, name, role string
		if err := rows.Scan(&idStr, &name, &role); err != nil {
			return nil, err
		}
		id, err := uuid.Parse(idStr)
		if err != nil {
			return nil, err
		}
		out = append(out, driven.TimelineContact{ID: id, DisplayName: name, Role: role})
	}
	return out, rows.Err()
}

func snippetText(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max]) + "..."
}

func (r *Repository) CreateIssue(ctx context.Context, row driven.IssueRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO issues (id, organisation_id, project_id, title, current_position_note, status, assignee_user_id, assignee_contact_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), row.Title, row.CurrentPositionNote, row.Status, nullUUID(row.AssigneeUserID), nullUUID(row.AssigneeContactID), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) GetIssue(ctx context.Context, organisationID, issueID uuid.UUID) (*driven.IssueRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, title, current_position_note, status, assignee_user_id, assignee_contact_id, created_at, updated_at
		FROM issues WHERE id = ? AND organisation_id = ?
	`, issueID.String(), organisationID.String())
	item, err := scanIssueRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListIssuesByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.IssueRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, organisation_id, project_id, title, current_position_note, status, assignee_user_id, assignee_contact_id, created_at, updated_at
		FROM issues WHERE organisation_id = ? AND project_id = ?
		ORDER BY updated_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.IssueRow, 0)
	for rows.Next() {
		item, err := scanIssueRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateIssue(ctx context.Context, row driven.IssueRow) error {
	res, err := r.execContext(ctx, `
		UPDATE issues SET title = ?, current_position_note = ?, status = ?, assignee_user_id = ?, assignee_contact_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, row.Title, row.CurrentPositionNote, row.Status, nullUUID(row.AssigneeUserID), nullUUID(row.AssigneeContactID), row.UpdatedAt.UTC(), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) AddIssueItem(ctx context.Context, row driven.IssueItemRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO issue_items (id, issue_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.IssueID.String(), nullUUID(row.MessageID), nullUUID(row.ManualItemID), row.AddedAt.UTC())
	return err
}

func (r *Repository) RemoveIssueItem(ctx context.Context, organisationID, issueID, itemID uuid.UUID) error {
	res, err := r.execContext(ctx, `
		DELETE FROM issue_items
		WHERE id = ? AND issue_id = ? AND issue_id IN (SELECT id FROM issues WHERE organisation_id = ?)
	`, itemID.String(), issueID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetIssueItem(ctx context.Context, organisationID, itemID uuid.UUID) (*driven.IssueItemRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT ii.id, ii.issue_id, ii.message_id, ii.manual_item_id, ii.added_at
		FROM issue_items ii
		INNER JOIN issues i ON i.id = ii.issue_id
		WHERE ii.id = ? AND i.organisation_id = ?
	`, itemID.String(), organisationID.String())
	item, err := scanIssueItem(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListIssueItems(ctx context.Context, issueID uuid.UUID) ([]driven.IssueItemRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, issue_id, message_id, manual_item_id, added_at
		FROM issue_items WHERE issue_id = ?
		ORDER BY added_at ASC
	`, issueID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.IssueItemRow, 0)
	for rows.Next() {
		item, err := scanIssueItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) FindIssueIDByMessage(ctx context.Context, messageID uuid.UUID) (*uuid.UUID, error) {
	var idStr string
	err := r.queryRowContext(ctx, `SELECT issue_id FROM issue_items WHERE message_id = ?`, messageID.String()).Scan(&idStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func (r *Repository) FindIssueIDByManualItem(ctx context.Context, manualItemID uuid.UUID) (*uuid.UUID, error) {
	var idStr string
	err := r.queryRowContext(ctx, `SELECT issue_id FROM issue_items WHERE manual_item_id = ?`, manualItemID.String()).Scan(&idStr)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

func scanIssueRow(s rowScanner) (*driven.IssueRow, error) {
	var idStr, orgStr, projectStr, title, note, status string
	var assigneeUser, assigneeContact sql.NullString
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &projectStr, &title, &note, &status, &assigneeUser, &assigneeContact, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	projectID, _ := uuid.Parse(projectStr)
	row := &driven.IssueRow{ID: id, OrganisationID: orgID, ProjectID: projectID, Title: title, CurrentPositionNote: note, Status: status, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	if row.AssigneeUserID, _ = scanUUID(assigneeUser); false {
	}
	contactID, err := scanUUID(assigneeContact)
	if err != nil {
		return nil, err
	}
	row.AssigneeContactID = contactID
	return row, nil
}

func scanIssueItem(s rowScanner) (*driven.IssueItemRow, error) {
	var idStr, issueStr string
	var messageID, manualItemID sql.NullString
	var addedAt time.Time
	if err := s.Scan(&idStr, &issueStr, &messageID, &manualItemID, &addedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	issueID, _ := uuid.Parse(issueStr)
	row := &driven.IssueItemRow{ID: id, IssueID: issueID, AddedAt: addedAt.UTC()}
	if row.MessageID, _ = scanUUID(messageID); false {
	}
	row.ManualItemID, _ = scanUUID(manualItemID)
	return row, nil
}

func (r *Repository) CreateFact(ctx context.Context, row driven.FactRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO facts (id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), nullUUID(row.IssueID), row.SubjectKey, row.Label, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) UpdateFact(ctx context.Context, row driven.FactRow) error {
	res, err := r.execContext(ctx, `
		UPDATE facts SET issue_id = ?, label = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, nullUUID(row.IssueID), row.Label, row.UpdatedAt.UTC(), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) GetFact(ctx context.Context, organisationID, factID uuid.UUID) (*driven.FactRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE id = ? AND organisation_id = ?
	`, factID.String(), organisationID.String())
	out, err := scanFactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) GetFactBySubject(ctx context.Context, organisationID, projectID uuid.UUID, subjectKey string) (*driven.FactRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE organisation_id = ? AND project_id = ? AND subject_key = ?
	`, organisationID.String(), projectID.String(), subjectKey)
	out, err := scanFactRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListFactsByProject(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.FactRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, subject_key, label, created_at, updated_at
		FROM facts WHERE organisation_id = ? AND project_id = ?
		ORDER BY label ASC, subject_key ASC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FactRow, 0)
	for rows.Next() {
		item, err := scanFactRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) CreateFactVersion(ctx context.Context, row driven.FactVersionRow) error {
	return withSerializableWrite(ctx, func() error {
		_, err := r.execContext(ctx, `
			INSERT INTO fact_versions (
				id, fact_id, status, value_json, value_text, unit, source, confidence, interpretation_id,
				supersedes_version_id, superseded_by_version_id, superseded_at, created_by_user_id, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, row.ID.String(), row.FactID.String(), row.Status, row.ValueJSON, row.ValueText, nullStr(row.Unit), row.Source, nullFloat(row.Confidence),
			nullUUID(row.InterpretationID), nullUUID(row.SupersedesVersionID), nullUUID(row.SupersededByVersionID), nullTime(row.SupersededAt), nullUUID(row.CreatedByUserID), row.CreatedAt.UTC())
		return err
	})
}

func (r *Repository) GetFactVersion(ctx context.Context, organisationID, versionID uuid.UUID) (*driven.FactVersionRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT fv.id, fv.fact_id, fv.status, fv.value_json, fv.value_text, fv.unit, fv.source, fv.confidence, fv.interpretation_id,
			fv.supersedes_version_id, fv.superseded_by_version_id, fv.superseded_at, fv.created_by_user_id, fv.created_at
		FROM fact_versions fv
		INNER JOIN facts f ON f.id = fv.fact_id
		WHERE fv.id = ? AND f.organisation_id = ?
	`, versionID.String(), organisationID.String())
	out, err := scanFactVersionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) ListFactVersions(ctx context.Context, factID uuid.UUID) ([]driven.FactVersionRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, fact_id, status, value_json, value_text, unit, source, confidence, interpretation_id,
			supersedes_version_id, superseded_by_version_id, superseded_at, created_by_user_id, created_at
		FROM fact_versions WHERE fact_id = ?
		ORDER BY created_at ASC
	`, factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.FactVersionRow, 0)
	for rows.Next() {
		item, err := scanFactVersionRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetActiveFactVersion(ctx context.Context, factID uuid.UUID) (*driven.FactVersionRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, fact_id, status, value_json, value_text, unit, source, confidence, interpretation_id,
			supersedes_version_id, superseded_by_version_id, superseded_at, created_by_user_id, created_at
		FROM fact_versions WHERE fact_id = ? AND status = 'active'
	`, factID.String())
	out, err := scanFactVersionRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return out, err
}

func (r *Repository) UpdateFactVersion(ctx context.Context, row driven.FactVersionRow) error {
	return withSerializableWrite(ctx, func() error {
		res, err := r.execContext(ctx, `
			UPDATE fact_versions SET status = ?, value_json = ?, value_text = ?, unit = ?, source = ?, confidence = ?,
				interpretation_id = ?, supersedes_version_id = ?, superseded_by_version_id = ?, superseded_at = ?, created_by_user_id = ?
			WHERE id = ?
		`, row.Status, row.ValueJSON, row.ValueText, nullStr(row.Unit), row.Source, nullFloat(row.Confidence), nullUUID(row.InterpretationID),
			nullUUID(row.SupersedesVersionID), nullUUID(row.SupersededByVersionID), nullTime(row.SupersededAt), nullUUID(row.CreatedByUserID), row.ID.String())
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			return sql.ErrNoRows
		}
		return nil
	})
}

func withSerializableWrite(ctx context.Context, fn func() error) error {
	_, err := withSerializableRetry(ctx, func(context.Context) (struct{}, error) {
		return struct{}{}, fn()
	})
	return err
}

func (r *Repository) AddFactEvidence(ctx context.Context, row driven.FactEvidenceRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO fact_evidence (id, fact_version_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.FactVersionID.String(), nullUUID(row.MessageID), nullUUID(row.ManualItemID), row.AddedAt.UTC())
	return err
}

func (r *Repository) RemoveFactEvidence(ctx context.Context, organisationID, versionID, evidenceID uuid.UUID) error {
	res, err := r.execContext(ctx, `
		DELETE FROM fact_evidence
		WHERE id = ? AND fact_version_id = ?
		  AND fact_version_id IN (
			SELECT fv.id FROM fact_versions fv
			INNER JOIN facts f ON f.id = fv.fact_id
			WHERE f.organisation_id = ?
		  )
	`, evidenceID.String(), versionID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListFactEvidence(ctx context.Context, versionID uuid.UUID) ([]driven.FactEvidenceRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, fact_version_id, message_id, manual_item_id, added_at
		FROM fact_evidence WHERE fact_version_id = ?
		ORDER BY added_at ASC
	`, versionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactEvidenceRows(rows)
}

func (r *Repository) ListFactEvidenceForFact(ctx context.Context, factID uuid.UUID) ([]driven.FactEvidenceRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT fe.id, fe.fact_version_id, fe.message_id, fe.manual_item_id, fe.added_at
		FROM fact_evidence fe
		INNER JOIN fact_versions fv ON fv.id = fe.fact_version_id
		WHERE fv.fact_id = ?
		ORDER BY fe.added_at ASC
	`, factID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanFactEvidenceRows(rows)
}

func scanFactRow(s rowScanner) (*driven.FactRow, error) {
	var idStr, orgStr, projectStr, subjectKey, label string
	var issueID sql.NullString
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &projectStr, &issueID, &subjectKey, &label, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	projectID, _ := uuid.Parse(projectStr)
	row := &driven.FactRow{ID: id, OrganisationID: orgID, ProjectID: projectID, SubjectKey: subjectKey, Label: label, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	row.IssueID, _ = scanUUID(issueID)
	return row, nil
}

func scanFactVersionRow(s rowScanner) (*driven.FactVersionRow, error) {
	var idStr, factStr, status, valueJSON, valueText, source string
	var unit, interpretationID, supersedesID, supersededByID, createdBy sql.NullString
	var confidence sql.NullFloat64
	var supersededAt sql.NullTime
	var createdAt time.Time
	if err := s.Scan(&idStr, &factStr, &status, &valueJSON, &valueText, &unit, &source, &confidence, &interpretationID, &supersedesID, &supersededByID, &supersededAt, &createdBy, &createdAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	factID, _ := uuid.Parse(factStr)
	row := &driven.FactVersionRow{ID: id, FactID: factID, Status: status, ValueJSON: valueJSON, ValueText: valueText, Unit: nullStringPtr(unit), Source: source, CreatedAt: createdAt.UTC(), SupersededAt: nullTimePtr(supersededAt)}
	if confidence.Valid {
		v := confidence.Float64
		row.Confidence = &v
	}
	row.InterpretationID, _ = scanUUID(interpretationID)
	row.SupersedesVersionID, _ = scanUUID(supersedesID)
	row.SupersededByVersionID, _ = scanUUID(supersededByID)
	row.CreatedByUserID, _ = scanUUID(createdBy)
	return row, nil
}

func scanFactEvidenceRows(rows *sql.Rows) ([]driven.FactEvidenceRow, error) {
	out := make([]driven.FactEvidenceRow, 0)
	for rows.Next() {
		item, err := scanFactEvidenceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanFactEvidenceRow(s rowScanner) (*driven.FactEvidenceRow, error) {
	var idStr, versionStr string
	var messageID, manualItemID sql.NullString
	var addedAt time.Time
	if err := s.Scan(&idStr, &versionStr, &messageID, &manualItemID, &addedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	versionID, _ := uuid.Parse(versionStr)
	row := &driven.FactEvidenceRow{ID: id, FactVersionID: versionID, AddedAt: addedAt.UTC()}
	row.MessageID, _ = scanUUID(messageID)
	row.ManualItemID, _ = scanUUID(manualItemID)
	return row, nil
}

func (r *Repository) CreateInterpretation(ctx context.Context, row driven.InterpretationRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO interpretations (id, organisation_id, project_id, account_id, run_id, status, payload_json, confidence, reason, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), nullUUID(row.AccountID), nullUUID(row.RunID), row.Status, row.PayloadJSON, nullFloat(row.Confidence), row.Reason, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) AddInterpretationSource(ctx context.Context, row driven.InterpretationSourceRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO interpretation_sources (id, interpretation_id, message_id, manual_item_id, connector_message_id)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.InterpretationID.String(), nullUUID(row.MessageID), nullUUID(row.ManualItemID), nullUUID(row.ConnectorMessageID))
	return err
}

func (r *Repository) GetInterpretation(ctx context.Context, organisationID, id uuid.UUID) (*driven.InterpretationRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, account_id, run_id, status, payload_json, confidence, reason, created_at, updated_at
		FROM interpretations WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	item, err := scanInterpretationRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListPendingInterpretations(ctx context.Context, organisationID, projectID uuid.UUID) ([]driven.InterpretationRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, organisation_id, project_id, account_id, run_id, status, payload_json, confidence, reason, created_at, updated_at
		FROM interpretations
		WHERE organisation_id = ? AND project_id = ? AND status = 'pending'
		ORDER BY created_at DESC
	`, organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.InterpretationRow, 0)
	for rows.Next() {
		item, err := scanInterpretationRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateInterpretationStatus(ctx context.Context, organisationID, id uuid.UUID, status string, updatedAt time.Time) error {
	res, err := r.execContext(ctx, `
		UPDATE interpretations SET status = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, status, updatedAt.UTC(), id.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListInterpretationSources(ctx context.Context, interpretationID uuid.UUID) ([]driven.InterpretationSourceRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, interpretation_id, message_id, manual_item_id, connector_message_id
		FROM interpretation_sources WHERE interpretation_id = ?
	`, interpretationID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.InterpretationSourceRow, 0)
	for rows.Next() {
		item, err := scanInterpretationSource(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanInterpretationRow(s rowScanner) (*driven.InterpretationRow, error) {
	var idStr, orgStr, projectStr, status, payloadJSON, reason string
	var accountID, runID sql.NullString
	var confidence sql.NullFloat64
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &projectStr, &accountID, &runID, &status, &payloadJSON, &confidence, &reason, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	projectID, _ := uuid.Parse(projectStr)
	row := &driven.InterpretationRow{ID: id, OrganisationID: orgID, ProjectID: projectID, Status: status, PayloadJSON: payloadJSON, Reason: reason, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	row.AccountID, _ = scanUUID(accountID)
	row.RunID, _ = scanUUID(runID)
	if confidence.Valid {
		v := confidence.Float64
		row.Confidence = &v
	}
	return row, nil
}

func scanInterpretationSource(s rowScanner) (*driven.InterpretationSourceRow, error) {
	var idStr, interpretationStr string
	var messageID, manualItemID, connectorMessageID sql.NullString
	if err := s.Scan(&idStr, &interpretationStr, &messageID, &manualItemID, &connectorMessageID); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	interpretationID, _ := uuid.Parse(interpretationStr)
	row := &driven.InterpretationSourceRow{ID: id, InterpretationID: interpretationID}
	row.MessageID, _ = scanUUID(messageID)
	row.ManualItemID, _ = scanUUID(manualItemID)
	row.ConnectorMessageID, _ = scanUUID(connectorMessageID)
	return row, nil
}

func (r *Repository) CreateContradiction(ctx context.Context, row driven.ContradictionRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO contradictions (id, organisation_id, project_id, status, summary, resolution_note, resolved_at, resolved_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), row.Status, row.Summary, nullStr(row.ResolutionNote), nullTime(row.ResolvedAt), nullUUID(row.ResolvedByUserID), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) AddContradictionSide(ctx context.Context, row driven.ContradictionSideRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO contradiction_sides (id, contradiction_id, fact_version_id, decision_id)
		VALUES (?, ?, ?, ?)
	`, row.ID.String(), row.ContradictionID.String(), nullUUID(row.FactVersionID), nullUUID(row.DecisionID))
	return err
}

func (r *Repository) GetContradiction(ctx context.Context, organisationID, id uuid.UUID) (*driven.ContradictionRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, status, summary, resolution_note, resolved_at, resolved_by_user_id, created_at, updated_at
		FROM contradictions WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	item, err := scanContradiction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListContradictionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]driven.ContradictionRow, error) {
	q := `
		SELECT id, organisation_id, project_id, status, summary, resolution_note, resolved_at, resolved_by_user_id, created_at, updated_at
		FROM contradictions WHERE organisation_id = ? AND project_id = ?`
	args := []any{organisationID.String(), projectID.String()}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY created_at DESC`
	rows, err := r.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContradictionRow, 0)
	for rows.Next() {
		item, err := scanContradiction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateContradiction(ctx context.Context, row driven.ContradictionRow) error {
	res, err := r.execContext(ctx, `
		UPDATE contradictions SET status = ?, summary = ?, resolution_note = ?, resolved_at = ?, resolved_by_user_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, row.Status, row.Summary, nullStr(row.ResolutionNote), nullTime(row.ResolvedAt), nullUUID(row.ResolvedByUserID), row.UpdatedAt.UTC(), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListContradictionSides(ctx context.Context, contradictionID uuid.UUID) ([]driven.ContradictionSideRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, contradiction_id, fact_version_id, decision_id
		FROM contradiction_sides WHERE contradiction_id = ?
	`, contradictionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ContradictionSideRow, 0)
	for rows.Next() {
		item, err := scanContradictionSide(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanContradiction(s rowScanner) (*driven.ContradictionRow, error) {
	var idStr, orgStr, projectStr, status, summary string
	var note, resolvedBy sql.NullString
	var resolvedAt sql.NullTime
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &projectStr, &status, &summary, &note, &resolvedAt, &resolvedBy, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	projectID, _ := uuid.Parse(projectStr)
	row := &driven.ContradictionRow{ID: id, OrganisationID: orgID, ProjectID: projectID, Status: status, Summary: summary, ResolutionNote: nullStringPtr(note), ResolvedAt: nullTimePtr(resolvedAt), CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	row.ResolvedByUserID, _ = scanUUID(resolvedBy)
	return row, nil
}

func scanContradictionSide(s rowScanner) (*driven.ContradictionSideRow, error) {
	var idStr, contradictionStr string
	var factVersionID, decisionID sql.NullString
	if err := s.Scan(&idStr, &contradictionStr, &factVersionID, &decisionID); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	contradictionID, _ := uuid.Parse(contradictionStr)
	row := &driven.ContradictionSideRow{ID: id, ContradictionID: contradictionID}
	row.FactVersionID, _ = scanUUID(factVersionID)
	row.DecisionID, _ = scanUUID(decisionID)
	return row, nil
}

func (r *Repository) CreateDecision(ctx context.Context, row driven.DecisionRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO decisions (
			id, organisation_id, project_id, issue_id, statement, status, decided_at, assignee_user_id, assignee_contact_id,
			source, confidence, supersedes_decision_id, created_by_user_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.OrganisationID.String(), row.ProjectID.String(), nullUUID(row.IssueID), row.Statement, row.Status, nullTime(row.DecidedAt), nullUUID(row.AssigneeUserID), nullUUID(row.AssigneeContactID), row.Source, nullFloat(row.Confidence), nullUUID(row.SupersedesDecisionID), nullUUID(row.CreatedByUserID), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) GetDecision(ctx context.Context, organisationID, id uuid.UUID) (*driven.DecisionRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, organisation_id, project_id, issue_id, statement, status, decided_at, assignee_user_id, assignee_contact_id,
			source, confidence, supersedes_decision_id, created_by_user_id, created_at, updated_at
		FROM decisions WHERE id = ? AND organisation_id = ?
	`, id.String(), organisationID.String())
	item, err := scanDecision(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListDecisionsByProject(ctx context.Context, organisationID, projectID uuid.UUID, status string) ([]driven.DecisionRow, error) {
	q := `
		SELECT id, organisation_id, project_id, issue_id, statement, status, decided_at, assignee_user_id, assignee_contact_id,
			source, confidence, supersedes_decision_id, created_by_user_id, created_at, updated_at
		FROM decisions WHERE organisation_id = ? AND project_id = ?`
	args := []any{organisationID.String(), projectID.String()}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY COALESCE(decided_at, updated_at) DESC, created_at DESC`
	rows, err := r.queryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.DecisionRow, 0)
	for rows.Next() {
		item, err := scanDecision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) UpdateDecision(ctx context.Context, row driven.DecisionRow) error {
	res, err := r.execContext(ctx, `
		UPDATE decisions SET issue_id = ?, statement = ?, status = ?, decided_at = ?, assignee_user_id = ?, assignee_contact_id = ?,
			source = ?, confidence = ?, supersedes_decision_id = ?, created_by_user_id = ?, updated_at = ?
		WHERE id = ? AND organisation_id = ?
	`, nullUUID(row.IssueID), row.Statement, row.Status, nullTime(row.DecidedAt), nullUUID(row.AssigneeUserID), nullUUID(row.AssigneeContactID), row.Source, nullFloat(row.Confidence), nullUUID(row.SupersedesDecisionID), nullUUID(row.CreatedByUserID), row.UpdatedAt.UTC(), row.ID.String(), row.OrganisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) AddDecisionEvidence(ctx context.Context, row driven.DecisionEvidenceRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO decision_evidence (id, decision_id, message_id, manual_item_id, added_at)
		VALUES (?, ?, ?, ?, ?)
	`, row.ID.String(), row.DecisionID.String(), nullUUID(row.MessageID), nullUUID(row.ManualItemID), row.AddedAt.UTC())
	return err
}

func (r *Repository) RemoveDecisionEvidence(ctx context.Context, organisationID, decisionID, evidenceID uuid.UUID) error {
	res, err := r.execContext(ctx, `
		DELETE FROM decision_evidence
		WHERE id = ? AND decision_id = ? AND decision_id IN (SELECT id FROM decisions WHERE organisation_id = ?)
	`, evidenceID.String(), decisionID.String(), organisationID.String())
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *Repository) ListDecisionEvidence(ctx context.Context, decisionID uuid.UUID) ([]driven.DecisionEvidenceRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, decision_id, message_id, manual_item_id, added_at
		FROM decision_evidence WHERE decision_id = ?
		ORDER BY added_at ASC
	`, decisionID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.DecisionEvidenceRow, 0)
	for rows.Next() {
		item, err := scanDecisionEvidence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanDecision(s rowScanner) (*driven.DecisionRow, error) {
	var idStr, orgStr, projectStr, statement, status, source string
	var issueID, assigneeUserID, assigneeContactID, supersedesDecisionID, createdByUserID sql.NullString
	var decidedAt sql.NullTime
	var confidence sql.NullFloat64
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &orgStr, &projectStr, &issueID, &statement, &status, &decidedAt, &assigneeUserID, &assigneeContactID, &source, &confidence, &supersedesDecisionID, &createdByUserID, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	orgID, _ := uuid.Parse(orgStr)
	projectID, _ := uuid.Parse(projectStr)
	row := &driven.DecisionRow{ID: id, OrganisationID: orgID, ProjectID: projectID, Statement: statement, Status: status, DecidedAt: nullTimePtr(decidedAt), Source: source, CreatedAt: createdAt.UTC(), UpdatedAt: updatedAt.UTC()}
	row.IssueID, _ = scanUUID(issueID)
	row.AssigneeUserID, _ = scanUUID(assigneeUserID)
	row.AssigneeContactID, _ = scanUUID(assigneeContactID)
	row.SupersedesDecisionID, _ = scanUUID(supersedesDecisionID)
	row.CreatedByUserID, _ = scanUUID(createdByUserID)
	if confidence.Valid {
		v := confidence.Float64
		row.Confidence = &v
	}
	return row, nil
}

func scanDecisionEvidence(s rowScanner) (*driven.DecisionEvidenceRow, error) {
	var idStr, decisionStr string
	var messageID, manualItemID sql.NullString
	var addedAt time.Time
	if err := s.Scan(&idStr, &decisionStr, &messageID, &manualItemID, &addedAt); err != nil {
		return nil, err
	}
	id, _ := uuid.Parse(idStr)
	decisionID, _ := uuid.Parse(decisionStr)
	row := &driven.DecisionEvidenceRow{ID: id, DecisionID: decisionID, AddedAt: addedAt.UTC()}
	row.MessageID, _ = scanUUID(messageID)
	row.ManualItemID, _ = scanUUID(manualItemID)
	return row, nil
}

func (r *Repository) InsertConnectorAccount(ctx context.Context, row driven.ConnectorAccountRow, tokenCiphertext []byte) error {
	scopes, _ := json.Marshal(row.Scopes)
	if row.Scopes == nil {
		scopes = []byte("[]")
	}
	_, err := r.execContext(ctx, `
		INSERT INTO connector_accounts (
			id, user_id, provider, label, external_tenant_id, connection_status,
			last_error, scopes_json, token_ciphertext, last_synced_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, row.ID.String(), row.UserID.String(), row.Provider, row.Label, nullStr(row.ExternalTenantID), row.ConnectionStatus, nullStr(row.LastError), string(scopes), tokenCiphertext, nullTime(row.LastSyncedAt), row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) GetConnectorAccount(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorAccountRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT id, user_id, provider, label, external_tenant_id, connection_status, last_error, scopes_json, last_synced_at, created_at, updated_at
		FROM connector_accounts WHERE id = ? AND user_id = ?
	`, id.String(), userID.String())
	item, err := scanConnectorAccount(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListConnectorAccounts(ctx context.Context, userID uuid.UUID) ([]driven.ConnectorAccountRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT id, user_id, provider, label, external_tenant_id, connection_status, last_error, scopes_json, last_synced_at, created_at, updated_at
		FROM connector_accounts WHERE user_id = ? ORDER BY created_at ASC
	`, userID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorAccountRow, 0)
	for rows.Next() {
		item, err := scanConnectorAccount(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteConnectorAccount(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.execContext(ctx, `DELETE FROM connector_accounts WHERE id = ? AND user_id = ?`, id.String(), userID.String())
	return err
}

func (r *Repository) GetConnectorTokenCipher(ctx context.Context, userID, id uuid.UUID) ([]byte, error) {
	var cipher []byte
	err := r.queryRowContext(ctx, `SELECT token_ciphertext FROM connector_accounts WHERE id = ? AND user_id = ?`, id.String(), userID.String()).Scan(&cipher)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), cipher...), nil
}

func (r *Repository) UpdateConnectorToken(ctx context.Context, userID, id uuid.UUID, tokenCiphertext []byte, scopes []string, status string, lastError *string, lastSyncedAt *time.Time) error {
	scopesJSON, _ := json.Marshal(scopes)
	if scopes == nil {
		scopesJSON = []byte("[]")
	}
	_, err := r.execContext(ctx, `
		UPDATE connector_accounts
		SET token_ciphertext = ?, scopes_json = ?, connection_status = ?, last_error = ?, last_synced_at = COALESCE(?, last_synced_at), updated_at = ?
		WHERE id = ? AND user_id = ?
	`, tokenCiphertext, string(scopesJSON), status, nullStr(lastError), nullTime(lastSyncedAt), time.Now().UTC(), id.String(), userID.String())
	return err
}

func (r *Repository) CreateConnectorBinding(ctx context.Context, row driven.ConnectorBindingRow) error {
	_, err := r.execContext(ctx, `
		INSERT INTO connector_bindings (
			id, connector_account_id, organisation_id, external_channel_id, project_id, label, sync_cursor, created_at, updated_at
		) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE EXISTS (SELECT 1 FROM connector_accounts WHERE id = ?)
	`, row.ID.String(), row.ConnectorAccountID.String(), row.OrganisationID.String(), row.ExternalChannelID, nullUUID(row.ProjectID), row.Label, nullStr(row.SyncCursor), row.CreatedAt.UTC(), row.UpdatedAt.UTC(), row.ConnectorAccountID.String())
	return err
}

func (r *Repository) GetConnectorBinding(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorBindingRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT b.id, b.connector_account_id, b.organisation_id, b.external_channel_id, b.project_id, b.label, b.sync_cursor, b.created_at, b.updated_at
		FROM connector_bindings b
		INNER JOIN connector_accounts a ON a.id = b.connector_account_id AND a.user_id = ?
		WHERE b.id = ?
	`, userID.String(), id.String())
	item, err := scanConnectorBinding(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func (r *Repository) ListConnectorBindings(ctx context.Context, userID, connectorAccountID uuid.UUID) ([]driven.ConnectorBindingRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT b.id, b.connector_account_id, b.organisation_id, b.external_channel_id, b.project_id, b.label, b.sync_cursor, b.created_at, b.updated_at
		FROM connector_bindings b
		INNER JOIN connector_accounts a ON a.id = b.connector_account_id AND a.user_id = ?
		WHERE b.connector_account_id = ?
		ORDER BY b.created_at ASC
	`, userID.String(), connectorAccountID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorBindingRow, 0)
	for rows.Next() {
		item, err := scanConnectorBinding(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) DeleteConnectorBinding(ctx context.Context, userID, id uuid.UUID) error {
	_, err := r.execContext(ctx, `
		DELETE FROM connector_bindings
		WHERE id = ? AND connector_account_id IN (
			SELECT id FROM connector_accounts WHERE user_id = ?
		)
	`, id.String(), userID.String())
	return err
}

func (r *Repository) UpdateConnectorBindingCursor(ctx context.Context, userID, id uuid.UUID, cursor *string, updatedAt time.Time) error {
	_, err := r.execContext(ctx, `
		UPDATE connector_bindings
		SET sync_cursor = ?, updated_at = ?
		WHERE id = ? AND connector_account_id IN (
			SELECT id FROM connector_accounts WHERE user_id = ?
		)
	`, nullStr(cursor), updatedAt.UTC(), id.String(), userID.String())
	return err
}

func (r *Repository) UpsertConnectorMessage(ctx context.Context, row driven.ConnectorMessageRow) error {
	metaJSON := row.MetaJSON
	if metaJSON == "" {
		metaJSON = "{}"
	}
	_, err := r.execContext(ctx, `
		INSERT INTO connector_messages (
			id, connector_account_id, organisation_id, project_id, provider_event_id, external_channel_id,
			title, body_text, author_label, occurred_at, meta_json, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(connector_account_id, provider_event_id) DO UPDATE SET
			organisation_id = excluded.organisation_id,
			project_id = excluded.project_id,
			external_channel_id = excluded.external_channel_id,
			title = excluded.title,
			body_text = excluded.body_text,
			author_label = excluded.author_label,
			occurred_at = excluded.occurred_at,
			meta_json = excluded.meta_json,
			updated_at = excluded.updated_at
	`, row.ID.String(), row.ConnectorAccountID.String(), row.OrganisationID.String(), nullUUID(row.ProjectID), row.ProviderEventID, row.ExternalChannelID, row.Title, row.BodyText, row.AuthorLabel, row.OccurredAt.UTC(), metaJSON, row.CreatedAt.UTC(), row.UpdatedAt.UTC())
	return err
}

func (r *Repository) ListConnectorMessagesForProject(ctx context.Context, userID, organisationID, projectID uuid.UUID) ([]driven.ConnectorMessageRow, error) {
	rows, err := r.queryContext(ctx, `
		SELECT m.id, m.connector_account_id, m.organisation_id, m.project_id, m.provider_event_id, m.external_channel_id,
			m.title, m.body_text, m.author_label, m.occurred_at, m.meta_json, m.created_at, m.updated_at
		FROM connector_messages m
		INNER JOIN connector_accounts a ON a.id = m.connector_account_id AND a.user_id = ?
		WHERE m.organisation_id = ? AND m.project_id = ?
		ORDER BY m.occurred_at DESC
	`, userID.String(), organisationID.String(), projectID.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]driven.ConnectorMessageRow, 0)
	for rows.Next() {
		item, err := scanConnectorMessage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func (r *Repository) GetConnectorMessage(ctx context.Context, userID, id uuid.UUID) (*driven.ConnectorMessageRow, error) {
	row := r.queryRowContext(ctx, `
		SELECT m.id, m.connector_account_id, m.organisation_id, m.project_id, m.provider_event_id, m.external_channel_id,
			m.title, m.body_text, m.author_label, m.occurred_at, m.meta_json, m.created_at, m.updated_at
		FROM connector_messages m
		INNER JOIN connector_accounts a ON a.id = m.connector_account_id AND a.user_id = ?
		WHERE m.id = ?
	`, userID.String(), id.String())
	item, err := scanConnectorMessage(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return item, err
}

func scanConnectorAccount(s rowScanner) (*driven.ConnectorAccountRow, error) {
	var row driven.ConnectorAccountRow
	var idStr, userStr, scopesJSON string
	var tenant, lastError sql.NullString
	var lastSynced sql.NullTime
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &userStr, &row.Provider, &row.Label, &tenant, &row.ConnectionStatus, &lastError, &scopesJSON, &lastSynced, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(idStr); err != nil {
		return nil, err
	}
	if row.UserID, err = uuid.Parse(userStr); err != nil {
		return nil, err
	}
	row.ExternalTenantID = nullStringPtr(tenant)
	row.LastError = nullStringPtr(lastError)
	if err := json.Unmarshal([]byte(scopesJSON), &row.Scopes); err != nil {
		return nil, err
	}
	if row.Scopes == nil {
		row.Scopes = []string{}
	}
	row.LastSyncedAt = nullTimePtr(lastSynced)
	row.CreatedAt = createdAt.UTC()
	row.UpdatedAt = updatedAt.UTC()
	return &row, nil
}

func scanConnectorBinding(s rowScanner) (*driven.ConnectorBindingRow, error) {
	var row driven.ConnectorBindingRow
	var idStr, accountStr, organisationStr string
	var projectID, cursor sql.NullString
	var createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &accountStr, &organisationStr, &row.ExternalChannelID, &projectID, &row.Label, &cursor, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(idStr); err != nil {
		return nil, err
	}
	if row.ConnectorAccountID, err = uuid.Parse(accountStr); err != nil {
		return nil, err
	}
	if row.OrganisationID, err = uuid.Parse(organisationStr); err != nil {
		return nil, err
	}
	row.ProjectID, err = scanUUID(projectID)
	if err != nil {
		return nil, err
	}
	row.SyncCursor = nullStringPtr(cursor)
	row.CreatedAt = createdAt.UTC()
	row.UpdatedAt = updatedAt.UTC()
	return &row, nil
}

func scanConnectorMessage(s rowScanner) (*driven.ConnectorMessageRow, error) {
	var row driven.ConnectorMessageRow
	var idStr, accountStr, organisationStr string
	var projectID sql.NullString
	var occurredAt, createdAt, updatedAt time.Time
	if err := s.Scan(&idStr, &accountStr, &organisationStr, &projectID, &row.ProviderEventID, &row.ExternalChannelID, &row.Title, &row.BodyText, &row.AuthorLabel, &occurredAt, &row.MetaJSON, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var err error
	if row.ID, err = uuid.Parse(idStr); err != nil {
		return nil, err
	}
	if row.ConnectorAccountID, err = uuid.Parse(accountStr); err != nil {
		return nil, err
	}
	if row.OrganisationID, err = uuid.Parse(organisationStr); err != nil {
		return nil, err
	}
	row.ProjectID, err = scanUUID(projectID)
	if err != nil {
		return nil, err
	}
	row.OccurredAt = occurredAt.UTC()
	row.CreatedAt = createdAt.UTC()
	row.UpdatedAt = updatedAt.UTC()
	return &row, nil
}
