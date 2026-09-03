package card

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"log"
	"net/http"
	"os"
	"path"
	"time"

	"github.com/etcd-io/bbolt"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/sessions"
	"github.com/kataras/iris/v12/sessions/sessiondb/boltdb"
)

// SourceSession the session id for detect client
const SourceSession = "VerifyServer"

// Session result code
const (
	SessionPassed      = 200
	SessionPassedAdmin = 201
	SessionPassedTv    = 202
	SessionVersionOld  = 401
	SessionNoVerify    = 402
)

const pwsalt = "2f6a4c16e84e"
const password = "5d1bcaa20ff4d815b4972639c2885cf31c3a4ee7cbe335f7a55433d8486acdba"

// GenPWHash the function to create user pw hash pair
func GenPWHash(username string, pw string) (hash string) {

	mac := hmac.New(sha256.New, []byte(pwsalt))
	mac.Write([]byte(username + pw))
	outMAC := mac.Sum(nil)
	hash = fmt.Sprintf("%x", outMAC)

	return
}

type CardAdmin struct {
	sess       *sessions.Sessions
	basedb     sessions.Database
	CookieName string
	Hostname   string // deprecated

	// userDB is the multi-user account store (Stage 1+). Nil in unit tests
	// that build CardAdmin without MakeCardAdmin; the legacy cookie flow is
	// used then to keep behaviour backward compatible.
	userDB *UserDatabase

	// lastToken holds the session token minted by the most recent AdminIn,
	// so API login (verifyApiLogin) can return it to the client.
	lastToken string
}

func createSessionDB(s *sessions.Sessions, dbtarget string) (sessions.Database, error) {
	basedb, err := bbolt.Open(dbtarget, os.FileMode(0640), &bbolt.Options{})
	if err != nil {
		return nil, err
	}
	db, err2 := boltdb.NewFromDB(basedb, "irissession")
	if err2 != nil {
		return nil, err2
	}
	s.UseDatabase(db)
	return db, err
}

func MakeCardAdmin(hostname, dbpath string) (a *CardAdmin) {
	a = new(CardAdmin)
	a.CookieName = SourceSession
	a.sess = sessions.New(sessions.Config{Cookie: a.CookieName, Expires: time.Hour * 24 * 7})

	if db, dberr := createSessionDB(a.sess, path.Join(dbpath, "adminsession.db")); dberr != nil {
		log.Printf("session error: %s\n", dberr)
		log.Println("session database cannot create, use ram cache session")
	} else {
		a.basedb = db
	}

	if hostname == "" {
		// log.Printf("Warning: cookie host have not setup, please set it before user login")
	} else {
		a.Hostname = hostname
	}

	return
}

// MakeUserAdmin opens the multi-user account database (userdb.db) and attaches
// it to the admin session manager. Called from the server init after
// MakeCardAdmin. Safe to call once; subsequent calls are ignored.
func (a *CardAdmin) MakeUserAdmin(dbpath string) {
	if a.userDB != nil {
		return
	}
	if db, err := CreateUserDB(dbpath); err != nil {
		log.Printf("user database error: %s\n", err)
		log.Println("user database cannot open, login will use legacy path")
	} else {
		a.userDB = db
	}
}

// AttachUserDB attaches an already-opened UserDatabase (used by server init and
// tests). It is a no-op if a user DB is already attached.
func (a *CardAdmin) AttachUserDB(db *UserDatabase) {
	if a.userDB == nil {
		a.userDB = db
	}
}

func (a *CardAdmin) openSession(ctx iris.Context) (nsess *sessions.Session) {
	nsess = a.sess.Start(ctx, func(c *http.Cookie) {
		c.Domain = ctx.Host() // Use wildcard domain for multiple domain access - 2024-11-29 by Kevin Mak
	})
	return
}

// AdminOut logout user
func (a *CardAdmin) AdminOut(ctx iris.Context) {
	a.openSession(ctx).Destroy()
}

// AdminCheck check user have login (GUI cookie path).
func (a *CardAdmin) AdminCheck(ctx iris.Context) int {
	curs := a.openSession(ctx)
	if a.userDB != nil {
		if token := curs.GetString("token"); token != "" {
			if _, ok := a.ResolveSession(token); ok {
				return SessionPassed
			}
		}
		return SessionNoVerify
	}
	// Legacy fallback (unit tests / pre-userDB): boolean cookie.
	if b, err := curs.GetBoolean("login"); err == nil && b {
		return SessionPassed
	}
	return SessionNoVerify
}

