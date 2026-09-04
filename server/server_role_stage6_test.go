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

// stage6App wires the real admin + operator security routes so the Stage 6 UI
// (role selector + show/hide password toggle + mustChange banner) can be
// exercised. No 302 redirect is asserted here — that behaviour is deferred to
// manual server testing.
func stage6App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	_, thisFile, _, _ := runtime.Caller(0)
	htmlDir := filepath.Join(filepath.Dir(thisFile), "..", "local", "html")
	app.RegisterView(iris.HTML(htmlDir, ".html"))
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	app.Get("/verify/admin/security", adminSecurity).Use(checkAdminVerify)
	app.Post("/verify/admin/changepw", adminChangePW).Use(checkAdminVerify)
	app.Post("/verify/admin/changerole", adminChangeRole).Use(checkAdminVerify)
	app.Get("/verify/admin/userlist", adminUserList).Use(checkAdminVerify)
	app.Get("/verify/operator/security", adminSecurity).Use(checkOperatorVerify)
	app.Post("/verify/operator/changepw", adminChangePW).Use(checkOperatorVerify)

	return app
}

// TestStage6SecurityUI verifies the Stage 6 security-page UI: role management is
// shown to admins (and carries the mustChange banner for the bootstrap admin),
// hidden for operators, and operators can still change their own password.
func TestStage6SecurityUI(t *testing.T) {
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

	// --- Bootstrap admin (MustChange=true) ---
	e := httptest.New(t, app, httptest.URL("http://test.com"))
	e.POST("/login").WithForm(iris.Map{"inid": "admin", "inpw": "123456"}).
		Expect().Status(card.SessionPassed)
	sec := e.GET("/verify/admin/security").Expect()
	sec.Status(iris.StatusOK)
	// Role-management section is shown to admins.
	sec.Body().Contains("userSelect")
	// The mustChange warning banner is shown.
	sec.Body().Contains("default bootstrap credentials")

	// Admin userlist reachable and never exposes password material (rec/salt).
	ul := e.GET("/verify/admin/userlist").Expect()
	ul.Status(iris.StatusOK)
	ul.JSON().Object().Value("msg").Equal("OK")
	ul.Body().NotContains(`"rec"`)
	ul.Body().NotContains(`"salt"`)

	// --- Operator: reaches the operator security page, no role section ---
	id, _ := card.UserNewID()
	salt, _ := card.UserNewID()
	rec := card.GenUserRec("op1", "oppass", []byte(salt))
	if err := udb.UserAdd(id, "op1", rec, salt, card.RoleOperator); err != nil {
		t.Fatalf("seed op: %v", err)
	}
	op := httptest.New(t, app, httptest.URL("http://test.com"))
	op.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)
	opSec := op.GET("/verify/operator/security").Expect()
	opSec.Status(iris.StatusOK)
	// Role management must be hidden for operators.
	opSec.Body().NotContains("userSelect")
	// But the change-password form is present.
	opSec.Body().Contains("Change password")

	// Operator can change their own password (rotation) via the operator endpoint.
	op.POST("/verify/operator/changepw").WithForm(iris.Map{
		"orgid":    "op1",
		"orgpw":    "oppass",
		"changeid": "op1",
		"changepw": "newoppass",
		"changepw2": "newoppass",
	}).Expect().JSON().Object().Value("msg").Equal("OK")
	// Old password no longer verifies after rotation.
	if udb.UserVerify("op1", "oppass") {
		t.Fatalf("store: old oppass should NOT verify after change")
	}
}
