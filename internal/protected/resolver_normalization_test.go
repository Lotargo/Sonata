package protected

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestManifestResolverNormalizesUnicodeAndLineEndings(t *testing.T) {
	resolver, err := NewManifestResolver(testBundle(), DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	const expected = "Café\nstyle"
	hash := sha256.Sum256([]byte(expected))
	manifest := UserManifest{
		Metadata: Metadata{ID: "global-1", Version: 1, Hash: hex.EncodeToString(hash[:])},
		OwnerID:  "user-1",
		Scope:    ManifestScopeGlobal,
		Status:   ManifestStatusActive,
		Content:  "  Cafe\u0301\r\nstyle  ",
	}
	resolved, err := resolver.Resolve(ResolveManifestInput{
		InstructionID: "router",
		OwnerID:       "user-1",
		Global:        &manifest,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved.UserText != expected {
		t.Fatalf("normalized manifest=%q, want %q", resolved.UserText, expected)
	}
	if resolved.Metadata.Hash != hex.EncodeToString(hash[:]) {
		t.Fatalf("normalized hash=%q", resolved.Metadata.Hash)
	}
}

func TestManifestResolverRejectsInvalidUTF8(t *testing.T) {
	resolver, err := NewManifestResolver(testBundle(), DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	manifest := UserManifest{
		Metadata: Metadata{ID: "global-1", Version: 1},
		OwnerID:  "user-1",
		Scope:    ManifestScopeGlobal,
		Status:   ManifestStatusActive,
		Content:  string([]byte{0xff, 0xfe}),
	}
	if _, err := resolver.Resolve(ResolveManifestInput{InstructionID: "router", OwnerID: "user-1", Global: &manifest}); err == nil {
		t.Fatal("invalid UTF-8 user manifest was accepted")
	}
}
