package server

import (
	"errors"
	"html/template"
	"log"
	"marveldigital/tag-reader-server/card"
	"strings"
	"sync"
	"time"

	"github.com/kataras/iris/v12"
)

var (
	cardAdmin *card.CardAdmin

	// The session manager to improve timeout and performance
	AppSess AppSession
)

const baseTitle = "NFC tag verify server"

type AppSession struct {
	Ch chan bool

	updateTime time.Duration
	removeTime time.Duration

	session map[string]int64
	started bool
	ticker  *time.Ticker
	mu      sync.Mutex
}

func (u *AppSession) Update(token string, curTimeout int64) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.session == nil {
		u.session = make(map[string]int64)
	}
	u.session[token] = curTimeout
	if u.started {
		return
	}
	if u.updateTime == 0 {
		u.updateTime = time.Minute * 30
	}
	if u.removeTime == 0 {
		u.removeTime = time.Hour
	}

	go func() {
		u.mu.Lock()
		u.started = true
		u.ticker = time.NewTicker(u.updateTime)
		u.Ch = make(chan bool, 1)
		u.mu.Unlock()

		for range u.ticker.C {
			log.Println("update session")
			if err := cardDB.OnLoginUpdate(u.session); err != nil {
				logio.Println("auto session update failed")
			}
			u.mu.Lock()
			rmtime := u.removeTime
			u.mu.Unlock()
			for i, v := range u.session {
				if time.Since(time.Unix(v, 0)) > rmtime {
					delete(u.session, i)
				}
			}
			if !u.started || len(u.session) == 0 {
				break
			}
		}

		u.mu.Lock()
		u.session = nil
		u.ticker = nil
		u.started = false
		u.mu.Unlock()
		u.Ch <- true
	}()
}

func (u *AppSession) Kill() {
	if u.started && u.ticker != nil {
		log.Println("to kill the app session...")
		u.mu.Lock()
		u.started = false
		u.mu.Unlock()
		u.ticker.Reset(time.Microsecond)
		<-u.Ch
	}
}

