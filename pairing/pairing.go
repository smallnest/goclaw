// Package pairing implements OpenClaw-style DM pairing for channel access control
package pairing

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// DMPolicy defines the direct message access policy
type DMPolicy string

const (
	// DMPolicyOpen allows all senders
	DMPolicyOpen DMPolicy = "open"
	// DMPolicyPairing requires pairing approval for unknown senders
	DMPolicyPairing DMPolicy = "pairing"
	// DMPolicyAllowlist only allows senders in the allowlist
	DMPolicyAllowlist DMPolicy = "allowlist"
	// DMPolicyClosed blocks all DMs
	DMPolicyClosed DMPolicy = "closed"
)

// PairingRequest represents a pending pairing request
type PairingRequest struct {
	ID        string                 `json:"id"`         // sender ID (e.g., open_id, user_id)
	Name      string                 `json:"name"`       // sender display name
	Code      string                 `json:"code"`       // 8-char uppercase code
	CreatedAt int64                  `json:"created_at"` // Unix timestamp
	ExpiresAt int64                  `json:"expires_at"` // Unix timestamp
	Metadata  map[string]interface{} `json:"metadata"`   // Additional metadata
}

// IsExpired checks if the pairing request has expired
func (p *PairingRequest) IsExpired() bool {
	return time.Now().Unix() > p.ExpiresAt
}

// AllowStore manages the allowlist (approved senders)
type AllowStore struct {
	filePath string
	mu       sync.RWMutex
	allowMap map[string]string // sender ID -> name
}

// PairingStore manages pairing requests and allowlist
type PairingStore struct {
	channel      string // e.g., "feishu", "telegram"
	accountID    string // empty for default account
	dataDir      string // e.g., ~/.goclaw/credentials/
	requestsFile string // path to pending requests JSON
	allowFile    string // path to allowlist JSON
	mu           sync.RWMutex
	requests     []*PairingRequest
	allowStore   *AllowStore
	codeLength   int           // default 8
	codeExpiry   time.Duration // default 1 hour
	maxPending   int           // max pending requests, default 3
}

// Config options for PairingStore
type Config struct {
	Channel    string
	AccountID  string // empty for default account
	DataDir    string
	CodeLength int           // default 8
	CodeExpiry time.Duration // default 1 hour
	MaxPending int           // max pending requests, default 3
}

// NewPairingStore creates a new pairing store
func NewPairingStore(cfg Config) (*PairingStore, error) {
	if cfg.DataDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		cfg.DataDir = filepath.Join(homeDir, ".goclaw", "credentials")
	}

	// Ensure data directory exists
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create credentials directory: %w", err)
	}

	// Set defaults
	if cfg.CodeLength <= 0 {
		cfg.CodeLength = 8
	}
	if cfg.CodeExpiry <= 0 {
		cfg.CodeExpiry = time.Hour
	}
	if cfg.MaxPending <= 0 {
		cfg.MaxPending = 3
	}

	// Build file paths
	var requestsFile, allowFile string
	if cfg.AccountID == "" {
		requestsFile = filepath.Join(cfg.DataDir, fmt.Sprintf("%s-pairing.json", cfg.Channel))
		allowFile = filepath.Join(cfg.DataDir, fmt.Sprintf("%s-allowFrom.json", cfg.Channel))
	} else {
		requestsFile = filepath.Join(cfg.DataDir, fmt.Sprintf("%s-%s-pairing.json", cfg.Channel, cfg.AccountID))
		allowFile = filepath.Join(cfg.DataDir, fmt.Sprintf("%s-%s-allowFrom.json", cfg.Channel, cfg.AccountID))
	}

	ps := &PairingStore{
		channel:      cfg.Channel,
		accountID:    cfg.AccountID,
		dataDir:      cfg.DataDir,
		requestsFile: requestsFile,
		allowFile:    allowFile,
		requests:     make([]*PairingRequest, 0),
		codeLength:   cfg.CodeLength,
		codeExpiry:   cfg.CodeExpiry,
		maxPending:   cfg.MaxPending,
		allowStore: &AllowStore{
			filePath: allowFile,
			allowMap: make(map[string]string),
		},
	}

	// Load existing data
	if err := ps.load(); err != nil {
		return nil, fmt.Errorf("failed to load pairing data: %w", err)
	}

	return ps, nil
}

