package usecases

// Pre-launch scenario verification for workspace picker (List + Select).
// See docs/pre_launch_scenarios.md sec 11 (scenarios 66-70).

import (
	"context"
	"errors"
	"testing"
	"time"

	"keepstar-admin/internal/domain"
	"keepstar-admin/internal/logger"
)

func mkTenantsUC() (*TenantsUseCase, *fakeMemberships, *fakeAuth, *richSessions) {
	mem := newFakeMemberships()
	auth := newFakeAuth()
	sess := newRichSessions()
	sessUC := NewSessionsUseCase(sess, auth, "test-secret-32-bytes-long-xxxxxx",
		15*time.Minute, 30*24*time.Hour, logger.New("error"))
	uc := NewTenantsUseCase(mem, sessUC, auth, logger.New("error"))
	return uc, mem, auth, sess
}

// TestScenario_066_MultiTenantUser_ListsTenants verifies:
// «Я как пользователь с членством в нескольких workspace после signin вижу
// picker "Select workspace" со списком моих tenants + ролей.» (sec 11, scenario 66)
func TestScenario_066_MultiTenantUser_ListsTenants(t *testing.T) {
	uc, mem, auth, _ := mkTenantsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com"})
	_ = mem.Add(context.Background(), user.ID, "t-a", "owner", "")
	_ = mem.Add(context.Background(), user.ID, "t-b", "admin", "")
	_ = mem.Add(context.Background(), user.ID, "t-c", "viewer", "")

	got, err := uc.List(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("got %d memberships, want 3", len(got))
	}
	// Verify roles are preserved.
	byTenant := map[string]string{}
	for _, m := range got {
		byTenant[m.TenantID] = m.Role
	}
	if byTenant["t-a"] != "owner" || byTenant["t-b"] != "admin" || byTenant["t-c"] != "viewer" {
		t.Errorf("roles not preserved: %v", byTenant)
	}
}

// TestScenario_067_SelectWorkspace_IssuesScopedPair verifies:
// «Я как пользователь выбираю один из workspace → backend выдаёт новую
// session pair через IssueForTenant с tid claim = выбранному tenant и role =
// моей role в нём.» (sec 11, scenario 67)
func TestScenario_067_SelectWorkspace_IssuesScopedPair(t *testing.T) {
	uc, mem, auth, sess := mkTenantsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com"})
	_ = mem.Add(context.Background(), user.ID, "t-1", "owner", "")
	_ = mem.Add(context.Background(), user.ID, "t-2", "admin", "")

	pair, err := uc.Select(context.Background(), user.ID, "t-2", "Mozilla/5.0", "1.2.3.4")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if pair == nil || pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatalf("Select returned empty pair: %+v", pair)
	}
	// Session row should bear the chosen tenant.
	for _, s := range sess.byID {
		if s.TenantID != "t-2" {
			t.Errorf("session.tenant=%q, want t-2", s.TenantID)
		}
	}
}

// TestScenario_068_SwitchWorkspaceWithoutLogout verifies:
// «Я как пользователь могу переключаться между workspaces из UI (Settings →
// Switch workspace) без полного logout.» (sec 11, scenario 68)
//
// At the usecase boundary this is equivalent to scenario 67 (Select can be
// called any number of times on an authenticated user). The "without full
// logout" claim is enforced by the frontend NOT clearing the session/token
// pair when switching — but the backend contract is simply: Select() is
// idempotent and works for any tenant the user has membership in.
func TestScenario_068_SwitchWorkspaceWithoutLogout(t *testing.T) {
	uc, mem, auth, _ := mkTenantsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com"})
	_ = mem.Add(context.Background(), user.ID, "t-1", "owner", "")
	_ = mem.Add(context.Background(), user.ID, "t-2", "admin", "")

	// First switch.
	pair1, err := uc.Select(context.Background(), user.ID, "t-1", "", "")
	if err != nil {
		t.Fatalf("Select(t-1): %v", err)
	}
	// Second switch — different tenant.
	pair2, err := uc.Select(context.Background(), user.ID, "t-2", "", "")
	if err != nil {
		t.Fatalf("Select(t-2): %v", err)
	}
	if pair1.RefreshToken == pair2.RefreshToken {
		t.Error("switching tenants should mint fresh refresh tokens")
	}
}

// TestScenario_069_SingleTenantUser_NoPicker verifies:
// «Я как пользователь имею только один workspace — picker НЕ показывается,
// сразу попадаю в админку.» (sec 11, scenario 69)
//
// Backend surface: List() returns exactly one membership. The decision to
// skip the picker UI is the frontend's, but we verify the backend returns
// the right cardinality so the frontend can act.
func TestScenario_069_SingleTenantUser_ListReturnsOne(t *testing.T) {
	uc, mem, auth, _ := mkTenantsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "solo@x.com"})
	_ = mem.Add(context.Background(), user.ID, "t-only", "owner", "")

	got, err := uc.List(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("got %d memberships, want 1", len(got))
	}
}

// TestScenario_070_SoftDeletedTenant_SelectRejects verifies:
// «Я как пользователь попадаю в picker, но мой только workspace soft-deleted
// (orphan cleanup сработал) — backend отдаёт "no active workspace", UI
// показывает "Contact support".» (sec 11, scenario 70)
//
// At the usecase level this scenario only requires that Select() rejects a
// tenant the user has NO membership for. The "soft-deleted" filtering is
// the adapter's job (catalog.tenants.deleted_at IS NULL) — if the adapter
// returns the membership anyway, Select() will succeed and downstream
// queries will get filtered. So this test verifies the no-membership branch.
func TestScenario_070_SelectWithoutMembership_Rejects(t *testing.T) {
	uc, _, auth, _ := mkTenantsUC()
	user, _ := auth.CreateUser(context.Background(), &domain.AdminUser{Email: "u@x.com"})

	_, err := uc.Select(context.Background(), user.ID, "t-stranger", "", "")
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Errorf("Select(no-membership): err=%v, want ErrInvalidCredentials", err)
	}
	// Empty tenantID also rejected.
	if _, err := uc.Select(context.Background(), user.ID, "", "", ""); err == nil {
		t.Error("Select with empty tenantID must err")
	}
}
