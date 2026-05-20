package password

import (
	"os"
	"strconv"

	"golang.org/x/crypto/bcrypt"
)

var cost = 12

func init() {
	if v := os.Getenv("BCRYPT_COST"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= bcrypt.MinCost && n <= bcrypt.MaxCost {
			cost = n
		}
	}
}

// Hash returns a bcrypt hash of the plaintext password.
func Hash(plaintext string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(plaintext), cost)
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// Compare reports whether plaintext matches the stored bcrypt hash.
func Compare(hash, plaintext string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plaintext)) == nil
}