// AdminIn checks the user's right to login and, on success, creates a session
// in the multi-user store (when available) and sets the GUI cookie to the
// session token. Returns SessionPassed / SessionNoVerify.
func (a *CardAdmin) AdminIn(ctx iris.Context, db ICardDB) int {
	id := ctx.PostValue("inid")
	pw := ctx.PostValue("inpw")

	token, status := a.adminLoginCore(id, pw, db)
	if status == SessionPassed && token != "" {
		a.openSession(ctx).Set("token", token)
	}
	return status
}

// adminLoginCore performs the credential check and, on success, creates the
// session + login-log entry, returning the token. It is separated from
// AdminIn so the logic is testable without an iris context.
func (a *CardAdmin) adminLoginCore(id, pw string, db ICardDB) (token string, status int) {
	a.lastToken = ""

	// Primary path: verify against the multi-user store.
	if a.userDB != nil {
		info, err := a.userDB.UserGetByUsername(id)
		if err == nil && a.userDB.UserVerify(id, pw) {
			t, e := a.userDB.UserLoginNew(info.ID, info.Username, info.Role)
			if e != nil {
				return "", SessionNoVerify
			}
			a.lastToken = t
			return t, SessionPassed
		}

		// Bootstrap: no accounts yet -> accept legacy admin/123456 (single
		// bootstrap admin) and seed the first account into the user store.
		// After seeding, subsequent logins use the new store only.
		if exists, e := a.userDB.userRecordExists(); e == nil && !exists {
			if db.CheckAdminLogin(id, pw) {
				salt, _ := DbUUID.NewV4()
				rec := genPWHashWithSalt(id, pw, []byte(salt.String()))
				if aerr := a.userDB.UserAdd(id, id, rec, salt.String(), RoleAdmin); aerr == nil {
					t, te := a.userDB.UserLoginNew(id, id, RoleAdmin)
					if te == nil {
						a.lastToken = t
						return t, SessionPassed
					}
				}
			}
		}
		return "", SessionNoVerify
	}

	// Legacy path (no userDB): keep old behaviour for tests/compatibility.
	if db.CheckAdminLogin(id, pw) {
		return "", SessionPassed
	}
	return "", SessionNoVerify
}

// ResolveSession returns the active session for a token (used by API auth and
// AdminCheck). The boolean reports validity (token present and not expired).
func (a *CardAdmin) ResolveSession(token string) (UserSession, bool) {
	if a.userDB == nil {
		return UserSession{}, false
	}
	s, err := a.userDB.SessionGet(token)
	if err != nil {
		return UserSession{}, false
	}
	if DbClock.Since(time.Unix(s.LoginTime, 0)) > API_SESSION_TIME {
		return UserSession{}, false
	}
	return s, true
}

// LastLoginToken returns the session token minted by the most recent AdminIn.
func (a *CardAdmin) LastLoginToken() string {
	return a.lastToken
}

// UserRoleInCtx resolves the role of the account behind the current GUI
// session (cookie). It returns the role and true when a valid session token is
// present; otherwise ("", false). Used by the server's role-based authorization
// for admin GUI pages (Stage 4).
func (a *CardAdmin) UserRoleInCtx(ctx iris.Context) (role string, ok bool) {
	if a.userDB == nil {
		return "", false
	}
	token := a.openSession(ctx).GetString("token")
	if token == "" {
		return "", false
	}
	return a.UserRoleOfToken(token)
}

// UserRoleOfToken resolves a session token to the account's role. It returns
// the role and true when the token is present, not expired, and maps to a
// known session; otherwise ("", false). Used by the server's role-based
// authorization in checkAdminVerify (Stage 4).
func (a *CardAdmin) UserRoleOfToken(token string) (role string, ok bool) {
	s, ok := a.ResolveSession(token)
	if !ok {
		return "", false
	}
	return s.Role, true
}

// UserSessionTimeout mirrors the legacy CheckLoginSession API for AppSess.
func (a *CardAdmin) UserSessionTimeout(token string) (int64, error) {
	if a.userDB == nil {
		return 0, ErrCardDBRead
	}
	return a.userDB.UserCheckLoginSession(token)
}

// CloseDB for stop database connection, DON't USE session after close DB
func (a *CardAdmin) CloseDB() (err error) {
	switch a.basedb.(type) {
	case *boltdb.Database:
		err = a.basedb.(*boltdb.Database).Close()
	}
	return
}
