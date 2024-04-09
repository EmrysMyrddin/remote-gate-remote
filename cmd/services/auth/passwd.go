package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"runtime"
	"woody-wood-portail/cmd/services/db"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidHash         = errors.New("the encoded hash is not in the correct format")
	ErrIncompatibleVersion = errors.New("incompatible version of argon2")
)

type ArgonParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	Version     int
}

const SALT_LENGTH = 16
const KEY_LENGTH = 32

func CreateHash(password string) (db.CreateUserParams, error) {
	salt, err := generateRandomSalt()
	if err != nil {
		return db.CreateUserParams{}, fmt.Errorf("failed to generate salt: %w", err)
	}

	result := db.CreateUserParams{
		PwdSalt:        base64.RawStdEncoding.EncodeToString(salt),
		PwdVersion:     argon2.Version,
		PwdIterations:  3,
		PwdMemory:      64 * 1024,
		PwdParallelism: int16(runtime.NumCPU()),
	}

	result.PwdHash = base64.RawStdEncoding.EncodeToString(argon2.IDKey(
		[]byte(password),
		salt,
		uint32(result.PwdIterations),
		uint32(result.PwdMemory),
		uint8(result.PwdParallelism),
		KEY_LENGTH,
	))

	return result, nil
}

func CompareHashAgainstPassword(user db.User, password string) (bool, error) {
	if user.PwdVersion != argon2.Version {
		return false, ErrIncompatibleVersion
	}

	salt, err := base64.RawStdEncoding.DecodeString(user.PwdSalt)
	if err != nil {
		return false, fmt.Errorf("failed to decode salt: %w", err)
	}

	hash := base64.RawStdEncoding.EncodeToString(argon2.IDKey(
		[]byte(password),
		salt,
		uint32(user.PwdIterations),
		uint32(user.PwdMemory),
		uint8(user.PwdParallelism),
		KEY_LENGTH,
	))

	return hash == user.PwdHash, nil
}

func generateRandomSalt() ([]byte, error) {
	salt := make([]byte, SALT_LENGTH)
	_, err := rand.Read(salt)
	if err != nil {
		return nil, err
	}
	return salt, nil
}