// load loads pairing requests and allowlist from disk
func (ps *PairingStore) load() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Load pending requests
	if data, err := os.ReadFile(ps.requestsFile); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &ps.requests); err != nil {
				return fmt.Errorf("failed to parse requests file: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read requests file: %w", err)
	}

	// Clean expired requests
	ps.cleanExpired()

	// Load allowlist
	if data, err := os.ReadFile(ps.allowFile); err == nil {
		if len(data) > 0 {
			if err := json.Unmarshal(data, &ps.allowStore.allowMap); err != nil {
				return fmt.Errorf("failed to parse allowlist file: %w", err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read allowlist file: %w", err)
	}

	return nil
}

// save saves pairing requests to disk
// Note: caller must hold the write lock (mu.Lock)
func (ps *PairingStore) save() error {
	data, err := json.MarshalIndent(ps.requests, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal requests: %w", err)
	}

	if err := os.WriteFile(ps.requestsFile, data, 0600); err != nil {
		return fmt.Errorf("failed to write requests file: %w", err)
	}

	return nil
}

// generateCode generates an 8-character uppercase code without ambiguous chars (0O1I)
func (ps *PairingStore) generateCode() (string, error) {
	// Use uppercase letters excluding O and I, and digits excluding 0 and 1
	chars := "23456789ABCDEFGHJKLMNPQRSTUVWXYZ" // 32 chars
	code := make([]byte, ps.codeLength)

	for i := 0; i < ps.codeLength; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", fmt.Errorf("failed to generate random code: %w", err)
		}
		code[i] = chars[n.Int64()]
	}

	return string(code), nil
}

// cleanExpired removes expired pairing requests
func (ps *PairingStore) cleanExpired() {
	now := time.Now().Unix()
	valid := make([]*PairingRequest, 0, len(ps.requests))

	for _, req := range ps.requests {
		if now <= req.ExpiresAt {
			valid = append(valid, req)
		}
	}

	ps.requests = valid
}

// UpsertRequest creates or updates a pairing request
// Returns (code, created, error)
// created is true if a new request was created, false if existing request was found
func (ps *PairingStore) UpsertRequest(senderID, senderName string) (string, bool, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()

	// Check for existing non-expired request
	for _, req := range ps.requests {
		if req.ID == senderID && !req.IsExpired() {
			return req.Code, false, nil
		}
	}

	// Clean expired first
	ps.cleanExpired()

	// Check max pending limit
	if len(ps.requests) >= ps.maxPending {
		return "", false, fmt.Errorf("max pending requests (%d) reached", ps.maxPending)
	}

	// Generate code
	code, err := ps.generateCode()
	if err != nil {
		return "", false, err
	}

	// Create request
	req := &PairingRequest{
		ID:        senderID,
		Name:      senderName,
		Code:      code,
		CreatedAt: now.Unix(),
		ExpiresAt: now.Add(ps.codeExpiry).Unix(),
		Metadata:  make(map[string]interface{}),
	}

	ps.requests = append(ps.requests, req)

	// Save to disk
	if err := ps.save(); err != nil {
		ps.requests = ps.requests[:len(ps.requests)-1] // rollback
		return "", false, err
	}

	return code, true, nil
}

// ListPending returns all non-expired pending requests
func (ps *PairingStore) ListPending() []*PairingRequest {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	ps.cleanExpired()

	result := make([]*PairingRequest, 0, len(ps.requests))
	for _, req := range ps.requests {
		if !req.IsExpired() {
			result = append(result, req)
		}
	}

	return result
}

