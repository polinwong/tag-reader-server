package card

import (
	"os"
	"testing"
)

// TestStage5StoreMutations covers the store-level account mutations: password
// change recomputes rec/salt and rotates sessions; role change enforces the
// >=1 admin guard and rotates the target's sessions.
func TestStage5StoreMutations(t *testing.T) {
	dir, _ := os.MkdirTemp("", "stage5-store")
	defer os.RemoveAll(dir)
	b, err := CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer b.Close()

	// Seed admin + operator.
	aID, _ := UserNewID()
	aSalt, _ := UserNewID()
	aRec := GenUserRec("admin1", "adminpw", []byte(aSalt))
	if err := b.UserAdd(aID, "admin1", aRec, aSalt, RoleAdmin); err != nil {
		t.Fatalf("add admin: %v", err)
	}
	oID, _ := UserNewID()
	oSalt, _ := UserNewID()
	oRec := GenUserRec("op1", "oppass", []byte(oSalt))
	if err := b.UserAdd(oID, "op1", oRec, oSalt, RoleOperator); err != nil {
		t.Fatalf("add op: %v", err)
	}

	// Login both to create sessions.
	aTok, _ := b.UserLoginNew(aID, "admin1", RoleAdmin)
	oTok, _ := b.UserLoginNew(oID, "op1", RoleOperator)

	// Password change: old password must verify, then rotate sessions.
	if err := b.UserChangePW(aID, "newadminpw"); err != nil {
		t.Fatalf("UserChangePW: %v", err)
	}
	if _, err := b.SessionGet(aTok); err == nil {
		t.Fatalf("old admin session should be invalidated after password change")
	}
	// New password verifies via UserVerify.
	if !b.UserVerify("admin1", "newadminpw") {
		t.Fatalf("new password should verify")
	}
	if b.UserVerify("admin1", "adminpw") {
		t.Fatalf("old password should no longer verify")
	}

	// Role change: promote operator -> admin.
	if err := b.UserChangeRole(oID, RoleAdmin); err != nil {
		t.Fatalf("promote op: %v", err)
	}
	info, _ := b.UserGetByID(oID)
	if info.Role != RoleAdmin {
		t.Fatalf("op role should be admin, got %s", info.Role)
	}
	// Old operator session rotated.
	if _, err := b.SessionGet(oTok); err == nil {
		t.Fatalf("old operator session should be invalidated after role change")
	}

	// Now two admins -> demoting one is allowed (guard only blocks <=1 admin).
	if err := b.UserChangeRole(oID, RoleOperator); err != nil {
		t.Fatalf("demote second admin: %v", err)
	}
	// Demoting the remaining admin now fails again (it is the last admin).
	if err := b.UserChangeRole(aID, RoleOperator); err == nil {
		t.Fatalf("demoting last admin should fail")
	}
}
