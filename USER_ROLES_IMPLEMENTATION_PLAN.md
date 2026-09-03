# User Roles & Multi-User Account Implementation Plan

> Status: **Design / plan only. No code has been written yet.**
> Scope: add `admin` and `operator` roles, support multiple users (admins and
> operators) each with their own password, introduce a dedicated user database,
> and keep a complete login-history log. This is a focused subset of the broader
> hardening described in `endpoint-separation-login-hardening.md`.

## 1. Goals (confirmed with user)

- Support **multiple users**, each with their own username + password.
- Two roles: **`admin`** (full access) and **`operator`** (limited access).
- Role can be changed later (admin → operator, operator → admin) by an admin.
- At least **one admin must always exist** (guard: role change/deletion rejected
  if it would leave zero admins).
- Complete login record: who logged in and when (per-user, full history).
- **Minimal change** to clients (Vue/desktop/mobile): they already send the
  username; the server is the sole gatekeeper. Existing clients need no change.
- **Preserve all existing data**: only add rows / buckets / keys. Never modify
  the existing `rec`/`salt` in `admin-hash`, never alter `carddata.db`.
- GUI continues to use the iris cookie session (`adminsession.db`) as transport
  (minimal-change constraint) — the cookie will carry the session token.

## 2. Current credential model (as read from code, not modified)

- `local/admin.db` bucket `admin-hash`: two fixed keys `rec` and `salt`.
  `rec = HMAC-SHA256(key=salt, msg=username+password)`, hex-encoded
  (`genPWHashWithSalt`, card_db.go:546). Username is folded into the hash, not
  stored as a key. `ChangeAdminPW` (card_db.go:485) rewrites `rec`/`salt`.
- Legacy fallback: if `rec` absent, `CheckAdminLogin` (card_db.go:513) compares
  `GenPWHash(username, pw)` to hardcoded `password` constant
  (`card_admin.go:32` = the `admin`/`123456` default).
- Sessions today: `admin-record` bucket (token → 8-byte login timestamp) in
  `admin.db`, plus iris cookie `login` boolean in `adminsession.db`. Neither
  stores username or role.

## 3. New database file: `local/userdb.db`

A new Bolt DB, **separate** from `admin.db` and `carddata.db`. Contains:

### Bucket `users`
| Key | Value (JSON) |
|-----|--------------|
| `userID` (stable id, e.g. UUID or uint) | `{ username, rec, salt, role }` |

- `username` — **immutable** after creation (username change is disallowed).
- `rec` = `HMAC-SHA256(key=salt, msg=username+password)`, reusing existing
  `genPWHashWithSalt` scheme. Username is fixed, so `rec` only recomputes on
  **password change**.
- `salt` — per-user random value (UUID string, as in `ChangeAdminPW`).
- `role` — `"admin"` or `"operator"`.

### Bucket `sessions` (active sessions; replaces `admin-record` usage)
| Key | Value (JSON) |
|-----|--------------|
| `token` (UUID) | `{ userID, username, role, loginTime }` |

- `loginTime` = `time.Now().Unix()` at login; expiry checked against
  `API_SESSION_TIME` (7 days, card_db.go:30), same as today.
- Resolves both the API `X-token` path and the GUI cookie path (cookie stores
  the token; `checkAdminVerify` reads `sessions[token]` to get user+role).

### Bucket `loginlog` (complete history; appended on every successful login)
| Key | Value (JSON) |
|-----|--------------|
| `loginID` (UUID or `loginTime+userID`) | `{ userID, username, role, loginTime }` |

- Never deleted on logout; gives a permanent "who and when" record.
- `getLoginRecord` (server_card_verify.go:157) repoints here for the history view.

## 4. Username lookup

- By **scanning** the `users` bucket for the row whose `Value.username` matches
  (N is small, <10 users; delay acceptable). No separate index bucket needed.

## 5. Roles & endpoint authorization

Confirmed operator-allowed endpoints (read/limited):
- `cardwrite`
- `modellist`
- `modelsearch`
- `cardsearch`

All other `/verify/api/*` and `/verify/admin/*` routes are **admin-only**.

