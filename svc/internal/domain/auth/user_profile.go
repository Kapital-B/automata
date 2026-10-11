package auth

import "github.com/google/uuid"

type UserProfile struct {
	Email              string
	HomeOrganisationID *uuid.UUID
}
