package server

import (
	"os"
	"testing"

	"marveldigital/tag-reader-server/card"
	"marveldigital/tag-reader-server/card/mock"

	"github.com/golang/mock/gomock"
	"github.com/kataras/iris/v12"
	"github.com/kataras/iris/v12/httptest"
)

// stage5App wires the multi-user CardAdmin with the Stage 5 mutation endpoints
// (changepw, changerole) plus a protected probe route to test session validity.
func stage5App(t *testing.T, cardMock *mock.MockICardDB, userDB *card.UserDatabase) *iris.Application {
	t.Helper()
	app := iris.New()
	cardAdmin = card.MakeCardAdmin("test.com", t.TempDir())
	cardAdmin.AttachUserDB(userDB)

	app.Post("/login", func(ctx iris.Context) {
		ctx.StatusCode(cardAdmin.AdminIn(ctx, cardMock))
	})
	app.Post("/verify/admin/changepw", adminChangePW).Use(checkAdminVerify)
	app.Post("/verify/admin/changerole", adminChangeRole).Use(checkAdminVerify)
	app.Get("/verify/admin/security", func(ctx iris.Context) {
		ctx.StatusCode(iris.StatusOK)
	}).Use(checkAdminVerify)

	return app
}

func seedUser(t *testing.T, udb *card.UserDatabase, user, pw, role string) {
	t.Helper()
	id, _ := card.UserNewID()
	salt, _ := card.UserNewID()
	rec := card.GenUserRec(user, pw, []byte(salt))
	if err := udb.UserAdd(id, user, rec, salt, role); err != nil {
		t.Fatalf("seed %s: %v", user, err)
	}
}

func TestStage5PasswordAndRole(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	cardMock := mock.NewMockICardDB(ctrl)
	cardMock.EXPECT().CheckAdminLogin(gomock.Any(), gomock.Any()).Return(false).AnyTimes()

	dir, _ := os.MkdirTemp("", "stage5-userdb")
	defer os.RemoveAll(dir)
	udb, err := card.CreateUserDB(dir)
	if err != nil {
		t.Fatalf("CreateUserDB: %v", err)
	}
	defer udb.Close()

	seedUser(t, udb, "admin1", "adminpw", card.RoleAdmin)
	seedUser(t, udb, "op1", "oppass", card.RoleOperator)

	app := stage5App(t, cardMock, udb)

	// --- Password change with session rotation ---
	cli := httptest.New(t, app, httptest.URL("http://test.com"))
	cli.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "adminpw"}).
		Expect().Status(card.SessionPassed)
	// Authenticated before change.
	cli.GET("/verify/admin/security").Expect().Status(iris.StatusOK)
	// Change own password (old=adminpw, new=newadminpw).
	cli.POST("/verify/admin/changepw").WithForm(iris.Map{
		"orgid":    "admin1",
		"orgpw":    "adminpw",
		"changeid": "admin1",
		"changepw": "newadminpw",
		"changepw2": "newadminpw",
	}).Expect().JSON().Object().Value("msg").Equal("OK")
	// Session rotated: old cookie no longer valid -> 404.
	cli.GET("/verify/admin/security").Expect().Status(iris.StatusNotFound)
	// Can log in with the new password.
	cli2 := httptest.New(t, app, httptest.URL("http://test.com"))
	cli2.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "newadminpw"}).
		Expect().Status(card.SessionPassed)

	// Wrong old password rejected (as the current admin user).
	cli3 := httptest.New(t, app, httptest.URL("http://test.com"))
	cli3.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "newadminpw"}).
		Expect().Status(card.SessionPassed)
	cli3.POST("/verify/admin/changepw").WithForm(iris.Map{
		"orgid":    "admin1",
		"orgpw":    "wrongpw",
		"changeid": "admin1",
		"changepw": "x",
		"changepw2": "x",
	}).Expect().JSON().Object().Value("msg").Equal("FAIL")

	// --- Role change (admin only) ---
	adminCli := httptest.New(t, app, httptest.URL("http://test.com"))
	adminCli.POST("/login").WithForm(iris.Map{"inid": "admin1", "inpw": "newadminpw"}).
		Expect().Status(card.SessionPassed)
	// Log in op1 BEFORE promotion to capture the pre-change session.
	opCli := httptest.New(t, app, httptest.URL("http://test.com"))
	opCli.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)
	// Promote op1 to admin.
	op1, _ := udb.UserGetByUsername("op1")
	adminCli.POST("/verify/admin/changerole").WithForm(iris.Map{
		"userid": op1.ID, "role": card.RoleAdmin,
	}).Expect().JSON().Object().Value("msg").Equal("OK")
	// op1's pre-promotion session was rotated -> 404 (must re-login with new role).
	opCli.GET("/verify/admin/security").Expect().Status(iris.StatusNotFound)
	// op1 can now log in fresh as admin.
	opCli2 := httptest.New(t, app, httptest.URL("http://test.com"))
	opCli2.POST("/login").WithForm(iris.Map{"inid": "op1", "inpw": "oppass"}).
		Expect().Status(card.SessionPassed)
	opCli2.GET("/verify/admin/security").Expect().Status(iris.StatusOK)

	// --- Guard: cannot demote the last remaining admin ---
	// Now admin1 and op1 are both admins. Demote op1 back to operator first.
	adminCli.POST("/verify/admin/changerole").WithForm(iris.Map{
		"userid": op1.ID, "role": card.RoleOperator,
	}).Expect().JSON().Object().Value("msg").Equal("OK")
	// admin1 is the only admin now; demoting it must FAIL.
	admin1, _ := udb.UserGetByUsername("admin1")
	adminCli.POST("/verify/admin/changerole").WithForm(iris.Map{
		"userid": admin1.ID, "role": card.RoleOperator,
	}).Expect().JSON().Object().Value("msg").Equal("FAIL")
}