`checkAdminVerify` (server_admin.go:136) is extended: after resolving the
session → `{userID, username, role}`, it permits operator-only paths for
`operator` and everything for `admin`; otherwise `NotFound()` (information
hiding, consistent with current behavior). The three operator API paths already
bypass the cookie requirement today (server_admin.go:145-152); they keep that but
now also carry role context.

## 6. First-admin bootstrap & forced password change

- On first run, `users` bucket is empty. Login via the legacy `admin/123456`
  path (legacy `admin-hash` fallback) is accepted **once**.
- Immediately after that successful login, the server:
  1. Creates the first `users` row: `userID`, `username="admin"`,
     `role="admin"`, with `rec`/`salt` computed from the **new** password the
     admin sets.
  2. **Disables the legacy `admin-hash` fallback** so `admin/123456` no longer
     works. (The existing `admin-hash` `rec`/`salt` rows are left untouched on
     disk — we only stop relying on the fallback; this respects the
     "preserve existing data" rule.)
- Forced password change on first login: redirect the admin to
  `/verify/admin/security` (the existing `adminChangePW` page,
  server_admin.go:204) and block other admin pages until the password is
  changed. Username `admin` is acceptable (security rests on the password, which
  is changed immediately); username itself is not secret.
- After bootstrap, all accounts are created/edited through the admin page.

## 7. Password & role change behavior

- **Password change**: allowed for the user themselves (admin or operator).
  On change, generate a new `salt`, recompute `rec = genPWHashWithSalt(username,
  newpw, salt)`, and write `rec`+`salt` into the user's `Value` immediately.
- **Role change**: admin-only. Updates `Value.role` in place.
- **Admin-count guard**: before applying any admin→operator (or admin delete)
  change, count `role=="admin"` in `users`; reject if it would drop to zero.

## 8. Login flow (end to end, new design)

1. `POST /verify/admin/login` → `adminLogin` → `AdminIn`:
   - read `inid` (username) + `inpw` (password).
   - look up user by scanning `users` for matching `username`; verify
     `genPWHashWithSalt` against stored `rec`/`salt`.
   - (bootstrap) if `users` empty and legacy fallback matches `admin/123456`,
     proceed to forced-pw-change flow.
   - on success: generate `token`, write `sessions[token]` and append
     `loginlog[loginID]`; set iris cookie holding the `token`.
2. `POST /verify/api/login` (app): same verification; returns `{"token": ...}`
   as today (server_card_verify.go:131).
3. `checkAdminVerify`: resolve `token` (from `X-token` header or cookie) →
   `sessions[token]` → `{userID, username, role}`; enforce role-based paths.

## 9. Files & functions to change (plan only — not yet implemented)

| File | Function / area | Change |
|------|----------------|--------|
| `card/card_db.go` | new `CreateUserDB` / `makeDB` for `userdb.db` | open new DB, ensure `users`/`sessions`/`loginlog` buckets |
| `card/card_db.go` | `UserAdd`, `UserGetByUsername` (scan), `UserVerify`, `UserChangePW`, `UserChangeRole`, `CountAdmins`, `SessionCreate`, `SessionGet`, `SessionDelete`, `LoginLogAppend`, `LoginLogList` | new functions |
| `card/card_admin.go` | `AdminIn` | use new user verification + session write + loginlog append; bootstrap path |
| `card/card_admin.go` | `AdminCheck` / cookie handling | cookie now stores `token`; resolve via `sessions` |
| `server/server_admin.go` | `checkAdminVerify` | role-based path enforcement; resolve token→user/role |
| `server/server_admin.go` | `adminLogin`, `adminSecurity`, `adminChangePW` | bootstrap forced-pw-change redirect; role field in change-pw; admin-count guard |
| `server/server_card_verify.go` | `verifyApiLogin` | returns token as today; session/loginlog written |
| `server/server_card_verify.go` | `getLoginRecord` | repoint to `loginlog` (full history) |
| `server/server_card_verify.go` | API routes for role change | new admin-only endpoint to change a user's role (with guard) |
| HTML / JS admin page | security page | add role management UI (admin only) |

No changes to `carddata.db` access, `verifySUN`, `verifyUID`, or the public
verification paths.

## 10. Open items / hazards reviewed

- [x] One `rec` only in old design → solved by per-user `users` bucket.
- [x] Username not a stored field before → now `username` is a value field,
      immutable; stable `userID` is the key.
