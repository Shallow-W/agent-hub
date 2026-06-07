package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// TokenIssuer generates scoped JWT tokens for agent management.
// Shared by AgentService and OrchestratorService to avoid duplicate token logic.
type TokenIssuer struct {
	secret string
}

// NewTokenIssuer creates a TokenIssuer with the given JWT secret.
func NewTokenIssuer(secret string) *TokenIssuer {
	return &TokenIssuer{secret: secret}
}

// IssueAgentToken generates an agent_management scoped JWT with 5-minute expiry.
func (ti *TokenIssuer) IssueAgentToken(userID string) (token string, expiresAt time.Time, err error) {
	now := time.Now()
	expiresAt = now.Add(5 * time.Minute)
	claims := jwt.MapClaims{
		"user_id": userID,
		"scope":   "agent_management",
		"iat":     now.Unix(),
		"exp":     expiresAt.Unix(),
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := t.SignedString([]byte(ti.secret))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign agent token: %w", err)
	}
	return tokenStr, expiresAt, nil
}
