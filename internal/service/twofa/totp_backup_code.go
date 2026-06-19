package servicetwofa

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/hydroan/gst/database"
	modeltwofa "github.com/hydroan/gst/internal/model/twofa"
	"github.com/hydroan/gst/types"
	"github.com/hydroan/gst/types/consts"
	"golang.org/x/crypto/bcrypt"
)

const (
	totpBackupCodeCount     = 10
	totpBackupCodeRawLength = 16
	totpBackupCodeGroupSize = 4
	totpBackupCodeAlphabet  = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

var errTOTPBackupCodeInvalid = errors.New("invalid backup code")

// GenerateTOTPBackupCodes creates one-time recovery codes for a new TOTP device.
func GenerateTOTPBackupCodes() ([]string, error) {
	codes := make([]string, totpBackupCodeCount)
	for i := range codes {
		code, err := generateTOTPBackupCode()
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// HashTOTPBackupCodes normalizes and hashes recovery codes before storage.
func HashTOTPBackupCodes(codes []string) ([]string, error) {
	hashes := make([]string, 0, len(codes))
	for _, code := range codes {
		normalizedCode, err := normalizeTOTPBackupCode(code)
		if err != nil {
			return nil, err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(normalizedCode), bcrypt.DefaultCost)
		if err != nil {
			return nil, errors.Wrap(err, "hash TOTP backup code")
		}
		hashes = append(hashes, string(hash))
	}
	return hashes, nil
}

// ConsumeTOTPBackupCode verifies and removes one recovery code for the user.
func ConsumeTOTPBackupCode(ctx *types.ServiceContext, userID, code string) error {
	if ctx == nil || strings.TrimSpace(userID) == "" {
		return types.NewServiceError(http.StatusUnauthorized, "authentication required")
	}
	normalizedCode, err := normalizeTOTPBackupCode(code)
	if err != nil {
		return errTOTPBackupCodeInvalid
	}

	return database.Database[*modeltwofa.TOTPDevice](ctx.DatabaseContext()).Transaction(func(tx types.Database[*modeltwofa.TOTPDevice]) error {
		devices := make([]*modeltwofa.TOTPDevice, 0)
		if err := tx.WithLock(consts.LockUpdate).WithQuery(&modeltwofa.TOTPDevice{
			UserID:   strings.TrimSpace(userID),
			IsActive: true,
		}).List(&devices); err != nil {
			return errors.Wrap(err, "list TOTP devices for backup code")
		}

		for _, device := range devices {
			for i, hash := range device.BackupCodeHashes {
				if bcrypt.CompareHashAndPassword([]byte(hash), []byte(normalizedCode)) != nil {
					continue
				}

				device.BackupCodeHashes = append(device.BackupCodeHashes[:i], device.BackupCodeHashes[i+1:]...)
				now := time.Now()
				device.LastUsedAt = &now
				if err := tx.Update(device); err != nil {
					return errors.Wrap(err, "consume TOTP backup code")
				}
				return nil
			}
		}

		return errTOTPBackupCodeInvalid
	})
}

func generateTOTPBackupCode() (string, error) {
	var b strings.Builder
	b.Grow(totpBackupCodeRawLength)
	for range totpBackupCodeRawLength {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(totpBackupCodeAlphabet))))
		if err != nil {
			return "", fmt.Errorf("generate TOTP backup code: %w", err)
		}
		b.WriteByte(totpBackupCodeAlphabet[idx.Int64()])
	}
	return formatTOTPBackupCode(b.String()), nil
}

func normalizeTOTPBackupCode(code string) (string, error) {
	code = strings.TrimSpace(code)
	code = strings.ReplaceAll(code, "-", "")
	code = strings.ToUpper(code)
	if len(code) != totpBackupCodeRawLength {
		return "", errTOTPBackupCodeInvalid
	}
	for _, r := range code {
		if !isTOTPBackupCodeChar(r) {
			return "", errTOTPBackupCodeInvalid
		}
	}
	return code, nil
}

func formatTOTPBackupCode(code string) string {
	var b strings.Builder
	b.Grow(totpBackupCodeRawLength + (totpBackupCodeRawLength / totpBackupCodeGroupSize) - 1)
	for i, r := range code {
		if i > 0 && i%totpBackupCodeGroupSize == 0 {
			b.WriteByte('-')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func isTOTPBackupCodeChar(r rune) bool {
	return strings.ContainsRune(totpBackupCodeAlphabet, r)
}
