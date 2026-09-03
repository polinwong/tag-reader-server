package card

import (
	"os"
	"path"
	"testing"
	"time"
)

func tempUserDBPath(t *testing.T) string {
	dir, err := os.MkdirTemp("", "userdb-test")
	if err != nil {
		t.Fatalf("cannot create temp dir: %v", err)
	}
	return dir
}

func TestUserDBStage1(t *testing.T) {
	dir := tempUserDBPath(t)
	defer os.RemoveAll(dir)

	db, err := CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB failed: %v", err)
	}
	defer db.Close()

	// No users yet.
	exists, err := db.userRecordExists()
	if err != nil {
		t.Fatalf("userRecordExists failed: %v", err)
	}
	if exists {
		t.Error("expected no users initially")
	}

	// Add an admin.
	uid, _ := UserNewID()
	salt := "testsalt"
	rec := genPWHashWithSalt("alice", "secret", []byte(salt))
	if err := db.UserAdd(uid, "alice", rec, salt, RoleAdmin); err != nil {
		t.Fatalf("UserAdd failed: %v", err)
	}

	// Add an operator.
	uid2, _ := UserNewID()
	salt2 := "testsalt2"
	rec2 := genPWHashWithSalt("bob", "oppass", []byte(salt2))
	if err := db.UserAdd(uid2, "bob", rec2, salt2, RoleOperator); err != nil {
		t.Fatalf("UserAdd operator failed: %v", err)
	}

	// userdb.db file should exist.
	if _, err := os.Stat(path.Join(dir, "userdb.db")); err != nil {
		t.Errorf("userdb.db not created: %v", err)
	}

	// Lookup by username.
	info, err := db.UserGetByUsername("alice")
	if err != nil {
		t.Fatalf("UserGetByUsername alice failed: %v", err)
	}
	if info.Role != RoleAdmin || info.Username != "alice" {
		t.Errorf("unexpected alice record: %+v", info)
	}

	// Verify correct / wrong password.
	if !db.UserVerify("alice", "secret") {
		t.Error("alice should verify with correct password")
	}
	if db.UserVerify("alice", "wrong") {
		t.Error("alice should NOT verify with wrong password")
	}
	if !db.UserVerify("bob", "oppass") {
		t.Error("bob should verify with correct password")
	}

	// Count admins.
	c, err := db.CountAdmins()
	if err != nil {
		t.Fatalf("CountAdmins failed: %v", err)
	}
	if c != 1 {
		t.Errorf("expected 1 admin, got %d", c)
	}

	// Non-existent user lookup.
	if _, err := db.UserGetByUsername("nobody"); err == nil {
		t.Error("expected error for missing user")
	}

	// --- Stage 2: sessions + loginlog ---
	now := time.Now().Unix()
	tokUUID, _ := DbUUID.NewV4()
	tok := tokUUID.String()
	if err := db.SessionCreate(tok, info.ID, info.Username, info.Role, now); err != nil {
		t.Fatalf("SessionCreate failed: %v", err)
	}
	got, err := db.SessionGet(tok)
	if err != nil {
		t.Fatalf("SessionGet failed: %v", err)
	}
	if got.UserID != info.ID || got.Username != "alice" || got.Role != RoleAdmin || got.LoginTime != now {
		t.Errorf("unexpected session: %+v", got)
	}

	// Missing session.
	if _, err := db.SessionGet("missing-token"); err == nil {
		t.Error("expected error for missing session")
	}

	// Login log append + list.
	lidUUID, _ := DbUUID.NewV4()
	lid := lidUUID.String()
	if err := db.LoginLogAppend(lid, info.ID, info.Username, info.Role, now); err != nil {
		t.Fatalf("LoginLogAppend failed: %v", err)
	}
	logs, err := db.LoginLogList()
	if err != nil {
		t.Fatalf("LoginLogList failed: %v", err)
	}
	if len(logs) != 1 {
		t.Errorf("expected 1 login log, got %d", len(logs))
	} else if logs[0].Username != "alice" || logs[0].LoginTime != now {
		t.Errorf("unexpected login log: %+v", logs[0])
	}

	// Session delete (logout/rotation).
	if err := db.SessionDelete(tok); err != nil {
		t.Fatalf("SessionDelete failed: %v", err)
	}
	if _, err := db.SessionGet(tok); err == nil {
		t.Error("session should be gone after delete")
	}

	// Login log is append-only: still present after session delete.
	logs, _ = db.LoginLogList()
	if len(logs) != 1 {
		t.Errorf("login log should persist after session delete, got %d", len(logs))
	}
}
