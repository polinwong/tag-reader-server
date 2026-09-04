# NFC Tag Reader Server — Reference

> Derived from the source in this repository (binary version `0.3.2`, plus the
> user-roles work in `USER_ROLES_IMPLEMENTATION_PLAN.md`). **The code is the
> source of truth** — if this file disagrees with it, the code wins.
>
> Scope of this document: the Go server only. For the cross-repo view (desktop
> client, NFC key model, tag issuing flow) see `PROJECT_UNDERSTANDING.md`.

---

## 1. Overview

**Artefact Tag Authenticate Server** — a Go/Iris web service that verifies
NFC **NTAG 424 DNA** tags for art/artefact authentication.

- NFC tag cryptographic verification (AES-CMAC and LRP)
- ECDSA (P-224) signature verification of card UIDs
- Card and model (artefact) records in an embedded bbolt database
- **Multi-user accounts with two roles** (`admin`, `operator`) — §2.3
- Admin web UI + API for cards, models, accounts and login history
- Public verification page (Vue 3 + Quasar)
- `ancientAuth/` — deprecated histogram-based image authentication module

**Tech stack:** Go 1.17 · Iris v12 · bbolt (embedded K/V) · Vue 3 (UMD) +
Quasar + vue-i18n (user UI) · Iris HTML templates + Bootstrap 4.5.3 +
jQuery 3.5.1 (admin UI) · urfave/cli v2

---

## 2. Server Functions

### 2.1 Startup and configuration

Entry point `main.go` → `server.ServerMain` (`server/server_main.go`).

**CLI flags** (`main.go`):

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | uint | 4430 | HTTP(S) listen port |
| `--https` | bool | false | Enable HTTPS mode |
| `--cert` / `--key` | string | `server.crt` / `server-key.pem` | TLS material |
| `--ip` | string | (all) | Bind address |
| `--domain` | string | (empty) | Deprecated |
| `--index` | bool | false | Redirect `/` → `/verify` |
| `--debug` | bool | false | Use the all-zero test key for verification |

**Startup sequence** (`newIrisApp`):

1. `MakeAdminPage(app)` — creates `card.CardAdmin`, opens the **user store**
   (`cardAdmin.MakeUserAdmin(dbPath)` → `local/userdb.db`), registers admin &
   operator routes, HTML templates, Iris session DB.
2. `MakeCardPage(app)` — registers verification/card/model routes, then
   `card.MakeVerifyCard(dbPath)` (loads/generates `metakey` + `appmasterkey`)
   and `card.CreateCardDB(dbPath)` (opens `admin.db` + `carddata.db`), and
   creates the image directory.
3. `ancientauth.MakeAncientAuthServer(app)` — deprecated routes.

`MakeAdminPage` runs **before** `MakeCardPage`, so the user store is available
to every guard.

**Image directory** — `<exe-dir>/source/img`, overridable with the `IMG_DIR`
env var. Resolving it relative to the executable keeps the server independent of
the working directory (systemd/Docker safe).

### 2.2 Routes

Guards: **A** = `checkAdminVerify` (admin-only GUI, or token on the operator API
whitelist), **O** = `checkOperatorVerify` (any logged-in user), **–** = public.

**Public / client**

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET/POST | `/verify/sun` | `verifySUN` | NFC verification page (GET) and SDM API (POST: `picc_data`, `cmac`, `d`) |
| POST | `/verify/api` | `verifyUID` | ECDSA UID signature verification (`s`, `u`) |
| GET | `/verify/api/linkmodel/{id}` | `getLinkModelDetail` | Model/artefact detail by ID |
| GET | `/verify` | `LoginPage` | Login page; redirects if already authenticated |
| POST | `/verify/admin/login` | `adminLogin` | GUI login (`inid`, `inpw`) |
| POST | `/verify/api/login` | `verifyApiLogin` | App login; requires `X-Requested-With`; returns `{"token": …}` |

