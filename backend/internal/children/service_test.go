package children

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	"github.com/family-habit/family-habit/backend/internal/auth"
	"github.com/family-habit/family-habit/backend/internal/database"
)

func TestNormalizeNickname(t *testing.T) {
	if got := NormalizeNickname("  Sam  "); got != "Sam" {
		t.Fatalf("NormalizeNickname() = %q", got)
	}
}

func TestChildLifecycleAuthorizationIntegration(t *testing.T) {
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

	var random [8]byte
	if _, err = rand.Read(random[:]); err != nil {
		t.Fatal(err)
	}
	password := "correct horse battery staple"
	authService := auth.NewService(pool)
	parent, token, err := authService.Register(ctx, "children-"+hex.EncodeToString(random[:])+"@example.test", password, "Children Test", "Asia/Jakarta", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM audit_events WHERE family_id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM idempotency_records WHERE family_id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM sessions WHERE family_id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM children WHERE family_id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM family_memberships WHERE family_id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM families WHERE id=$1`, parent.FamilyID)
		_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, parent.UserID)
	})

	service := NewService(pool)
	child, replay, err := service.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "child-1", []byte("request-1"), "Sam", "fox", "#112233", "")
	if err != nil || replay {
		t.Fatalf("Create() = %+v, %v, %v", child, replay, err)
	}
	if _, _, err = service.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "child-2", []byte("request-2"), "sam", "bear", "#223344", ""); !errors.Is(err, ErrNicknameExists) {
		t.Fatalf("case-insensitive duplicate error = %v", err)
	}

	if _, err = authService.LockParent(ctx, parent); err != nil {
		t.Fatal(err)
	}
	nickname := "Samuel"
	if _, err = service.Update(ctx, parent.ID, parent.UserID, parent.FamilyID, child.ID, Update{Nickname: &nickname}); !errors.Is(err, ErrParentAuthority) {
		t.Fatalf("update after concurrent lock error = %v", err)
	}
	parent, err = authService.UnlockParent(ctx, parent, password, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Archive(ctx, parent.ID, parent.UserID, parent.FamilyID, child.ID); err != nil {
		t.Fatal(err)
	}
	if err = service.Archive(ctx, parent.ID, parent.UserID, parent.FamilyID, child.ID); err != nil {
		t.Fatalf("repeated Archive() = %v", err)
	}
	active, err := service.List(ctx, parent.FamilyID, false)
	if err != nil || len(active) != 0 {
		t.Fatalf("active children = %+v, %v", active, err)
	}
	all, err := service.List(ctx, parent.FamilyID, true)
	if err != nil || len(all) != 1 || all[0].Active {
		t.Fatalf("archived children = %+v, %v", all, err)
	}
	if _, _, err = service.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "child-3", []byte("request-3"), "sam", "bear", "#223344", ""); err != nil {
		t.Fatalf("nickname reuse after archive = %v", err)
	}
	if _, err = service.Enter(ctx, parent.ID, parent.UserID, parent.FamilyID, child.ID, ""); !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("archived entry error = %v", err)
	}

	pinHash, err := auth.HashPassword("1234")
	if err != nil {
		t.Fatal(err)
	}
	protected, _, err := service.Create(ctx, parent.ID, parent.UserID, parent.FamilyID, "child-4", []byte("request-4"), "Jo", "owl", "#334455", pinHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = service.Enter(ctx, parent.ID, parent.UserID, parent.FamilyID, protected.ID, "9999"); !errors.Is(err, ErrProfileUnavailable) {
		t.Fatalf("wrong PIN error = %v", err)
	}
	if _, err = service.Enter(ctx, parent.ID, parent.UserID, parent.FamilyID, protected.ID, "1234"); err != nil {
		t.Fatalf("correct PIN entry = %v", err)
	}
	entered, err := authService.Authenticate(ctx, token)
	if err != nil || entered.Mode != "child" || entered.ActiveChildID != protected.ID {
		t.Fatalf("child session = %+v, %v", entered, err)
	}

	otherParent, _, err := authService.Login(ctx, "children-"+hex.EncodeToString(random[:])+"@example.test", password)
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Archive(ctx, otherParent.ID, otherParent.UserID, otherParent.FamilyID, protected.ID); err != nil {
		t.Fatal(err)
	}
	downgraded, err := authService.Authenticate(ctx, token)
	if err != nil || downgraded.Mode != "shared" || downgraded.ActiveChildID != "" {
		t.Fatalf("archived child session = %+v, %v", downgraded, err)
	}
	if err = service.Leave(ctx, parent.ID, parent.UserID, parent.FamilyID); err != nil {
		t.Fatalf("idempotent leave = %v", err)
	}
}
