package card_test

import (
	"os"
	"testing"

	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"

	"github.com/golang/mock/gomock"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// makeUserRoute builds an iris app whose login route uses AdminIn against a
// real multi-user store (userDB). The legacy cardDB mock is only consulted for
// the bootstrap (empty-store) path.
func makeUserRoute(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *httptest.Expect {
	app := iris.New()
	// Use an absolute temp dir for the iris session DB so the test runs
	// regardless of the current working directory.
	sess := card.MakeCardAdmin("test.com", t.TempDir())
	if userDB != nil {
		sess.AttachUserDB(userDB)
	}

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(sess.AdminIn(ctx, cardMock))
	})
	app.Get("/check", func(ctx iris.Context) {
		ctx.StatusCode(sess.AdminCheck(ctx))
	})

	e := httptest.New(t, app, httptest.URL("http://test.com"))
	return e
}

// TestStage3AdminLogin exercises Stage 3 end-to-end over HTTP:
//   - a normal user authenticates against the new store and gets a session,
//   - the empty-store bootstrap (legacy admin/123456) seeds the first admin and
//     writes a session + login-log entry,
//   - subsequent logins no longer touch the legacy CheckAdminLogin.
func TestStage3AdminLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)

	// (1) Operator logs in via the new store.
	dir, _ := os.MkdirTemp("", "stage3-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	opID, _ := card.UserNewID()
	opSalt, _ := card.UserNewID()
	opRec := card.GenUserRec("op1", "oppass", []byte(opSalt))
	if err := udb.UserAdd(opID, "op1", opRec, opSalt, card.RoleOperator); err != nil {
		t.Fatalf("seed operator: %v", err)
	}

	e := makeUserRoute(t, cardMock, udb)
	e.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).Expect().
		Status(card.SessionPassed)

	logs, err := udb.LoginLogList()
	if err != nil || len(logs) != 1 {
		t.Fatalf("expected 1 login log, got %d err=%v", len(logs), err)
	}

	// (2) Bootstrap: empty store + legacy admin/123456 seeds first admin.
	dir2, _ := os.MkdirTemp("", "stage3-boot")
	defer os.RemoveAll(dir2)
	boot, err := card.CreateUserDB(dir2)
	if err != nil {
		t.Fatalf("CreateUserDB boot: %v", err)
	}
	defer boot.Close()

	// Expect legacy CheckAdminLogin exactly once (bootstrap). The second login
	// must go through the new store and must NOT call the legacy path again.
	cardMock.EXPECT().CheckAdminLogin("admin", "123456").Return(true).Times(1)

	be := makeUserRoute(t, cardMock, boot)
	be.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).Expect().
		Status(card.SessionPassed)
	// Second login: same credentials, now via new store (no legacy call).
	be.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).Expect().
		Status(card.SessionPassed)

	c, err := boot.CountAdmins()
	if err != nil || c != 1 {
		t.Fatalf("expected 1 admin after bootstrap, got %d err=%v", c, err)
	}
	logs2, err := boot.LoginLogList()
	if err != nil || len(logs2) != 2 {
		t.Fatalf("expected 2 login logs, got %d err=%v", len(logs2), err)
	}
}