// Approve approves a pairing request by code
// Returns (senderID, name, error)
func (ps *PairingStore) Approve(code string) (string, string, error) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Find matching request
	var approvedReq *PairingRequest
	var approvedIndex = -1

	for i, req := range ps.requests {
		if req.Code == code && !req.IsExpired() {
			approvedReq = req
			approvedIndex = i
			break
		}
	}

	if approvedReq == nil {
		return "", "", fmt.Errorf("invalid or expired pairing code")
	}

	// Add to allowlist
	ps.allowStore.allowMap[approvedReq.ID] = approvedReq.Name

	// Remove from pending
	ps.requests = append(ps.requests[:approvedIndex], ps.requests[approvedIndex+1:]...)

	// Save both files
	if err := ps.save(); err != nil {
		// Rollback allowlist change
		delete(ps.allowStore.allowMap, approvedReq.ID)
		ps.requests = append(ps.requests, approvedReq)
		return "", "", fmt.Errorf("failed to save requests: %w", err)
	}

	if err := ps.allowStore.Save(); err != nil {
		// Rollback both changes
		delete(ps.allowStore.allowMap, approvedReq.ID)
		ps.requests = append(ps.requests, approvedReq)
		return "", "", fmt.Errorf("failed to save allowlist: %w", err)
	}

	return approvedReq.ID, approvedReq.Name, nil
}

// Reject rejects a pairing request by code
func (ps *PairingStore) Reject(code string) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// Find matching request
	var rejectedIndex = -1

	for i, req := range ps.requests {
		if req.Code == code && !req.IsExpired() {
			rejectedIndex = i
			break
		}
	}

	if rejectedIndex < 0 {
		return fmt.Errorf("invalid or expired pairing code")
	}

	// Remove from pending
	ps.requests = append(ps.requests[:rejectedIndex], ps.requests[rejectedIndex+1:]...)

	// Save to disk
	if err := ps.save(); err != nil {
		return err
	}

	return nil
}

// IsAllowed checks if a sender is in the allowlist
func (ps *PairingStore) IsAllowed(senderID string) bool {
	ps.allowStore.mu.RLock()
	defer ps.allowStore.mu.RUnlock()

	_, ok := ps.allowStore.allowMap[senderID]
	return ok
}

// GetAllowlist returns all allowed senders
func (ps *PairingStore) GetAllowlist() map[string]string {
	ps.allowStore.mu.RLock()
	defer ps.allowStore.mu.RUnlock()

	result := make(map[string]string, len(ps.allowStore.allowMap))
	for k, v := range ps.allowStore.allowMap {
		result[k] = v
	}

	return result
}

// RemoveFromAllowlist removes a sender from the allowlist
func (ps *PairingStore) RemoveFromAllowlist(senderID string) error {
	ps.allowStore.mu.Lock()
	defer ps.allowStore.mu.Unlock()

	if _, ok := ps.allowStore.allowMap[senderID]; !ok {
		return fmt.Errorf("sender not in allowlist: %s", senderID)
	}

	delete(ps.allowStore.allowMap, senderID)

	if err := ps.allowStore.Save(); err != nil {
		// Rollback
		ps.allowStore.allowMap[senderID] = ""
		return err
	}

	return nil
}

// AddToAllowlist directly adds a sender to the allowlist (for admin use)
func (ps *PairingStore) AddToAllowlist(senderID, name string) error {
	ps.allowStore.mu.Lock()
	defer ps.allowStore.mu.Unlock()

	ps.allowStore.allowMap[senderID] = name

	if err := ps.allowStore.Save(); err != nil {
		// Rollback
		delete(ps.allowStore.allowMap, senderID)
		return err
	}

	return nil
}

// Save saves the allowlist to disk
// Note: caller must hold the write lock (mu.Lock)
func (as *AllowStore) Save() error {
	data, err := json.MarshalIndent(as.allowMap, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal allowlist: %w", err)
	}

	if err := os.WriteFile(as.filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write allowlist file: %w", err)
	}

	return nil
}

// BuildPairingReply builds the pairing reply message sent to the user
func BuildPairingReply(channel, idLine, code string) string {
	return fmt.Sprintf("👋 Hi! Your pairing code for %s is:\n\n%s\n\n%s\n\n"+
		"To approve, run: goclaw pairing approve %s %s",
		channel, code, idLine, channel, code)
}
