package db

type Argon2Password struct {
	Salt        string
	Hash        string
	Iterations  int32
	Parallelism int16
	Memory      int32
	Version     int32
}

type WithPassword interface {
	SetPassword(password Argon2Password)
}

func (c *CreateUserParams) SetPassword(password Argon2Password) {
	c.PwdSalt = password.Salt
	c.PwdHash = password.Hash
	c.PwdIterations = password.Iterations
	c.PwdParallelism = password.Parallelism
	c.PwdMemory = password.Memory
	c.PwdVersion = password.Version
}

func (u *UpdatePasswordParams) SetPassword(password Argon2Password) {
	u.PwdSalt = password.Salt
	u.PwdHash = password.Hash
	u.PwdIterations = password.Iterations
	u.PwdParallelism = password.Parallelism
	u.PwdMemory = password.Memory
	u.PwdVersion = password.Version
}
