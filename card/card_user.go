// 3D Artefact Exhibition backend
// File: card_user.go, multi-user account database (Stage 1)
// Created by: user-roles implementation
//
// This file introduces a dedicated user database (userdb.db) that stores
// accounts with their own username + password and a role (admin / operator).
// Stage 1 only defines the data layer: opening the DB and basic CRUD on the
// "users" bucket. Session and login-log handling are added in later stages.
//
// Existing data in admin.db (rec/salt) and carddata.db is NOT touched.

package card

import (
	"encoding/json"
	"errors"
	"path"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

const (
	// DB_USER is the bucket holding one row per account.
	// Key = userID (stable UUID string).
	// Value (JSON) = { username, rec, salt, role }.
	DB_USER = "users"

	// DB_USER_SESS is the bucket holding active sessions.
	// Key = token (UUID string).
	// Value (JSON) = { userID, username, role, loginTime }.
	DB_USER_SESS = "sessions"

	// DB_USER_LOG is the bucket holding the complete login history.
	// Key = loginID (UUID string).
	// Value (JSON) = { userID, username, role, loginTime }.
	// Rows are appended on every successful login and never deleted.
	DB_USER_LOG = "loginlog"
)

// Role constants.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
)

// UserInfo is the database record entry for an account.
type UserInfo struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	Rec        string `json:"rec"`
	Salt       string `json:"salt"`
	Role       string `json:"role"`
	MustChange bool   `json:"mustChange"`
}

// UserDatabase manages the multi-user account store.
type UserDatabase struct {
	userDb IDB
}

// CreateUserDB opens (or creates) userdb.db with the users bucket.
func CreateUserDB(dbpath string) (d *UserDatabase, err error) {
	if dbpath == "" {
		return nil, errors.New("path cannot empty")
	}
	d = new(UserDatabase)

	// This DB holds the per-user accounts, active sessions, and login history.
	// It is independent of admin.db and carddata.db; those are left untouched.
	var userDb IDB
	if userDb, err = DbOpen(path.Join(dbpath, "userdb.db"), 0640, &bbolt.Options{}); err != nil {
		return nil, err
	}
	if err = userDb.Update(func(tx ITx) error {
		for _, name := range []string{DB_USER, DB_USER_SESS, DB_USER_LOG} {
			if _, e := tx.CreateBucketIfNotExists([]byte(name)); e != nil {
				return e
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	d.userDb = userDb

	return d, err
}

// CreateUserDBRaw builds a UserDatabase from injected DB handles (for tests).
func CreateUserDBRaw(userDb IDB) (d *UserDatabase, err error) {
	if userDb == nil {
		return nil, ErrCardDBRead
	}
	d = new(UserDatabase)
	d.userDb = userDb
	return
}

// Close closes the underlying database.
func (b *UserDatabase) Close() {
	if b.userDb != nil {
		b.userDb.Close()
	}
}

// UserAdd creates a new account row. userID should be a stable UUID string.
func (b *UserDatabase) UserAdd(userID, username, rec, salt, role string) (err error) {
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}

		if userID == "" || username == "" || rec == "" || salt == "" || role == "" {
			return ErrCardInput
		}

		data, err := json.Marshal(UserInfo{
			ID:       userID,
			Username: username,
			Rec:      rec,
			Salt:     salt,
			Role:     role,
		})
		if err != nil {
			return err
		}

		return buk.Put([]byte(userID), data)
	})
}

// UserGetByUsername scans the users bucket for a matching username (small N).
// Returns ErrCardDBRead when not found.
func (b *UserDatabase) UserGetByUsername(username string) (info UserInfo, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}

		found := false
		if err := buk.ForEach(func(k, v []byte) error {
			var u UserInfo
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if strings.Compare(u.Username, username) == 0 {
				info = u
				found = true
			}
			return nil
		}); err != nil {
			return err
		}

		if !found {
			return ErrCardDBRead
		}
		return nil
	})
	return
}

// UserVerify checks the given password against the stored rec/salt for a user.
func (b *UserDatabase) UserVerify(username, pw string) (chk bool) {
	info, err := b.UserGetByUsername(username)
	if err != nil {
		return false
	}
	if strings.Compare(genPWHashWithSalt(username, pw, []byte(info.Salt)), info.Rec) == 0 {
		return true
	}
	return false
}

// CountAdmins returns the number of accounts with role == admin.
func (b *UserDatabase) CountAdmins() (count int, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}

		return buk.ForEach(func(k, v []byte) error {
			var u UserInfo
			if err := json.Unmarshal(v, &u); err != nil {
				return err
			}
			if strings.Compare(u.Role, RoleAdmin) == 0 {
				count++
			}
			return nil
		})
	})
	return
}

