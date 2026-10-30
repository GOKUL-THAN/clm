package satelliteauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

const (
	saltSize   = 16
	secretSize = 32
	scryptN    = 1 << 15
	scryptR    = 8
	scryptP    = 1
	keySize    = 32
)

var dummySalt = []byte("clm-auth-dummy!!")

// Generate creates a cryptographically random bearer credential and its scrypt
// encoding. The plaintext must be returned to the operator exactly once.
func Generate() (plaintext, encodedHash string, err error) {
	secret := make([]byte, secretSize)
	if _, err = rand.Read(secret); err != nil {
		return "", "", fmt.Errorf("generating satellite credential: %w", err)
	}
	plaintext = base64.RawURLEncoding.EncodeToString(secret)
	encodedHash, err = Hash(plaintext)
	return plaintext, encodedHash, err
}

// Hash derives a salted scrypt hash suitable for server-side storage.
func Hash(plaintext string) (string, error) {
	if strings.TrimSpace(plaintext) == "" {
		return "", errors.New("satellite credential is empty")
	}
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generating credential salt: %w", err)
	}
	derived, err := scrypt.Key([]byte(plaintext), salt, scryptN, scryptR, scryptP, keySize)
	if err != nil {
		return "", fmt.Errorf("hashing satellite credential: %w", err)
	}
	return fmt.Sprintf("$scrypt$ln=15,r=%d,p=%d$%s$%s", scryptR, scryptP,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(derived)), nil
}

// Verify parses an encoded hash, derives the candidate key, and compares it in
// constant time. Malformed/missing hashes still perform the expensive derivation
// so unknown and unprovisioned identities do not become a cheap timing oracle.
func Verify(encodedHash, candidate string) bool {
	n, r, p, salt, expected, ok := parse(encodedHash)
	if !ok {
		n, r, p, salt, expected = scryptN, scryptR, scryptP, dummySalt, make([]byte, keySize)
	}
	derived, err := scrypt.Key([]byte(candidate), salt, n, r, p, len(expected))
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(derived, expected) == 1 && ok
}

func parse(encoded string) (n, r, p int, salt, hash []byte, ok bool) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[1] != "scrypt" {
		return 0, 0, 0, nil, nil, false
	}
	var ln int
	for _, setting := range strings.Split(parts[2], ",") {
		kv := strings.SplitN(setting, "=", 2)
		if len(kv) != 2 {
			return 0, 0, 0, nil, nil, false
		}
		value, err := strconv.Atoi(kv[1])
		if err != nil {
			return 0, 0, 0, nil, nil, false
		}
		switch kv[0] {
		case "ln":
			ln = value
		case "r":
			r = value
		case "p":
			p = value
		default:
			return 0, 0, 0, nil, nil, false
		}
	}
	if ln != 15 || r != scryptR || p != scryptP {
		return 0, 0, 0, nil, nil, false
	}
	n = 1 << ln
	var err error
	salt, err = base64.RawStdEncoding.DecodeString(parts[3])
	if err != nil || len(salt) != saltSize {
		return 0, 0, 0, nil, nil, false
	}
	hash, err = base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(hash) != keySize {
		return 0, 0, 0, nil, nil, false
	}
	return n, r, p, salt, hash, true
}
