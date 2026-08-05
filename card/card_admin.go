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

// AdminCheck check user have login
func (a *CardAdmin) AdminCheck(ctx iris.Context) int {
	if b, err := a.openSession(ctx).GetBoolean("login"); err == nil && b {
		return SessionPassed
	}
	return SessionNoVerify
}

// AdminIn check user a right to login
func (a *CardAdmin) AdminIn(ctx iris.Context, db ICardDB) int {
	id := ctx.PostValue("inid")
	pw := ctx.PostValue("inpw")
	if db.CheckAdminLogin(id, pw) {
		curs := a.openSession(ctx)
		curs.Set("login", true)
		return SessionPassed
	}
	return SessionNoVerify
}

// CloseDB for stop database connection, DON't USE session after close DB
func (a *CardAdmin) CloseDB() (err error) {
	switch a.basedb.(type) {
	case *boltdb.Database:
		err = a.basedb.(*boltdb.Database).Close()
	}
	return
}