**Admin-only (role `admin`) — guard A**

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/verify/admin` | `verifyAdmin` | Card verify admin page |
| GET | `/verify/admin/security` | `adminSecurity` | Security page (pw change + role/user management) |
| POST | `/verify/admin/changepw` | `adminChangePW` | Change own password |
| POST | `/verify/admin/changerole` | `adminChangeRole` | Change another user's role (`userid`, `role`) |
| GET | `/verify/admin/userlist` | `adminUserList` | Account list for the management UI. Serves the `UserListView` projection — `rec`/`salt` never leave the server |
| POST | `/verify/admin/createuser` | `adminCreateUser` | Create account (`username`, `newpw`, `role`) |
| POST | `/verify/admin/resetpw` | `adminResetPW` | Reset another user's password (`userid`, `newpw`) |
| POST | `/verify/admin/logout` | `adminLogout` | Logout |
| GET | `/verify/admin/loginlog` | `adminLoginLog` | Login history page |
| GET | `/verify/api/loginrec` | `getLoginRecord` | Login history JSON (append-only) |
| DELETE | `/verify/api/logindel/{id}` | `removeLoginRecord` | **No-op** — history is permanent |
| GET | `/verify/api/carddata` | `cardData` | Paginated card list |
| POST | `/verify/api/cardwrite` | `cardWrite` | Create/update a card record |
| POST | `/verify/api/cardchecked` | `cardEdit` | Reset card status to `NORMAL` |
| POST | `/verify/api/cardpwupdate` | `cardEdit` | Update card file key |
| POST | `/verify/api/carddel` | `cardEdit` | Delete a card record |
| POST | `/verify/api/cardlinkset` | `cardEdit` | Set a card's linked model |
| GET | `/verify/api/cardsearch` | `cardSearch` | Search cards by UID hex or base64URL UID |
| GET | `/verify/api/modellist` | `modelList` | Paginated model list |
| POST | `/verify/api/modelwrite` | `modelWrite` | Create/update/delete a model |
| POST | `/verify/api/modelimgclean` | `modelImgClean` | Remove unreferenced images |
| GET | `/verify/api/modelsearch` | `modelSearch` | Search models by name |

**Self-service (any logged-in user — guard O)**

Unified routes exist so operators never POST to the admin-only URLs (which 404
for them). The role-specific aliases are kept for compatibility.

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/verify/operator/security` | `adminSecurity` | Operator security page (no role section) |
| POST | `/verify/changepw` | `adminChangePW` | Change own password (used by both roles) |
| POST | `/verify/operator/changepw` | `adminChangePW` | Alias |
| POST | `/verify/logout` | `adminLogout` | Logout (used by header “Sign Out”) |
| POST | `/verify/operator/logout` | `adminLogout` | Alias |
| GET | `/js/admin/*` | static | Admin JS (served `Cache-Control: no-store`) |

**CORS preflight (OPTIONS):** `/verify/sun`, `/verify/api/cardwrite`,
`/verify/api/cardsearch`, `/verify/api/modellist` — each requires
`X-Requested-With: com.mdl.arttagscanner`.

**Deprecated (`ancientAuth/`)**

| Method | Path | Handler |
|--------|------|---------|
| GET | `/auth` | `authBasic` |
| POST | `/auth/scan` | `authScan` |
| POST | `/auth/api/chk` | `authCheck` |
| POST | `/auth/api/ref` | `authRef` |
| POST | `/auth/api/submit` | `authSubmit` |

### 2.3 Authentication, roles and sessions

Two credential paths feed the **same** session store (`userdb.db`):

- **GUI** — Iris cookie session (`adminsession.db`, cookie `VerifyServer`,
  7 days). The session value `token` holds the user-store session token;
  `AdminCheck` resolves it through the `sessions` bucket.
- **API / app** — `POST /verify/api/login` returns the token; clients send it in
  the `X-token` header. Same 7-day expiry (`API_SESSION_TIME`).

Roles: `admin` (everything) and `operator` (limited). The role is part of the
session record, so it is enforced server-side; **no client change is needed**.

**Authorization matrix**