// UserNewID generates a stable userID (UUID v4 string).
func UserNewID() (string, error) {
	id, err := DbUUID.NewV4()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// userRecordExists reports whether any account already exists (used by the
// bootstrap logic in later stages).
func (b *UserDatabase) userRecordExists() (exists bool, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		exists = buk.Stats().KeyN > 0
		return nil
	})
	return
}

// ---------------------------------------------------------------------------
// Sessions (active login tokens)
// ---------------------------------------------------------------------------

// UserSession is the value stored for an active session.
type UserSession struct {
	UserID   string `json:"userID"`
	Username string `json:"username"`
	Role     string `json:"role"`
	LoginTime int64 `json:"loginTime"`
}

// SessionCreate writes a new active session keyed by token.
func (b *UserDatabase) SessionCreate(token, userID, username, role string, loginTime int64) (err error) {
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_SESS))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if token == "" || userID == "" || username == "" || role == "" {
			return ErrCardInput
		}
		data, err := json.Marshal(UserSession{
			UserID:    userID,
			Username:  username,
			Role:      role,
			LoginTime: loginTime,
		})
		if err != nil {
			return err
		}
		return buk.Put([]byte(token), data)
	})
}

// SessionGet returns the session for a token. ErrCardDBRead when not found.
func (b *UserDatabase) SessionGet(token string) (s UserSession, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_SESS))
		if buk == nil {
			return ErrCardDBNotExists
		}
		v := buk.Get([]byte(token))
		if v == nil {
			return ErrCardDBRead
		}
		return json.Unmarshal(v, &s)
	})
	return
}

// SessionDelete removes an active session (logout / rotation).
func (b *UserDatabase) SessionDelete(token string) (err error) {
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_SESS))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.Delete([]byte(token))
	})
}

// ---------------------------------------------------------------------------
// Login history (complete, append-only)
// ---------------------------------------------------------------------------

