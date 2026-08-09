package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"

	"github.com/family-habit/family-habit/backend/internal/database"
)

func TestRegistrationSessionLifecycleIntegration(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}
	var suffix [8]byte
	if _, err = rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	email := "auth-" + hex.EncodeToString(suffix[:]) + "@example.test"
	svc := NewService(pool)
	session, token, err := svc.Register(ctx, email, "correct horse battery staple", "Integration Home", "Asia/Jakarta", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE family_id=$1`, session.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE family_id=$1`, session.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM family_memberships WHERE family_id=$1`, session.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM families WHERE id=$1`, session.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, session.UserID)
	})
	if token == "" || session.CSRFToken == "" {
		t.Fatal("expected opaque credentials")
	}
	var storedPassword string
	var storedToken []byte
	if err = pool.QueryRow(ctx, `SELECT u.password_hash,s.token_hash FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.id=$1`, session.ID).Scan(&storedPassword, &storedToken); err != nil {
		t.Fatal(err)
	}
	if storedPassword == "correct horse battery staple" || string(storedToken) == token {
		t.Fatal("credentials were stored in plaintext")
	}
	got, err := svc.Authenticate(ctx, token)
	if err != nil || got.UserID != session.UserID || got.Mode != "parent" {
		t.Fatalf("authenticate = %+v, %v", got, err)
	}
	if !svc.CheckCSRF(ctx, session.ID, session.CSRFToken) || svc.CheckCSRF(ctx, session.ID, "wrong") {
		t.Fatal("CSRF verification failed")
	}
	if err = svc.Logout(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Authenticate(ctx, token); err != ErrUnauthenticated {
		t.Fatalf("revoked session error = %v", err)
	}
}