| Requester | GUI pages | API routes |
|-----------|-----------|------------|
| `admin` | admin set **+** self-service set | all of the admin set |
| `operator` | only the self-service set | only `/verify/api/modellist`, `/verify/api/cardwrite`, `/verify/api/cardsearch` |
| anonymous / invalid | 404 | 404 |

`checkAdminVerify` (server_admin.go):
- `X-token` present → path must be in the three-route operator whitelist,
  otherwise 404; then the token is resolved in the user store.
- otherwise → GUI cookie path; `CurrentUserFull` must resolve **and**
  `role == admin`.
- Denials are `NotFound()` (404) to avoid disclosing route existence.
  Role checks *inside* handlers (`adminChangeRole`, `adminCreateUser`,
  `adminResetPW`, `adminUserList`) return **403**.

**Login flow** (`CardAdmin.AdminIn` → `adminLoginCore`):

1. Look up the account by username in the `users` bucket and verify
   `rec = hex(HMAC-SHA256(key=salt, msg=username+password))`.
2. **Bootstrap**: if the `users` bucket is empty, fall back to the legacy
   `CardDatabase.CheckAdminLogin` (i.e. `admin.db` `admin-hash`, or the
   hardcoded `admin`/`123456` default). On success the first account is seeded
   with `role=admin`, tagged as the bootstrap user in `meta`, and flagged
   `mustChange`. After seeding, all later logins use the user store, so the
   default credentials stop working.
3. On success: `UserLoginNew` writes `sessions[token]` **and** appends
   `loginlog[token]`; the token is stored in the Iris session (GUI) or returned
   (API).
4. Redirect: operators → `/verify/operator/security`, admins → `/verify/admin`.

If `userdb.db` cannot be opened, `CardAdmin.userDB` stays `nil` and the legacy
`admin.db` path is used; in that mode the GUI is effectively unavailable
(`checkOperatorVerify` requires a resolvable session) — treat an unopenable user
store as an outage.

**Session lifecycle**

- **Logout** (`adminLogout` → `LogoutCurrentUser`): deletes `sessions[token]`
  server-side (token dies for both GUI and API) and destroys the cookie.
- **Rotation**: changing a password or a role deletes every session of that
  account (`SessionDeleteForUser`), so a demoted admin loses access immediately.
- **Login history** is append-only: `loginlog` rows are never deleted, which is
  why `/verify/api/logindel/{id}` is a no-op.

**Account management** (all admin-only, §2.2)

| Action | Effect |
|--------|--------|
| `createuser` | New account (`username`, initial password, role), duplicate usernames rejected, flagged `mustChange` |
| `resetpw` | Admin sets a password without knowing the old one; target flagged `mustChange` and rotated |
| `changerole` | Admin promotes/demotes; **demotion is refused when it would leave zero admins** (`ErrCardAdminFail`) |
| `changepw` (self) | Requires the current password; new salt + `rec`, clears `mustChange`, rotates sessions |

There is no account deletion; an account is disabled by resetting its password.

**Login failure logging** — GUI failures log
`Login failed for user "<name>" from <addr>`; the API path logs
`Login failed: <IP>` (the fail2ban-compatible format, see `README.md`).

### 2.4 Card and model management

**Cards** (`carddata.db` → `card-data`, key = 7-byte UID)

- `cardWrite` — create or update. Requires a valid ECDSA signature
  (`VerifyUIDBase64`); UID (7 bytes) and signature (56 bytes) are base64URL. A
  UUID `filekey` is generated for new cards. Response carries `fkey`, `metakey`
  and `appmasterkey` (hex) for tag provisioning.
- `cardEdit` — one handler for `cardchecked` (status → `NORMAL`),
  `cardpwupdate` (new `filekey`, 32 hex chars), `carddel` (requires the stored
  `filekey` as `pw`) and `cardlinkset` (set `link`).
- `cardSearch` — by UID hex (`key`) or base64URL UID (`id`, returns
  `EXISTED`/`NOTEXIST`). Capped at 50 results.

**Models** (`carddata.db` → `link-data`, key = UUID)

- `modelList` / `modelSearch` — paginated (50/page) / case-insensitive name
  search (max 50).