func (u *AppSession) ChangeTime(update, remove time.Duration) error {
	if update <= 0 || remove <= 0 {
		return errors.New("duration not greater the 0")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.updateTime = update
	u.removeTime = remove
	return nil
}

func MakeAdminPage(app *iris.Application) {
	cardAdmin = card.MakeCardAdmin(hostname, dbPath)
	cardAdmin.MakeUserAdmin(dbPath)

	tmpl := iris.HTML(dbPath+"/html", ".html")
	tmpl.AddFunc("html", func(text string) template.HTML {
		return template.HTML(text)
	})
	app.RegisterView(tmpl)

	if haveIndex {
		app.Get("/", func(c iris.Context) { c.Redirect("/verify") })
	}
	// app.Favicon(dbPath+"/html/")
	app.Get("/verify", LoginPage)
	app.Get("/verify/admin/security", adminSecurity).Use(checkAdminVerify)
	app.Post("/verify/admin/changepw", adminChangePW).Use(checkAdminVerify)
	app.Post("/verify/admin/changerole", adminChangeRole).Use(checkAdminVerify)
	app.Get("/verify/admin/userlist", adminUserList).Use(checkAdminVerify)
	app.Post("/verify/admin/logout", adminLogout).Use(checkAdminVerify)
	app.Post("/verify/operator/logout", adminLogout).Use(checkOperatorVerify)
	// Unified logout: any authenticated user (admin or operator) may log
	// themselves out. The header's Sign Out always targets this endpoint so
	// logout works regardless of which page the user is currently on.
	app.Post("/verify/logout", adminLogout).Use(checkOperatorVerify)
	app.Post("/verify/admin/login", adminLogin)

	// Operator self-service security page: any authenticated user may change
	// their own password. Role management is hidden for operators by the
	// template, keeping /verify/admin/security strictly admin-only (Stage 4).
	app.Get("/verify/operator/security", adminSecurity).Use(checkOperatorVerify)
	app.Post("/verify/operator/changepw", adminChangePW).Use(checkOperatorVerify)
	// Unified self-service change-password endpoint: any authenticated user
	// (admin or operator) may change their own password. The security page
	// always targets this so the operator page doesn't accidentally POST to the
	// admin-only /verify/admin/changepw (which 404s for operators).
	app.Post("/verify/changepw", adminChangePW).Use(checkOperatorVerify)

	// Stage 6.5: admin user management (create account + admin reset password).
	// Admin-only (checkAdminVerify already denies operators with 404).
	app.Post("/verify/admin/createuser", adminCreateUser).Use(checkAdminVerify)
	app.Post("/verify/admin/resetpw", adminResetPW).Use(checkAdminVerify)

	r := app.HandleDir("js/admin", dbPath+"/js/admin")
	// Any authenticated user (admin or operator) may load the admin JS. The
	// privileged operations stay admin-only because the API routes are still
	// guarded by checkAdminVerify; the JS itself only contains client logic.
	// Without this, operators 404 on admin-common.js and logout()/change-pw are
	// undefined on their security page.
	r.Use(checkOperatorVerify)
	// Never cache the admin JS so a stale admin-common.js (which still pointed
	// to the admin-only logout endpoint) can't mask a fix.
	r.Use(func(ctx iris.Context) {
		ctx.Header("Cache-Control", "no-store")
		ctx.Next()
	})
}

func MakeIndex() {
	haveIndex = true
}

func checkAdminVerify(ctx iris.Context) {
	defer func() {
		if err := recover(); err != nil {
			logio.Printf("panic checkAdmin, %s", err)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.WriteString("500 Internal Server Error")
		}
	}()
	if token := ctx.GetHeader("X-token"); len(token) > 0 {
		switch ctx.RequestPath(true) {
		case "/verify/api/modellist":
		case "/verify/api/cardwrite":
		case "/verify/api/cardsearch":
		default:
			ctx.NotFound()
			return
		}
		if curTimeout, err := cardAdmin.UserSessionTimeout(token); err != nil {
			ctx.NotFound()
		} else {
			AppSess.Update(token, curTimeout)
			ctx.Next()
		}
		return
	}
	// GUI cookie path. Resolve the session once (CurrentUserFull opens the iris
	// session a single time, also covering AdminCheck's validity check).
	sess, _, ok := cardAdmin.CurrentUserFull(ctx)
	if !ok {
		ctx.NotFound()
		return
	}
	// Stage 4: GUI admin pages are admin-only. Operators (who may use the
	// /verify/api/... data endpoints) are denied access to the admin GUI.
	if sess.Role != card.RoleAdmin {
		ctx.NotFound()
		return
	}
	ctx.Next()
}

// checkOperatorVerify guards the operator self-service security page. Any
// authenticated user (admin or operator) may reach it to manage their own
// password; the role-management section is hidden for operators by the template.
func checkOperatorVerify(ctx iris.Context) {
	defer func() {
		if err := recover(); err != nil {
			logio.Printf("panic checkOperator, %s", err)
			ctx.StatusCode(iris.StatusInternalServerError)
			ctx.WriteString("500 Internal Server Error")
		}
	}()
	if _, _, ok := cardAdmin.CurrentUserFull(ctx); !ok {
		ctx.NotFound()
		return
	}
	ctx.Next()
}

func LoginPage(ctx iris.Context) {
	ctx.ViewLayout("page-layout.html")
	if cardAdmin.AdminCheck(ctx) != card.SessionPassed {
		ctx.ViewData("title", "Admin Login - "+baseTitle)
		ctx.ViewData("addon", "")
		ctx.ViewData("message", "")
		ctx.View("admin-login.html")
		return
	}
	// Already authenticated: route operators to their own page, admins to the
	// admin index. Falls back to /verify/admin when the role can't be resolved
	// (e.g. legacy cookie sessions without a user store).
	if sess, _, ok := cardAdmin.CurrentUserFull(ctx); ok && sess.Role == card.RoleOperator {
		ctx.Redirect("/verify/operator/security")
		return
	}
	ctx.Redirect("/verify/admin")
}

func loginFailLog(addr string) {
	if strings.Count(addr, ".") == 3 && strings.Count(addr, ":") == 1 {
		addrIp := addr[0:strings.Index(addr, ":")]
		logio.Printf("Login failed: %s", addrIp)
	} else {
		logio.Printf("Login failed with unknown: %s", addr)
	}
}

func adminLogin(ctx iris.Context) {
	if cardAdmin.AdminIn(ctx, cardDB) == card.SessionPassed {
		// Route operators to their own security page; admins to the admin index.
		if role, ok := cardAdmin.UserRoleOfToken(cardAdmin.LastLoginToken()); ok && role == card.RoleOperator {
			ctx.Redirect("/verify/operator/security")
		} else {
			ctx.Redirect("/verify/admin")
		}
	} else {
		logio.Printf("Login failed for user %q from %s", ctx.PostValue("inid"), ctx.Request().RemoteAddr)
		ctx.WriteString("Login failed")
	}
}

func adminSecurity(ctx iris.Context) {
	ctx.ViewLayout("page-layout.html")
	ctx.ViewData("navActiveL2", " active")
	// Stage 6: expose the current user's role so the template can show the
	// role-management section to admins only. Also expose whether this is the
	// bootstrap account, so the forced-password-change banner only appears for
	// the first account created from an empty userdb.
	if info, ok := cardAdmin.CurrentUserInfo(ctx); ok {
		ctx.ViewData("role", info.Role)
		ctx.ViewData("mustChange", info.MustChange)
		ctx.ViewData("username", info.Username)
		isBootstrap := false
		if db := cardAdmin.UserDB(); db != nil {
			isBootstrap = db.IsBootstrapUser(info.ID)
		}
		ctx.ViewData("isBootstrap", isBootstrap)
	}
	ctx.View("admin-security.html")
}

// adminUserList returns all accounts (id, username, role) for the admin
// role-management UI. Admin-only.
func adminUserList(ctx iris.Context) {
	if role, ok := cardAdmin.UserRoleInCtx(ctx); !ok || role != card.RoleAdmin {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Admin privileges required"})
		return
	}
	if cardAdmin == nil || !cardAdmin.HasUserDB() {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "user store unavailable"})
		return
	}
	list, err := cardAdmin.UserDB().UserList()
	if err != nil {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
		return
	}
	ctx.JSON(iris.Map{"msg": "OK", "users": list})
}

