package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	protectedcore "github.com/Lotargo/Sonata/internal/protected"
)

func TestManifestRepositoryVersionsPutAndDelete(t *testing.T) {
	pool := openIntegrationPool(t)
	repository, err := NewManifestRepository(pool, protectedcore.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "manifest-owner-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})
	start := time.Date(2026, time.July, 14, 14, 0, 0, 0, time.UTC)

	first, err := repository.Put(context.Background(), PutUserManifestInput{
		OwnerID: ownerID,
		Scope:   protectedcore.ManifestScopeGlobal,
		Content: "  Пиши прямо.\r\nБез рекламы.  ",
		At:      start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.Version != 1 || first.Content != "Пиши прямо.\nБез рекламы." || first.Status != protectedcore.ManifestStatusActive {
		t.Fatalf("first manifest = %#v", first)
	}
	if first.Hash != hashManifestContent(first.Content) {
		t.Fatalf("first manifest hash = %q", first.Hash)
	}

	second, err := repository.Put(context.Background(), PutUserManifestInput{
		OwnerID: ownerID,
		Scope:   protectedcore.ManifestScopeGlobal,
		Content: "Пиши короче.",
		At:      start.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Version != 2 || second.Content != "Пиши короче." {
		t.Fatalf("second manifest = %#v", second)
	}

	deleted, err := repository.Delete(
		context.Background(),
		ownerID,
		protectedcore.ManifestScopeGlobal,
		"",
		start.Add(2*time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.ID != first.ID || deleted.Version != 3 || deleted.Status != protectedcore.ManifestStatusDeleted || deleted.Content != "" {
		t.Fatalf("deleted manifest = %#v", deleted)
	}
	if deleted.Hash != hashManifestContent("") {
		t.Fatalf("deleted manifest hash = %q", deleted.Hash)
	}

	stored, err := repository.Get(context.Background(), ownerID, protectedcore.ManifestScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	if stored.Version != 3 || stored.Status != protectedcore.ManifestStatusDeleted {
		t.Fatalf("stored manifest = %#v", stored)
	}
	if _, err := repository.Get(context.Background(), "other-"+ownerID, protectedcore.ManifestScopeGlobal, ""); !errors.Is(err, ErrUserManifestNotFound) {
		t.Fatalf("cross-owner get error = %v, want not found", err)
	}

	var versionCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*)
		FROM sonata.manifest_versions
		WHERE owner_id = $1 AND manifest_id = $2
	`, ownerID, first.ID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if versionCount != 3 {
		t.Fatalf("manifest version count = %d, want 3", versionCount)
	}
}

func TestManifestRepositorySerializesConcurrentScopeUpdates(t *testing.T) {
	pool := openIntegrationPool(t)
	repository, err := NewManifestRepository(pool, protectedcore.DefaultMaxUserManifestBytes)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	ownerID := "manifest-concurrent-owner-" + suffix
	scopeID := "chat-" + suffix
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM sonata.users WHERE id = $1`, ownerID)
	})

	const updates = 8
	start := time.Date(2026, time.July, 14, 15, 0, 0, 0, time.UTC)
	var wait sync.WaitGroup
	results := make(chan protectedcore.UserManifest, updates)
	errorsChannel := make(chan error, updates)
	for index := 0; index < updates; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			manifest, err := repository.Put(context.Background(), PutUserManifestInput{
				OwnerID: ownerID,
				Scope:   protectedcore.ManifestScopeChat,
				ScopeID: scopeID,
				Content: fmt.Sprintf("version candidate %d", index),
				At:      start.Add(time.Duration(index) * time.Microsecond),
			})
			if err != nil {
				errorsChannel <- err
				return
			}
			results <- manifest
		}()
	}
	wait.Wait()
	close(results)
	close(errorsChannel)
	for err := range errorsChannel {
		t.Fatal(err)
	}

	versions := make(map[int]struct{}, updates)
	manifestID := ""
	for manifest := range results {
		versions[manifest.Version] = struct{}{}
		if manifestID == "" {
			manifestID = manifest.ID
		} else if manifest.ID != manifestID {
			t.Fatalf("concurrent update allocated manifest ID %q, want %q", manifest.ID, manifestID)
		}
	}
	if len(versions) != updates {
		t.Fatalf("unique versions = %d, want %d: %#v", len(versions), updates, versions)
	}
	for version := 1; version <= updates; version++ {
		if _, exists := versions[version]; !exists {
			t.Fatalf("missing manifest version %d: %#v", version, versions)
		}
	}

	stored, err := repository.Get(context.Background(), ownerID, protectedcore.ManifestScopeChat, scopeID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.ID != manifestID || stored.Version != updates || stored.Status != protectedcore.ManifestStatusActive {
		t.Fatalf("stored concurrent manifest = %#v", stored)
	}
}