- `modelWrite` — create, update, or delete (`modelDel=1`). Deletion fails while
  any card links to the model (`ErrCardModelDelLinking`). Uploaded images are
  stored as `<sha256>` under the image directory.
- `modelImgClean` — deletes image files no longer referenced by any model.

---

## 3. NFC Verification Process

### 3.1 Overview

Core logic in `card/verify_base.go`. Two tag encryption modes are supported:
**AES** (standard) and **LRP** (Leakage Resilient Primitive, `card/lrp.go`).

### 3.2 SDM / SUN verification — `POST /verify/sun`

Request carries either `picc_data` + `cmac`, or the packed `d` parameter
(48 hex = AES: picc 32 + cmac 16; 64 hex = LRP: picc 48 + cmac 16).

`VerifyPICCData(bPicc, bCmac, cardDB, debug)`:

1. **Mode** — 16-byte PICC ⇒ AES, 24-byte PICC ⇒ LRP.
2. **Decrypt PICC** with `SdmMetaKey`: AES-128-CBC with a zero IV, or LRP with
   `piccRand = piccEncData[0:8]`.
3. **Parse** (AN12196 §4.3): config byte — bit 7 `uidMirrored`, bit 6
   `sdmReadCtrMirrored`, bits 0–3 `uidLength` (must be `0x07`); then the 7-byte
   UID and the 3-byte read counter when mirrored.
4. **Lookup** `ReadCardFilekey(uid)` → `filekey`, saved CTR.
5. **Recompute the SDM MAC** (`calculateSdmMac(filekey, uid+ctr, lrp)`):
   - AES: `CMAC` over `[0x3C,0xC3,0x00,0x01,0x00,0x80] || piccData`, then a
     second CMAC over an empty buffer; take the even-indexed bytes.
   - LRP: buffer `[0x00,0x01,0x00,0x80] || piccData || [0x1E,0xE1]`; master key
     = `LRP(filekey).cmac(buffer)`, session MACing key = `LRP(masterKey)`,
     digest = `sessionKey.cmac([])`; take the even-indexed bytes.
6. **Compare** with the received CMAC — mismatch ⇒ `ErrVerifySunNotMatch`.
7. **CTR check**
   - `req > saved`: delta > 2 ⇒ status `JUMP`, else `NORMAL`; CTR is updated.
   - `req <= saved`: status `REPEATED` (replay), `ErrVerifySunCtrRepeated`.

On success the handler resolves the linked model (`ReadCardData`) and returns
`{"msg":"OK","info":"data valid","link":"<model_id>"}`; on failure
`{"msg":"FAIL","info":"data invalid"}`. `GET` renders `verify-page.html`.

### 3.3 UID signature verification — `POST /verify/api`

`VerifyUIDBase64(sign, uid)` → base64URL-decode → `VerifyUID`: split the 56-byte
signature into `r = sign[0:28]`, `s = sign[28:56]` and verify with the hardcoded
P-224 public key (the NXP NTAG 424 DNA root key). The UID is signed **without
hashing**.

### 3.4 Key material

| Key | Location | Purpose | Size |
|-----|----------|---------|------|
| `SdmMetaKey` | `local/metakey` | Decrypts PICC data (NTAG key 2) | 16 bytes |
| `AppMasterKey` | `local/appmasterkey` | NTAG application key 0; authorises tag administration | 16 bytes |
| `filekey` | `carddata.db`, per card | SDM MAC key, unique per tag (NTAG key 1) | 16 bytes (hex UUID) |
| ECDSA public key | hardcoded in `verify_base.go` | Verifies UID signatures | P-224 |
| User password hash | `userdb.db` → `users` | `rec` = HMAC-SHA256(salt, username+password) | 32 bytes hex |
| Debug key | hardcoded (all zeros) | Used with `--debug` | 16 bytes |

### 3.5 Card status values

| Status | Meaning |
|--------|---------|
| `NORMAL` | CTR advanced normally (delta ≤ 2) |
| `JUMP` | CTR jumped by more than 2 (possible tampering) |
| `REPEATED` | CTR replayed (`req <= saved`) |

