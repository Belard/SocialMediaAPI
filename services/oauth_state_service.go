package services

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// OAuthState stores temporary state for OAuth flows
type OAuthState struct {
	UserID    string
	Platform  string
	CreatedAt time.Time
}

// OAuthStateService manages OAuth state tokens
type OAuthStateService struct {
	mu     sync.RWMutex
	states map[string]*OAuthState
}

func NewOAuthStateService() *OAuthStateService {
	service := &OAuthStateService{
		states: make(map[string]*OAuthState),
	}
	
	// Cleanup expired states every 10 minutes
	go service.cleanupExpired()
	
	return service
}

// GenerateState creates a new state token
func (s *OAuthStateService) GenerateState(userID, platform string) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Generate random state
	bytes := make([]byte, 32)
	rand.Read(bytes)
	state := hex.EncodeToString(bytes)

	// Store state
	s.states[state] = &OAuthState{
		UserID:    userID,
		Platform:  platform,
		CreatedAt: time.Now(),
	}

	return state
}

// ValidateState validates and consumes a state token
func (s *OAuthStateService) ValidateState(state string) (*OAuthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oauthState, exists := s.states[state]
	if !exists {
		return nil, false
	}

	// Check if expired (10 minutes)
	if time.Since(oauthState.CreatedAt) > 10*time.Minute {
		delete(s.states, state)
		return nil, false
	}

	// Delete state after use (one-time use)
	delete(s.states, state)

	return oauthState, true
}

// cleanupExpired removes expired states
func (s *OAuthStateService) cleanupExpired() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for state, oauthState := range s.states {
			if now.Sub(oauthState.CreatedAt) > 10*time.Minute {
				delete(s.states, state)
			}
		}
		s.mu.Unlock()
	}
}