func adminChangePW(ctx iris.Context) {
	orgid := ctx.PostValue("orgid")
	orgpw := ctx.PostValue("orgpw")
	id := ctx.PostValue("changeid")
	pw := ctx.PostValue("changepw")
	pw2 := ctx.PostValue("changepw2")

	// Stage 5: in multi-user mode the current user changes their own password,
	// so changeid/orgid are not used. Keep the legacy checks for the old path.
	if cardAdmin != nil && cardAdmin.HasUserDB() {
		if len(orgpw) == 0 || len(pw) == 0 || len(pw2) == 0 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"msg": "FAIL", "info": "Bad request"})
			return
		}
	} else {
		if len(id) == 0 || len(pw) == 0 || len(pw2) == 0 || len(orgpw) == 0 {
			ctx.StatusCode(iris.StatusBadRequest)
			ctx.JSON(iris.Map{"msg": "FAIL", "info": "Bad request"})
			return
		}
	}
	if strings.Compare(pw, pw2) != 0 {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Password not same"})
		return
	}

	// Stage 5: prefer the multi-user store. The old password (orgpw) must match
	// the current account; on success sessions are rotated and the user is
	// logged out.
	if cardAdmin != nil && cardAdmin.HasUserDB() {
		if err := cardAdmin.ChangeOwnPassword(ctx, orgpw, pw); err == nil {
			// Session rotated: log the user out so they re-authenticate.
			cardAdmin.AdminOut(ctx)
			ctx.JSON(iris.Map{"msg": "OK", "info": "Password changed. Please log in again."})
		} else {
			ctx.JSON(iris.Map{"msg": "FAIL", "info": "Change password failed: " + err.Error()})
		}
		return
	}

	if err := cardDB.ChangeAdminPW(id, pw, orgid, orgpw); err == nil {
		ctx.JSON(iris.Map{"msg": "OK"})
	} else {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Change password failed: " + err.Error()})
	}
}