`REPEATED` is never overwritten by the CTR update logic — only the admin
`cardchecked` action resets it to `NORMAL`.

---

## 4. Database Schema

bbolt (embedded key/value). There is no SQL schema; the layout is defined in Go.

### 4.1 Database files

| File | Opened by | Contents |
|------|-----------|----------|
| `userdb.db` | `card.CreateUserDB` (via `CardAdmin.MakeUserAdmin`) | Accounts, active sessions, login history, metadata — **the authoritative credential store** |
| `carddata.db` | `card.CreateCardDB` | `card-data`, `link-data` |
| `admin.db` | `card.CreateCardDB` | **Legacy**: `admin-hash` (credentials, bootstrap/ fallback only) and `admin-record` (legacy API sessions) |
| `adminsession.db` | `MakeCardAdmin` | Iris GUI session storage (`irissession` bucket) |
| `local/metakey`, `local/appmasterkey` | `MakeVerifyCard` | 16-byte AES keys (plain files, not bbolt) |

`local/` is the default `dbPath`; it also holds the HTML/JS/CSS assets.

### 4.2 `userdb.db`

**`users`** — key: userID (UUID string; the bootstrap account uses
`userID == username`). Value:

```json
{ "id": "...", "username": "...", "rec": "<hex>", "salt": "<uuid>",
  "role": "admin|operator", "mustChange": false }
```

- `username` is immutable after creation.
- `rec = hex(HMAC-SHA256(key=salt, msg=username+password))`, recomputed (with a
  fresh UUID salt) on every password change.
- Lookup is a bucket scan (`UserGetByUsername`) — the account count is small.

**`sessions`** — key: token (UUID). Value:
`{"userID","username","role","loginTime"}`. Expiry = `API_SESSION_TIME` (7 days).

