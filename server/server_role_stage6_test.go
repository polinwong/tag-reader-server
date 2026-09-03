package server

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"

	"github.com/golang/mock/gomock"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// stage6App wires checkAdminVerify + the Stage 6 endpoints and a probe GET route
// used to verify the forced-password-change redirect.
func stage6App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	// Register the view engine so adminSecurity (which renders admin-security.html
	// via the page-layout) can be exercised in the test.
	_, thisFile, _, _ := runtime.Caller(0)
	htmlDir := filepath.Join(filepath.Dir(thisFile), "..", "local", "html")
	app.RegisterView(iris.HTML(htmlDir, ".html"))
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	app.Get("/verify/admin/probe", func(ctx iris.Context) {
		ctx.StatusCode(iris.StatusOK)
	}).Use(checkAdminVerify)
	app.Get("/verify/admin/security", adminSecurity).Use(checkAdminVerify)
	app.Post("/verify/admin/changepw", adminChangePW).Use(checkAdminVerify)
	app.Post("/verify/admin/changerole", adminChangeRole).Use(checkAdminVerify)
	app.Get("/verify/admin/userlist", adminUserList).Use(checkAdminVerify)

	return app
}

func TestStage6SecurityUIAndForcedPW(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	// No seeded users -> bootstrap path; legacy admin/123456 accepted once.
	cardMock.EXPECT().CheckAdminLogin("admin", "123456").Return(true).AnyTimes()
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "stage6-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	app := stage6App(t, cardMock, udb)

	// --- Bootstrap login (admin/123456) ---
	// Bootstrap admin must change password -> probe redirects to security.
	// Use a raw client to inspect the redirect. httpexpect (Expect) does not
	// follow redirects, so the 302 + Location is observable directly.
	raw := httptest.New(t, app, httptest.URL("http://test.com"))
	raw.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).
		Expect().Status(card.SessionPassed)
	if info, err := udb.UserGetByID("admin"); err != nil {
		t.Fatalf("bootstrap: admin not found: %v", err)
	} else {
		t.Logf("bootstrap: role=%s mustChange=%v", info.Role, info.MustChange)
	}
	raw.GET("/verify/admin/probe").
		Expect().Status(iris.StatusFound).
		Header("Location").Equal("/verify/admin/security")

	// Security page itself is reachable.
	raw2 := httptest.New(t, app, httptest.URL("http://test.com"))
	raw2.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).
		Expect().Status(card.SessionPassed)
	raw2.GET("/verify/admin/security").Expect().Status(iris.StatusOK)

	// --- Change password (clears MustChange) ---
	raw3 := httptest.New(t, app, httptest.URL("http://test.com"))
	raw3.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).
		Expect().Status(card.SessionPassed)
	raw3.POST("/verify/admin/changepw").WithForm(iris.Map{
		"orgid":    "admin",
		"orgpw":    "123456",
		"changeid": "admin",
		"changepw": "newpw1",
		"changepw2": "newpw1",
	}).Expect().JSON().Object().Value("msg").Equal("OK")

	// Direct store-level verification after the change.
	if !udb.UserVerify("admin", "newpw1") {
		t.Fatalf("store: newpw1 should verify after change")
	}
	if udb.UserVerify("admin", "123456") {
		t.Fatalf("store: old 123456 should NOT verify after change")
	}

	// Now MustChange is cleared: a fresh login (new password) is NOT redirected.
	raw4 := httptest.New(t, app, httptest.URL("http://test.com"))
	raw4.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "newpw1"}).
		Expect().Status(card.SessionPassed)
	probeResp := raw4.GET("/verify/admin/probe").Expect().Raw()
	if probeResp.StatusCode != 200 {
		t.Fatalf("after pw change, probe should be 200, got %d", probeResp.StatusCode)
	}
	// userlist reachable for admin.
	raw4.GET("/verify/admin/userlist").Expect().JSON().Object().
		Value("msg").Equal("OK")

	// --- Operator is blocked from the admin GUI (userlist) ---
	// Seed an operator and log in.
	id, _ := card.UserNewID()
	salt, _ := card.UserNewID()
	rec := card.GenUserRec("op1", "oppass", []byte(salt))
	if err := udb.UserAdd(id, "op1", rec, salt, card.RoleOperator); err != nil {
		t.Fatalf("seed op: %v", err)
	}
	opCli := httptest.New(t, app, httptest.URL("http://test.com"))
	opCli.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)
	// Operators are denied admin GUI pages by checkAdminVerify (404).
	opCli.GET("/verify/admin/userlist").Expect().Status(iris.StatusNotFound)
}
