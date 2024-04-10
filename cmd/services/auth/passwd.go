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

const SALT_LENGTH = 16
const KEY_LENGTH = 32

func CreateHash(password string, params db.WithPassword) error {
	salt, err := generateRandomSalt()
	if err != nil {
		return fmt.Errorf("failed to generate salt: %w", err)
	}

	argon2Password := db.Argon2Password{
		Salt:        base64.RawStdEncoding.EncodeToString(salt),
		Iterations:  3,
		Memory:      64 * 1024,
		Parallelism: int16(runtime.NumCPU()),
		Version:     argon2.Version,
	}

	argon2Password.Hash = base64.RawStdEncoding.EncodeToString(argon2.IDKey(
		[]byte(password),
		salt,
		uint32(argon2Password.Iterations),
		uint32(argon2Password.Memory),
		uint8(argon2Password.Parallelism),
		KEY_LENGTH,
	))

	params.SetPassword(argon2Password)

	return nil
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
