package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMigrateIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.db")
	for i := 0; i < 2; i++ {
		s, err := Open(path)
		if err != nil {
			t.Fatalf("open #%d: %v", i, err)
		}
		s.Close()
	}
}

func TestUsersAndSessions(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	if _, err := s.AdminUser(ctx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("AdminUser on empty db: %v", err)
	}
	admin, err := s.CreateUser(ctx, "admin", "boss", true, "hash")
	if err != nil {
		t.Fatal(err)
	}
	cust, err := s.CreateUser(ctx, "Alice", "微信: alice", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser(ctx, "alice", "", false, ""); err == nil {
		t.Error("username uniqueness should be case-insensitive")
	}
	if got, err := s.GetUserByUsername(ctx, "ALICE"); err != nil || got.ID != cust.ID {
		t.Errorf("case-insensitive lookup failed: %v %v", got, err)
	}
	if a, err := s.AdminUser(ctx); err != nil || a.ID != admin.ID {
		t.Errorf("AdminUser = %v, %v", a, err)
	}

	users, err := s.ListUsers(ctx)
	if err != nil || len(users) != 2 || users[0].Username != "admin" {
		t.Fatalf("ListUsers = %+v, %v", users, err)
	}

	if err := s.UpdateUser(ctx, cust.ID, "alice2", "renamed"); err != nil {
		t.Fatal(err)
	}
	if u, _ := s.GetUser(ctx, cust.ID); u.Username != "alice2" || u.Note != "renamed" {
		t.Errorf("update not applied: %+v", u)
	}

	sid, err := s.CreateSession(ctx, admin.ID, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if u, err := s.UserBySession(ctx, sid); err != nil || u.ID != admin.ID {
		t.Errorf("UserBySession: %v %v", u, err)
	}
	expired, _ := s.CreateSession(ctx, admin.ID, -time.Minute)
	if _, err := s.UserBySession(ctx, expired); !errors.Is(err, ErrNotFound) {
		t.Errorf("expired session should be rejected, got %v", err)
	}
	if err := s.PurgeExpiredSessions(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteUserSessions(ctx, admin.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UserBySession(ctx, sid); !errors.Is(err, ErrNotFound) {
		t.Error("session should be gone after DeleteUserSessions")
	}

	if err := s.DeleteUser(ctx, cust.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetUser(ctx, cust.ID); !errors.Is(err, ErrNotFound) {
		t.Error("deleted user still found")
	}
}

func TestTokens(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.CreateUser(ctx, "alice", "", false, "")

	tok, err := s.CreateToken(ctx, u.ID, "laptop", "jbt_abcdefgh", "hash1", "read")
	if err != nil {
		t.Fatal(err)
	}
	if tok.LastUsedAt != nil || tok.RevokedAt != nil || tok.Scope != "read" {
		t.Errorf("fresh token wrong: %+v", tok)
	}
	if _, err := s.CreateToken(ctx, u.ID, "x", "p", "hash1", "read"); err == nil {
		t.Error("duplicate hash should fail")
	}
	if _, err := s.CreateToken(ctx, u.ID, "x", "p", "hash2", "admin"); err == nil {
		t.Error("invalid scope should fail the CHECK constraint")
	}

	got, owner, err := s.ActiveTokenByHash(ctx, "hash1")
	if err != nil || got.ID != tok.ID || owner.ID != u.ID {
		t.Fatalf("ActiveTokenByHash: %v %v %v", got, owner, err)
	}
	if err := s.TouchToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.GetToken(ctx, tok.ID); got.LastUsedAt == nil {
		t.Error("TouchToken did not set last_used_at")
	}

	if err := s.RevokeToken(ctx, tok.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ActiveTokenByHash(ctx, "hash1"); !errors.Is(err, ErrNotFound) {
		t.Error("revoked token should not authenticate")
	}
	list, _ := s.ListTokens(ctx, u.ID)
	if len(list) != 1 || list[0].RevokedAt == nil {
		t.Errorf("ListTokens after revoke = %+v", list)
	}
	users, _ := s.ListUsers(ctx)
	if users[0].TokenCount != 0 {
		t.Errorf("revoked tokens must not count as active, got %d", users[0].TokenCount)
	}
}

func TestPackagesReleasesFiles(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)

	pkg, err := s.UpsertPackage(ctx, "JohnnyBT-Demo", "johnnybt-demo", "first", "desc1", "text/markdown", true)
	if err != nil {
		t.Fatal(err)
	}
	// Re-upsert without meta refresh keeps summary; with refresh replaces it.
	if p, _ := s.UpsertPackage(ctx, "johnnybt_demo", "johnnybt-demo", "older", "", "", false); p.Summary != "first" || p.Name != "JohnnyBT-Demo" {
		t.Errorf("updateMeta=false should keep metadata, got %+v", p)
	}
	if p, _ := s.UpsertPackage(ctx, "johnnybt-demo", "johnnybt-demo", "newer", "desc2", "", true); p.Summary != "newer" || p.Name != "johnnybt-demo" {
		t.Errorf("updateMeta=true should replace metadata, got %+v", p)
	}

	for _, v := range []string{"0.9.0", "0.10.0", "1.0.0rc1"} {
		rel, err := s.GetOrCreateRelease(ctx, pkg.ID, v)
		if err != nil {
			t.Fatal(err)
		}
		f := &File{ReleaseID: rel.ID, Filename: "johnnybt_demo-" + v + "-py3-none-any.whl", SHA256: "sha" + v, Size: 10, BlobKey: "k" + v, MetadataSHA256: "m" + v}
		if err := s.CreateFile(ctx, f); err != nil {
			t.Fatal(err)
		}
		if f.ID == 0 {
			t.Error("CreateFile should populate ID")
		}
	}
	// Same release twice is fine, same filename twice is not.
	if _, err := s.GetOrCreateRelease(ctx, pkg.ID, "0.9.0"); err != nil {
		t.Fatal(err)
	}
	rel, _ := s.GetRelease(ctx, pkg.ID, "0.9.0")
	if err := s.CreateFile(ctx, &File{ReleaseID: rel.ID, Filename: "johnnybt_demo-0.9.0-py3-none-any.whl", SHA256: "x", BlobKey: "x"}); err == nil {
		t.Error("duplicate filename should fail")
	}
	if ok, _ := s.FileExists(ctx, "johnnybt_demo-0.9.0-py3-none-any.whl"); !ok {
		t.Error("FileExists false for existing file")
	}

	rels, _ := s.ListReleases(ctx, pkg.ID)
	if got := []string{rels[0].Version, rels[1].Version, rels[2].Version}; got[0] != "1.0.0rc1" || got[1] != "0.10.0" || got[2] != "0.9.0" {
		t.Errorf("ListReleases should sort by PEP 440 descending, got %v", got)
	}
	if v, _ := s.LatestVersion(ctx, pkg.ID); v != "1.0.0rc1" {
		t.Errorf("LatestVersion = %q", v)
	}
	if err := s.SetYanked(ctx, rels[0].ID, true, "broken"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.LatestVersion(ctx, pkg.ID); v != "0.10.0" {
		t.Errorf("LatestVersion should skip yanked, got %q", v)
	}
	files, _ := s.ListPackageFiles(ctx, pkg.ID)
	if len(files) != 3 || files[0].Version != "1.0.0rc1" || !files[0].Yanked || files[0].YankedReason != "broken" {
		t.Errorf("ListPackageFiles = %+v", files)
	}
	f, r, p, err := s.FileByName(ctx, "johnnybt_demo-0.10.0-py3-none-any.whl")
	if err != nil || f.SHA256 != "sha0.10.0" || r.Version != "0.10.0" || p.ID != pkg.ID {
		t.Errorf("FileByName = %v %v %v %v", f, r, p, err)
	}

	list, _ := s.ListPackages(ctx)
	if len(list) != 1 || list[0].LatestVersion != "0.10.0" || list[0].ReleaseCount != 3 || list[0].FileCount != 3 {
		t.Errorf("ListPackages = %+v", list)
	}

	if err := s.DeleteRelease(ctx, rels[2].ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.FileExists(ctx, "johnnybt_demo-0.9.0-py3-none-any.whl"); ok {
		t.Error("files should cascade-delete with their release")
	}
	if err := s.DeletePackage(ctx, pkg.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPackage(ctx, "johnnybt-demo"); !errors.Is(err, ErrNotFound) {
		t.Error("package should be deleted")
	}
	if n, _ := s.ListPackageFiles(ctx, pkg.ID); len(n) != 0 {
		t.Error("files should cascade-delete with their package")
	}
}

func TestEntitlements(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	admin, _ := s.CreateUser(ctx, "admin", "", true, "h")
	alice, _ := s.CreateUser(ctx, "alice", "", false, "")
	pkgA, _ := s.UpsertPackage(ctx, "a", "a", "", "", "", true)
	pkgB, _ := s.UpsertPackage(ctx, "b", "b", "", "", "", true)
	s.GetOrCreateRelease(ctx, pkgA.ID, "1.0")

	if ok, _ := s.HasAccess(ctx, alice.ID, pkgA.ID); ok {
		t.Error("no entitlement should mean no access")
	}
	if err := s.GrantEntitlement(ctx, alice.ID, pkgA.ID, nil, admin.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.HasAccess(ctx, alice.ID, pkgA.ID); !ok {
		t.Error("perpetual entitlement should grant access")
	}
	past := time.Now().Add(-time.Hour)
	if err := s.GrantEntitlement(ctx, alice.ID, pkgB.ID, &past, admin.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.HasAccess(ctx, alice.ID, pkgB.ID); ok {
		t.Error("expired entitlement should deny access")
	}
	future := time.Now().Add(time.Hour)
	if err := s.GrantEntitlement(ctx, alice.ID, pkgB.ID, &future, admin.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.HasAccess(ctx, alice.ID, pkgB.ID); !ok {
		t.Error("re-granting should extend access (upsert)")
	}

	mine, _ := s.ListUserEntitlements(ctx, alice.ID)
	if len(mine) != 2 || mine[0].PackageName != "a" || mine[0].LatestVersion != "1.0" || !mine[0].Active || mine[1].ExpiresAt == nil {
		t.Errorf("ListUserEntitlements = %+v", mine)
	}
	holders, _ := s.ListPackageEntitlements(ctx, pkgA.ID)
	if len(holders) != 1 || holders[0].Username != "alice" {
		t.Errorf("ListPackageEntitlements = %+v", holders)
	}
	visible, _ := s.ListPackagesForUser(ctx, alice.ID)
	if len(visible) != 2 {
		t.Errorf("ListPackagesForUser should list both active entitlements, got %+v", visible)
	}
	s.GrantEntitlement(ctx, alice.ID, pkgB.ID, &past, admin.ID)
	visible, _ = s.ListPackagesForUser(ctx, alice.ID)
	if len(visible) != 1 || visible[0].NormalizedName != "a" {
		t.Errorf("ListPackagesForUser should hide expired, got %+v", visible)
	}

	if err := s.RevokeEntitlement(ctx, alice.ID, pkgA.ID); err != nil {
		t.Fatal(err)
	}
	if ok, _ := s.HasAccess(ctx, alice.ID, pkgA.ID); ok {
		t.Error("revoked entitlement should deny access")
	}
	users, _ := s.ListUsers(ctx)
	for _, u := range users {
		if u.Username == "alice" && u.EntitlementCount != 1 {
			t.Errorf("EntitlementCount = %d", u.EntitlementCount)
		}
	}
}

func TestDownloads(t *testing.T) {
	ctx := context.Background()
	s := openTest(t)
	u, _ := s.CreateUser(ctx, "alice", "", false, "")
	tok, _ := s.CreateToken(ctx, u.ID, "", "p", "h", "read")
	pkg, _ := s.UpsertPackage(ctx, "a", "a", "", "", "", true)
	rel, _ := s.GetOrCreateRelease(ctx, pkg.ID, "1.0")
	f := &File{ReleaseID: rel.ID, Filename: "a-1.0.tar.gz", SHA256: "s", BlobKey: "k"}
	s.CreateFile(ctx, f)

	for i := 0; i < 3; i++ {
		if err := s.LogDownload(ctx, &Download{UserID: &u.ID, TokenID: &tok.ID, FileID: &f.ID, PackageID: &pkg.ID, Filename: f.Filename, IP: "1.2.3.4", UserAgent: "uv"}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := s.ListDownloads(ctx, 2)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListDownloads = %v %v", rows, err)
	}
	if rows[0].Username != "alice" || rows[0].PackageName != "a" || rows[0].IP != "1.2.3.4" {
		t.Errorf("joined fields wrong: %+v", rows[0])
	}
	counts, _ := s.DownloadCounts(ctx)
	if counts[pkg.ID] != 3 {
		t.Errorf("DownloadCounts = %v", counts)
	}
	// Deleting the user keeps the audit row, with the user reference nulled.
	s.DeleteUser(ctx, u.ID)
	rows, _ = s.ListDownloads(ctx, 10)
	if len(rows) != 3 || rows[0].UserID != nil || rows[0].Username != "" {
		t.Errorf("download log should survive user deletion: %+v", rows[0])
	}
}