- [x] Legacy `admin/123456` backdoor → disabled after first bootstrap login +
      forced password change.
- [x] "At least one admin" → guard on role change / delete.
- [x] Complete login history → `loginlog` bucket, appended every login.
- [x] Clients unchanged → username already sent; server enforces roles.
- [x] Existing data preserved → new `userdb.db`; old `rec`/`salt` left on disk.
- [x] **Role-management UI**: add a role selector (radio button or similar:
      `admin` / `operator`) on the existing admin security page
      (`/verify/admin/security`, `adminChangePW`, server_admin.go:204), alongside
      the current change-password fields. It is shown **only when the logged-in
      user is an `admin`**; hidden for `operator`. Admin-only endpoint applies
      the change with the ≥1-admin guard.
- [x] **Show/hide password toggle**: add an eye/show-hide icon next to every
      password input field (`Original password`, `New password`, `Repeat new
      password`) so the user can reveal or mask the entries. The existing GUI
      has no such control; adding it improves usability.
- [x] **`adminsession.db` iris cookie**: kept unchanged as the session-token
      transport. Negligible storage; harmless. No replacement needed.
- [x] **Session invalidation & rotation**:
      - **Logout**: `adminLogout` deletes the `sessions[token]` row
        (server-side invalidation) so the token is unusable immediately, in
        addition to destroying the iris cookie.
      - **Rotation on sensitive changes**: when a user's **password** or
        **role** changes, the current `sessions[token]` row is deleted and a
        **new token** is issued. This expires the old token immediately, so for
        example an admin demoted to `operator` loses access to advanced
        functions at once (the old token no longer resolves to `admin`).
      - Only newly-created/replaced session rows are affected; `users`,
        `loginlog`, `rec`/`salt`, and `carddata.db` are never touched.

## 12. Staged implementation task list (incremental testing)

Each stage is independently buildable and testable. We pause to test before
moving to the next stage. No stage modifies existing `rec`/`salt` or
`carddata.db` data.

### Stage 1 — New user database & core data layer (no auth change yet)
**Goal:** `userdb.db` opens with the three buckets and basic CRUD works.
- Add `CreateUserDB`/`makeDB` opening `local/userdb.db`.
- Add `UserAdd(userID, username, rec, salt, role)`.
- Add `UserGetByUsername(username)` (scan `users` bucket).
- Add `UserVerify(username, pw)` using `genPWHashWithSalt`.
- Add `CountAdmins`.
- **Test:** unit test / small helper: create a user, verify login, count admins.
  Existing `admin.db` login path still works unchanged.

### Stage 2 — Sessions & login log in the new DB
**Goal:** session + history written to `userdb.db`; old `admin-record` no longer
used for auth.
- Add `SessionCreate(token, userID, username, role, loginTime)`.
- Add `SessionGet(token)` → `{userID, username, role}`.
- Add `SessionDelete(token)`.
- Add `LoginLogAppend(...)` and `LoginLogList()`.
- **Test:** after a simulated login, confirm `sessions` + `loginlog` rows exist
  and `SessionGet` returns correct role.

### Stage 3 — Wire login to new user store (replace `CheckAdminLogin` usage) — DONE
**Goal:** `AdminIn` verifies against `users`; creates session + loginlog;
bootstrap `admin/123456` path.
- Update `AdminIn` (card_admin.go) to use `UserGetByUsername`+`UserVerify`
  (extracted to `adminLoginCore` for testability). On success: `UserLoginNew`
  (writes `sessions` + `loginlog`), sets iris cookie = token.
- Bootstrap: if `users` empty and legacy `cardDB.CheckAdminLogin` matches the
  default admin/123456 → seed the first `users` row (role=admin) computed from
  the provided password, then create session. After seeding, subsequent logins
  use the new store only (legacy fallback no longer triggered).
- `CardAdmin` gains `userDB` field + `MakeUserAdmin(dbpath)` (called in
  `MakeAdminPage`) + exported `AttachUserDB` (tests). `AdminIn` keeps its
  `int` return; a `lastToken` field + `LastLoginToken()` exposes the token to
  `verifyApiLogin`. `AdminCheck` now reads the session token from the cookie.