func adminLogout(ctx iris.Context) {
	cardAdmin.LogoutCurrentUser(ctx)
	ctx.Redirect("/verify/")
}

// adminChangeRole changes the role of a target user. It is admin-only (enforced
// here and by checkAdminVerify for the GUI). The underlying store enforces the
// >=1 admin guard, so the last admin cannot be demoted. On success the target's
// sessions are rotated (forced re-login).
func adminChangeRole(ctx iris.Context) {
	// Admin-only: operators must not reach this even via the API path.
	if role, ok := cardAdmin.UserRoleInCtx(ctx); !ok || role != card.RoleAdmin {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Admin privileges required"})
		return
	}
	targetID := ctx.PostValue("userid")
	newRole := ctx.PostValue("role")
	if targetID == "" || (newRole != card.RoleAdmin && newRole != card.RoleOperator) {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Bad request"})
		return
	}
	if err := cardAdmin.ChangeUserRole(targetID, newRole); err == nil {
		ctx.JSON(iris.Map{"msg": "OK", "info": "Role updated. User must log in again."})
	} else {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Change role failed: " + err.Error()})
	}
}

// adminCreateUser creates a new account (admin-only). The new account is forced
// to change its password on first login.
func adminCreateUser(ctx iris.Context) {
	if role, ok := cardAdmin.UserRoleInCtx(ctx); !ok || role != card.RoleAdmin {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Admin privileges required"})
		return
	}
	if cardAdmin == nil || !cardAdmin.HasUserDB() {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "user store unavailable"})
		return
	}
	username := ctx.PostValue("username")
	pw := ctx.PostValue("newpw")
	role := ctx.PostValue("role")
	if username == "" || pw == "" || (role != card.RoleAdmin && role != card.RoleOperator) {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Bad request"})
		return
	}
	if id, err := cardAdmin.CreateUser(username, pw, role); err == nil {
		ctx.JSON(iris.Map{"msg": "OK", "id": id})
	} else {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
	}
}

// adminResetPW resets a user's password on behalf of an admin (admin-only). The
// target is flagged MustChange and rotated, forcing re-login + self change.
func adminResetPW(ctx iris.Context) {
	if role, ok := cardAdmin.UserRoleInCtx(ctx); !ok || role != card.RoleAdmin {
		ctx.StatusCode(iris.StatusForbidden)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Admin privileges required"})
		return
	}
	if cardAdmin == nil || !cardAdmin.HasUserDB() {
		ctx.StatusCode(iris.StatusInternalServerError)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "user store unavailable"})
		return
	}
	userID := ctx.PostValue("userid")
	pw := ctx.PostValue("newpw")
	if userID == "" || pw == "" {
		ctx.StatusCode(iris.StatusBadRequest)
		ctx.JSON(iris.Map{"msg": "FAIL", "info": "Bad request"})
		return
	}
	if err := cardAdmin.AdminResetPassword(userID, pw); err == nil {
		ctx.JSON(iris.Map{"msg": "OK"})
	} else {
		ctx.JSON(iris.Map{"msg": "FAIL", "info": err.Error()})
	}
}
