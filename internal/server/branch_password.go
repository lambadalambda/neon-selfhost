package server

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"

	"neon-selfhost/internal/branch"
)

const generatedBranchPasswordBytes = 18

func generateBranchPassword() (string, error) {
	raw := make([]byte, generatedBranchPasswordBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate branch password: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ensureBranchPassword(store *branch.Store, branchName string) (branch.Branch, error) {
	b, err := store.GetActive(branchName)
	if err != nil {
		return branch.Branch{}, err
	}

	if strings.TrimSpace(b.Password) != "" {
		return b, nil
	}

	password, err := generateBranchPassword()
	if err != nil {
		return branch.Branch{}, fmt.Errorf("%w: %v", ErrPrimaryEndpointUnavailable, err)
	}

	return store.SetPassword(branchName, password)
}