**`loginlog`** — key: loginID (the login's token UUID). Value:
`{"loginID","userID","username","role","loginTime"}`. Appended on every
successful login, never deleted.

**`meta`** — `bootstrap-user` → userID of the account created from an empty
`users` bucket. The security page shows the “change the default password”
banner only for that account. Databases predating this bucket are backfilled
(`backfillBootstrapUser`) using the heuristic `id == username`.

### 4.3 `carddata.db`

**`card-data`** — key: 7-byte UID. Value:
`{"sign","ctr","link","filekey","status"}` (`sign`/`ctr` base64, `filekey` hex).

**`link-data`** — key: model UUID. Value: `{"name","desc","image"}`, where
`image` is the SHA-256 filename of the image in the image directory.

### 4.4 `admin.db` (legacy)

| Bucket | Key → Value | Status |
|--------|-------------|--------|
| `admin-hash` | `rec` → HMAC-SHA256 hex, `salt` → UUID | Read only for bootstrap / when `userdb.db` is unavailable. Never written by the roles code. |
| `admin-record` | session UUID → 8-byte varint timestamp | No longer authoritative; still written by the `AppSession` periodic sync. |

`CheckAdminLogin` uses `admin-hash` when `rec` exists, otherwise the hardcoded
default (`admin` / `123456`).

### 4.5 Relationships and file layout

```
link-data (model) 1:N  card-data (card)
    card: sign (ECDSA) · filekey (SDM MAC key) · ctr (anti-replay) · link · status

Verification needs: SdmMetaKey (decrypt PICC) + the card's filekey (MAC) + ctr.
Accounts live in userdb.db, entirely separate from card/model data.
```

| Path | Description |
|------|-------------|
| `local/metakey`, `local/appmasterkey` | 16-byte AES keys, auto-generated on first start (`0640`) |
| `<exe-dir>/source/img/<sha256>` | Model images (`IMG_DIR` overrides) |
| `config/` | Runtime logs (gitignored) |

---

## 5. Source Layout

```
tag-reader-server/
|-- main.go                      # CLI app + flags
|-- go.mod / go.sum
|-- build-bin.sh                 # Cross-compile build script
|-- nfc-server-ip.service        # systemd unit
|
|-- card/                        # NFC verification & database logic
|   |-- verify_base.go           # SUN/SDM verification, SDM MAC, ECDSA UID verify, key loading
|   |-- lrp.go                   # LRP (AN12304)
|   |-- card_db.go               # carddata.db + legacy admin.db (CRUD, CheckAdminLogin)
|   |-- card_user.go             # userdb.db: users / sessions / loginlog / meta, roles
|   |-- card_admin.go            # Login orchestration, session resolution, role/pw change
|   |-- db_types.go              # IDB/ITx/IBucket/Clock/UUID interfaces
|   |-- mock/                    # Generated GoMock mocks
|
|-- server/                      # HTTP handlers & routes
|   |-- server_main.go           # Init, CLI parsing, Iris app
|   |-- server_admin.go          # Admin/operator routes, checkAdminVerify, checkOperatorVerify, AppSession
|   |-- server_card_verify.go    # SUN verify, card/model CRUD, login history
|   |-- server_search.go         # Model & card search
|
|-- ancientAuth/                 # DEPRECATED histogram image auth
|-- local/                       # Served assets + default dbPath
|   |-- html/                    # page-layout, admin-header, admin-login,
|   |                            # admin-security, admin-loginlog, card-verify, verifysrc/
|   |-- js/admin/                # admin-common, admin-security, admin-loginlog, card-verify, user-common
|-- docs/                        # Architecture / function notes
|-- climgt-remote/               # Git submodule (CLI helper, not used in local builds)
```

Role-related tests: `card/card_user*_test.go`,
`server/server_role_stage{4,5,6,65,7}_test.go`.

---

## 6. Security Considerations

- **Default credentials** — `admin` / `123456` is accepted **only** to bootstrap
  the first account from an empty `users` bucket. That account is flagged
  `mustChange` and shows a banner until the password is changed; after seeding,
  the legacy path is no longer consulted. Verify this on every fresh deploy.
- **At least one admin** — role changes that would leave zero admins are
  rejected (`ErrCardAdminFail`). There is no account deletion.
- **Session rotation / invalidation** — password and role changes delete all of
  the account's sessions; logout deletes the session server-side, so a stolen
  token stops working for both GUI and API.
- **Least privilege** — operators reach only three API routes plus their own
  security page; the admin GUI is closed to them (404, not 403, for the GUI
  guard).
- **Login history** — `loginlog` is append-only and permanent; it cannot be
  trimmed through the API.
- **Password material stays server-side** — `UserList()` returns the
  `UserListView` projection (`id`/`username`/`role`/`mustChange`), so
  `/verify/admin/userlist` never serialises the `rec`/`salt` fields.
- **No GET submission of secrets** — the admin login form and all four security
  forms submit with `method="post"`, and the security-page scripts intercept
  the form *submit* event (covering both the button click and the Enter key),
  so passwords can never end up in the address bar as a query string.
- **`--debug`** — uses an all-zero verification key. Never in production.
- **`local/metakey`** — compromise allows decrypting tapped tag data (`0640`).
- **CTR replay protection** — repeated counters are flagged `REPEATED` and
  never auto-cleared.
- **fail2ban** — API login failures are logged as `Login failed: <IP>`.
- **CORS** — cross-origin calls require
  `X-Requested-With: com.mdl.arttagscanner`; `Access-Control-Allow-Origin`
  echoes the request origin with `Allow-Credentials: true`.
- **Data separation** — the roles work only adds `userdb.db`; `admin.db`
  (`rec`/`salt`) and `carddata.db` are never rewritten by it.

---

## 7. References

- [AN12196 — NTAG 424 DNA features and hints](https://www.nxp.com/docs/en/application-note/AN12196.pdf)
- [AN12304 — LRP specification](https://www.nxp.com/docs/en/application-note/AN12304.pdf)
- [sdm-backend](https://github.com/icedevml/sdm-backend) — SDM reference implementation
- [bbolt](https://github.com/etcd-io/bbolt) · [Iris](https://github.com/kataras/iris)
