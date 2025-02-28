// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.25.0

package db

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type Log struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	CreatedAt pgtype.Timestamp
}

type RegistrationCode struct {
	ID        int16
	Code      string
	UpdatedAt pgtype.Timestamp
}

type UsedToken struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Token     string
	CreatedAt pgtype.Timestamp
}

type User struct {
	ID                uuid.UUID
	Email             string
	FullName          string
	Apartment         string
	PwdSalt           string
	PwdHash           string
	PwdIterations     int32
	PwdParallelism    int16
	PwdMemory         int32
	PwdVersion        int32
	Role              string
	EmailVerified     bool
	CreatedAt         pgtype.Timestamp
	UpdatedAt         pgtype.Timestamp
	RegistrationState string
	LastRegistration  pgtype.Timestamp
}
