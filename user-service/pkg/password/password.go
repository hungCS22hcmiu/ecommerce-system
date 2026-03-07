package password

import "golang.org/x/crypto/bcrypt"

const cost = 12

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
