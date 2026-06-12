package port

import "time"

// TokenIssuerPort is the port interface for generating scoped JWT tokens
// for agent management. The concrete implementation lives in the service
// package (*TokenIssuer), but service consumers reference this interface
// to avoid coupling to the concrete implementation.
//
// *TokenIssuer satisfies this interface via structured typing.
type TokenIssuerPort interface {
	// IssueAgentToken generates an agent_management scoped JWT with
	// 5-minute expiry for the given userID.
	IssueAgentToken(userID string) (token string, expiresAt time.Time, err error)
}