- `verifyApiLogin` returns the token from `LastLoginToken()` instead of
  `cardDB.OnLoginUser()`.
- `checkAdminVerify` resolves API tokens via `cardAdmin.UserSessionTimeout`
  (new store); role enforcement still deferred to Stage 4.
- **Test:** `TestStage3AdminLogin` (card_test package, HTTP) — operator logs in
  via new store (session + loginlog written); empty-store bootstrap with
  admin/123456 seeds first admin, legacy `CheckAdminLogin` called exactly once
  (proving fallback disabled after bootstrap). PASS.
- **Deferred (to Stage 5/6):** the *forced password-change redirect* after the
  bootstrap login. Stage 3 seeds the account and disables the legacy fallback,
  but the UI redirect that blocks the admin until they change the password is
  implemented in Stage 6 (security page) — noted so it is not forgotten.

### Stage 4 — Role-based authorization in `checkAdminVerify` — DONE
**Goal:** operator blocked from non-allowed endpoints; admin unchanged.
- Added `UserRoleOfToken(token)` and `UserRoleInCtx(ctx)` to `card.CardAdmin`
  (card_admin.go): resolves the account role from the API token / GUI session.
- Rewrote `checkAdminVerify` (server_admin.go):
  - API `X-token` path: still restricted to the whitelisted
    `/verify/api/{modellist,cardwrite,cardsearch}` routes (operator scope);
    invalid/expired token → `NotFound()`.
  - GUI cookie path: after `AdminCheck` passes, resolve the session role via
    `UserRoleInCtx`; operators are denied admin GUI pages (`NotFound()`),
    admins pass through. Operator data endpoints remain reachable via the API
    path above.
- **Test:** `TestStage4RoleAuthz` (server package, HTTP). Verifies:
  - admin cookie client → GUI page 200;
  - operator cookie client → GUI page 404;
  - admin/operator API token (X-token) on `/verify/api/cardwrite` → 200;
  - bogus API token → 404. PASS.
- Existing Stage 1/2/3 tests (`TestUserDBStage1`, `TestStage3AdminLogin`) still
  PASS (no regression).

### Stage 5 — Password & role change + admin-count guard + rotation
**Goal:** users change own pw; admin changes roles; rotation on change.
- `UserChangePW` (recompute `rec`/`salt`); `UserChangeRole` (admin-only, with
  `CountAdmins` guard).
- On pw/role change: delete old `sessions[token]`, issue new token.
- **Test:** operator changes own pw → old token expires; admin demotes self/
  other → advanced access lost immediately; cannot remove last admin.

### Stage 6 — Admin security page UI (role selector + show/hide toggle)
**Goal:** usable management UI.
- Add role radio (`admin`/`operator`) shown only for admins; hidden for
  operators.
- Add show/hide password toggle icon on all password fields.
- Admin-only endpoint applies role change.
- **Test:** admin sees + uses role selector; operator does not; toggle works.

### Stage 7 — Logout invalidation + login-history view
**Goal:** logout truly ends session; history view shows full log.
- `adminLogout` deletes `sessions[token]` (keep iris cookie destroy).
- Repoint `getLoginRecord` to `loginlog` (full history) + add view.
- **Test:** after logout, old token rejected; history lists all logins with
  user + time.

### Stage 8 — Final verification & acceptance checklist
- Run Section 11 checklist end to end.
- Confirm existing `admin.db` `rec`/`salt` + `carddata.db` untouched on disk.
- Confirm no client changes needed.

**Testing gates:** after Stages 1, 2, 3, 4, 5, 6, 7 we pause for a test pass
before continuing. Stage 8 is the final gate.

## 11. Acceptance checklist (target)

- [ ] Multiple admins and operators can each log in with their own password.
- [ ] Operator is blocked from all non-allowed endpoints (returns 404/denied).
- [ ] Admin can change any user's role; cannot remove the last admin.
- [ ] User can change own password; `rec`/`salt` recomputed immediately.
- [ ] First login with `admin/123456` forces password change and disables the
      default afterward.
- [ ] Login history shows every login with username + timestamp.
- [ ] Existing `admin.db` `rec`/`salt` and `carddata.db` are unchanged on disk.
- [ ] No client (Vue/desktop/mobile) requires modification.