// LoginLogEntry is one row of the permanent login history.
type LoginLogEntry struct {
	LoginID   string `json:"loginID"`
	UserID    string `json:"userID"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	LoginTime int64  `json:"loginTime"`
}

// LoginLogAppend records a successful login. loginID should be a stable UUID.
func (b *UserDatabase) LoginLogAppend(loginID, userID, username, role string, loginTime int64) (err error) {
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_LOG))
		if buk == nil {
			return ErrCardDBNotExists
		}
		if loginID == "" || userID == "" || username == "" || role == "" {
			return ErrCardInput
		}
		data, err := json.Marshal(LoginLogEntry{
			LoginID:   loginID,
			UserID:    userID,
			Username:  username,
			Role:      role,
			LoginTime: loginTime,
		})
		if err != nil {
			return err
		}
		return buk.Put([]byte(loginID), data)
	})
}

// LoginLogList returns all login-history entries (oldest first by insertion
// order, which matches login chronology for UUID-ordered keys reasonably).
func (b *UserDatabase) LoginLogList() (list []LoginLogEntry, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_LOG))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(func(k, v []byte) error {
			var e LoginLogEntry
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			list = append(list, e)
			return nil
		})
	})
	return
}

// nowUnix is a small helper to keep call sites readable.
func nowUnix() int64 { return time.Now().Unix() }

// GenUserRec computes the stored password hash for a user account. Exported so
// callers (e.g. tests, account-seeding) can build a rec without reaching into
// the unexported genPWHashWithSalt.
func GenUserRec(username, pw string, salt []byte) string {
	return genPWHashWithSalt(username, pw, salt)
}

// UserLoginNew mints a new session token, writes the active session and a
// login-history entry, and returns the token. Used by AdminIn on success.
func (b *UserDatabase) UserLoginNew(userID, username, role string) (token string, err error) {
	tok, err := DbUUID.NewV4()
	if err != nil {
		return "", err
	}
	t := tok.String()
	now := nowUnix()
	if err := b.SessionCreate(t, userID, username, role, now); err != nil {
		return "", err
	}
	if err := b.LoginLogAppend(t, userID, username, role, now); err != nil {
		return "", err
	}
	return t, nil
}

// UserCheckLoginSession mirrors the legacy CardDatabase.CheckLoginSession API:
// returns the login time (curTimeout) when the token is valid and not expired,
// otherwise an error. Expiry uses API_SESSION_TIME (7 days).
func (b *UserDatabase) UserCheckLoginSession(token string) (curTimeout int64, err error) {
	s, err := b.SessionGet(token)
	if err != nil {
		return 0, err
	}
	if DbClock.Since(time.Unix(s.LoginTime, 0)) > API_SESSION_TIME {
		return 0, ErrCardDBRead
	}
	return s.LoginTime, nil
}

// ---------------------------------------------------------------------------
// Account mutation (Stage 5): password change, role change, session rotation
// ---------------------------------------------------------------------------

// UserGetByID returns the account record for a userID. ErrCardDBRead when not
// found.
func (b *UserDatabase) UserGetByID(userID string) (info UserInfo, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		v := buk.Get([]byte(userID))
		if v == nil {
			return ErrCardDBRead
		}
		return json.Unmarshal(v, &info)
	})
	return
}

// verifyUserPW reports whether pw matches the account's stored rec/salt.
func (b *UserDatabase) verifyUserPW(info UserInfo, pw string) bool {
	return genPWHashWithSalt(info.Username, pw, []byte(info.Salt)) == info.Rec
}

// UserChangePW resets the account's salt and recomputes rec for the new
// password. After a successful password change, all active sessions of the
// account are deleted (session rotation / forced re-login) for security.
func (b *UserDatabase) UserChangePW(userID, newPW string) (err error) {
	if newPW == "" {
		return ErrCardInput
	}
	info, err := b.UserGetByID(userID)
	if err != nil {
		return err
	}
	salt, _ := DbUUID.NewV4()
	rec := genPWHashWithSalt(info.Username, newPW, []byte(salt.String()))
	if err := b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		info.Rec = rec
		info.Salt = salt.String()
		info.MustChange = false // a successful change clears the forced flag
		data, e := json.Marshal(info)
		if e != nil {
			return e
		}
		return buk.Put([]byte(userID), data)
	}); err != nil {
		return err
	}
	// Session rotation: old tokens are no longer valid after the password change.
	return b.SessionDeleteForUser(userID)
}

// UserSetMustChange sets/clears the forced-password-change flag for an account.
func (b *UserDatabase) UserSetMustChange(userID string, mustChange bool) (err error) {
	info, err := b.UserGetByID(userID)
	if err != nil {
		return err
	}
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		info.MustChange = mustChange
		data, e := json.Marshal(info)
		if e != nil {
			return e
		}
		return buk.Put([]byte(userID), data)
	})
}

// UserList returns every account (id, username, role, mustChange) for the
// admin role-management UI. Password material is never included.
func (b *UserDatabase) UserList() (list []UserInfo, err error) {
	err = b.userDb.View(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		return buk.ForEach(func(k, v []byte) error {
			var info UserInfo
			if e := json.Unmarshal(v, &info); e != nil {
				return e
			}
			list = append(list, info)
			return nil
		})
	})
	return
}

// UserChangeRole changes the target account's role. It enforces the guard that
// the system must keep at least one admin: demoting an admin is rejected when
// it would leave zero admins. Returns ErrCardAdminFail in that case.
func (b *UserDatabase) UserChangeRole(targetUserID, newRole string) (err error) {
	if newRole != RoleAdmin && newRole != RoleOperator {
		return ErrCardInput
	}
	info, err := b.UserGetByID(targetUserID)
	if err != nil {
		return err
	}
	// Guard: do not remove the last admin.
	if info.Role == RoleAdmin && newRole != RoleAdmin {
		if c, e := b.CountAdmins(); e != nil {
			return e
		} else if c <= 1 {
			return ErrCardAdminFail
		}
	}
	if err := b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER))
		if buk == nil {
			return ErrCardDBNotExists
		}
		info.Role = newRole
		data, e := json.Marshal(info)
		if e != nil {
			return e
		}
		return buk.Put([]byte(targetUserID), data)
	}); err != nil {
		return err
	}
	// Session rotation: existing sessions keep the old role, so invalidate them
	// to force re-login with the updated role.
	return b.SessionDeleteForUser(targetUserID)
}

// SessionDeleteForUser removes every active session belonging to a user (used
// for rotation after a password or role change).
func (b *UserDatabase) SessionDeleteForUser(userID string) (err error) {
	return b.userDb.Update(func(tx ITx) error {
		buk := tx.Bucket([]byte(DB_USER_SESS))
		if buk == nil {
			return ErrCardDBNotExists
		}
		// Collect tokens to delete (bucket cannot be mutated during ForEach).
		var toDelete [][]byte
		if err := buk.ForEach(func(k, v []byte) error {
			var s UserSession
			if err := json.Unmarshal(v, &s); err != nil {
				return err
			}
			if s.UserID == userID {
				tk := make([]byte, len(k))
				copy(tk, k)
				toDelete = append(toDelete, tk)
			}
			return nil
		}); err != nil {
			return err
		}
		for _, k := range toDelete {
			if err := buk.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}
