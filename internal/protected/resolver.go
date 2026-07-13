package protected

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const DefaultMaxUserManifestBytes = 16 << 10

type ManifestSource string

const (
	ManifestSourceSystemDefault ManifestSource = "system_default"
	ManifestSourceUserGlobal    ManifestSource = "user_global"
	ManifestSourceUserChat      ManifestSource = "user_chat"
)

type ManifestScope string

const (
	ManifestScopeGlobal ManifestScope = "global"
	ManifestScopeChat   ManifestScope = "chat"
)

type ManifestStatus string

const (
	ManifestStatusActive   ManifestStatus = "active"
	ManifestStatusDisabled ManifestStatus = "disabled"
	ManifestStatusDeleted  ManifestStatus = "deleted"
	ManifestStatusRejected ManifestStatus = "rejected"
)

type UserManifest struct {
	Metadata
	OwnerID string
	Scope   ManifestScope
	ScopeID string
	Status  ManifestStatus
	Content string
}

type ResolveManifestInput struct {
	InstructionID string
	OwnerID       string
	ChatID        string
	Chat          *UserManifest
	Global        *UserManifest
}

type ResolvedManifest struct {
	Source   ManifestSource
	Metadata Metadata
	Default  *DefaultManifest
	UserText string
}

type ManifestResolver struct {
	bundle                *Bundle
	defaultsByInstruction map[string]DefaultManifest
	maxUserManifestBytes  int
}

func NewManifestResolver(bundle *Bundle, maxUserManifestBytes int) (*ManifestResolver, error) {
	if bundle == nil {
		return nil, errors.New("protected bundle is required")
	}
	if maxUserManifestBytes <= 0 {
		return nil, errors.New("user manifest byte limit must be positive")
	}
	defaults := make(map[string]DefaultManifest, len(bundle.DefaultManifests))
	for id, manifest := range bundle.DefaultManifests {
		if _, exists := bundle.Instructions[manifest.Target]; !exists {
			return nil, fmt.Errorf("default manifest %s targets unknown instruction %s", id, manifest.Target)
		}
		if previous, duplicate := defaults[manifest.Target]; duplicate {
			return nil, fmt.Errorf("default manifests %s and %s target the same instruction", previous.ID, id)
		}
		defaults[manifest.Target] = manifest
	}
	for instructionID := range bundle.Instructions {
		if _, exists := defaults[instructionID]; !exists {
			return nil, fmt.Errorf("instruction %s has no protected default manifest", instructionID)
		}
	}
	return &ManifestResolver{
		bundle:                bundle,
		defaultsByInstruction: defaults,
		maxUserManifestBytes:  maxUserManifestBytes,
	}, nil
}

func (r *ManifestResolver) Resolve(input ResolveManifestInput) (ResolvedManifest, error) {
	if r == nil || r.bundle == nil {
		return ResolvedManifest{}, errors.New("manifest resolver is not initialized")
	}
	if input.OwnerID == "" {
		return ResolvedManifest{}, errors.New("manifest owner ID is required")
	}
	if _, exists := r.bundle.Instructions[input.InstructionID]; !exists {
		return ResolvedManifest{}, fmt.Errorf("unknown protected instruction %q", input.InstructionID)
	}
	if input.Chat != nil {
		resolved, active, err := r.resolveUser(input.Chat, input.OwnerID, ManifestScopeChat, input.ChatID, ManifestSourceUserChat)
		if err != nil {
			return ResolvedManifest{}, err
		}
		if active {
			return resolved, nil
		}
	}
	if input.Global != nil {
		resolved, active, err := r.resolveUser(input.Global, input.OwnerID, ManifestScopeGlobal, "", ManifestSourceUserGlobal)
		if err != nil {
			return ResolvedManifest{}, err
		}
		if active {
			return resolved, nil
		}
	}
	manifest := r.defaultsByInstruction[input.InstructionID]
	copy := manifest
	return ResolvedManifest{
		Source:   ManifestSourceSystemDefault,
		Metadata: manifest.Metadata,
		Default:  &copy,
	}, nil
}

func (r *ManifestResolver) resolveUser(manifest *UserManifest, ownerID string, scope ManifestScope, scopeID string, source ManifestSource) (ResolvedManifest, bool, error) {
	if manifest.OwnerID != ownerID {
		return ResolvedManifest{}, false, errors.New("user manifest owner mismatch")
	}
	if manifest.Scope != scope {
		return ResolvedManifest{}, false, fmt.Errorf("user manifest %s has scope %q, expected %q", manifest.ID, manifest.Scope, scope)
	}
	if scope == ManifestScopeChat {
		if scopeID == "" {
			return ResolvedManifest{}, false, errors.New("chat ID is required for chat manifest resolution")
		}
		if manifest.ScopeID != scopeID {
			return ResolvedManifest{}, false, errors.New("user manifest chat scope mismatch")
		}
	} else if manifest.ScopeID != "" {
		return ResolvedManifest{}, false, errors.New("global user manifest must not have a scope ID")
	}
	switch manifest.Status {
	case ManifestStatusDisabled, ManifestStatusDeleted, ManifestStatusRejected:
		return ResolvedManifest{}, false, nil
	case ManifestStatusActive:
	default:
		return ResolvedManifest{}, false, fmt.Errorf("user manifest %s has unsupported status %q", manifest.ID, manifest.Status)
	}
	content, err := normalizeUserManifest(manifest.Content, r.maxUserManifestBytes)
	if err != nil {
		return ResolvedManifest{}, false, fmt.Errorf("user manifest %s: %w", manifest.ID, err)
	}
	metadata := manifest.Metadata
	if metadata.ID == "" || metadata.Version < 1 {
		return ResolvedManifest{}, false, errors.New("active user manifest requires ID and positive version")
	}
	hash := sha256.Sum256([]byte(content))
	actualHash := hex.EncodeToString(hash[:])
	if metadata.Hash == "" {
		metadata.Hash = actualHash
	} else if metadata.Hash != actualHash {
		return ResolvedManifest{}, false, errors.New("user manifest content hash mismatch")
	}
	return ResolvedManifest{
		Source:   source,
		Metadata: metadata,
		UserText: content,
	}, true, nil
}

func normalizeUserManifest(value string, maximum int) (string, error) {
	if len(value) > maximum {
		return "", fmt.Errorf("content exceeds %d bytes", maximum)
	}
	if !utf8.ValidString(value) {
		return "", errors.New("content must be valid UTF-8")
	}
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("content must not be empty")
	}
	if len(value) > maximum {
		return "", fmt.Errorf("content exceeds %d bytes", maximum)
	}
	return value, nil
}
