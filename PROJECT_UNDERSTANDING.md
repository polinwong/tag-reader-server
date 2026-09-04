# Project Understanding — NFC Tag Verification System

> Internal working document. **The code is the source of truth.** This file is a
> synthesis of reading the actual source in `tag-reader-server` (Go) and
> `tag-reader-desktop` (Python). The `.docx`/`.md` docs in the repos may contain
> legacy/revised logic kept for historical record and can disagree with code.

---

## 1. System Overview

Three components, three repos:

| Repo | Language | Role | Hardware |
|------|----------|------|----------|
| `tag-reader-server` | Go (Iris web framework, bbolt DB) | Backend NFC verification server | None (HTTP service) |
| `tag-reader-desktop` | Python (CLI) | Desktop provisioning/scanning app | **USB NFC reader required** (ACR122U / PN532) |
| `tag-reader-app` | (reference only, not analyzed in depth) | Android app | Phone NFC | 

**Goal of the system:** issue and verify **NTAG 424 DNA** NFC tags. Each physical
tag is cryptographically bound to a record in the server DB and to a "model"
(artefact) link.

Two verification paths exist on a tag:
1. **ECDSA signature over UID** — proves the tag is a genuine NXP NTAG 424 DNA
   (offline, using NXP's fixed root public key).
2. **SDM (Secure Dynamic Messaging)** — when tapped, the tag emits a dynamic URL
   containing (a) encrypted PICC data = `UID + read counter` and (b) a CMAC
   (`sdmmac`) over that data. The server recomputes the CMAC with the per-tag
   file key to prove the tap is authentic and untampered, and returns the linked
   model page.

`tag-reader-app` and `tag-reader-desktop` both talk to `tag-reader-server` over
the **same HTTP API**.

---

## 2. `tag-reader-server` (Go)

### 2.1 Startup / entry
- `main.go` → `server.ServerMain` → `server.newIrisApp()`. Binds Iris on
  `IP:PORT` (`server/server_main.go`); `:4430` (new `verifynfc.top`) or legacy
  `:443`/`:8080` (old `coinllectibles.art`).
- On startup it builds (in this order):
  - `card.MakeCardAdmin(...)` + `cardAdmin.MakeUserAdmin(dbPath)` — the GUI
    session store (`adminsession.db`, 7-day cookie) and the **user store**
    (`userdb.db`, see §2.3).
  - `card.MakeVerifyCard(dbPath)` — generates/loads `AppMasterKey` and
    `SdmMetaKey` (see §2.5), and hard-codes the NXP ECDSA public key.
  - `card.CreateCardDB(dbPath)` — opens `admin.db` (legacy credentials) and
    `carddata.db`.

### 2.2 Routes (client-facing)
The desktop and Android clients use exactly these endpoints:

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/verify/api` | none | Verify ECDSA signature (`s`=sig b64url, `u`=uid b64url). Returns `msg:"key verify"` + `result`. |
| POST | `/verify/sun` | none | Verify SDM tap data (`d`=SUN string). Returns `msg:"OK"` + `link`. |
| GET  | `/verify/api/linkmodel/:id` | none | Model name/img/desc for a linked artefact. |
| POST | `/verify/api/login` | none | Login (`inid`,`inpw`); returns `token` (also sets the GUI cookie). |
| GET  | `/verify/api/modellist?page=` | token | Paginated model list. |
| GET  | `/verify/api/cardsearch?id=` | token | Does this UID already exist? (`msg:"EXISTED"`). |
| POST | `/verify/api/cardwrite` | token | Register a tag: body `id`,`sign`,`link`. **Returns the three keys** `fkey`, `metakey`, `appmasterkey` (each 32 hex chars = 16-byte AES). |
| POST | `/verify/api/cardchecked`, `/cardpwupdate`, `/carddel`, `/cardlinkset` | token | Card status reset / file-key update / delete / re-link. |
| POST/GET | `/verify/api/modelwrite`, `/modelimgclean`, `/modelsearch` | token | Model CRUD and image cleanup. |

The **operator** role is limited to the three token routes `modellist`,
`cardwrite`, `cardsearch`; every other admin route is admin-only. Account and
role management, the admin GUI and the login-history view are documented in
`SERVER_REFERENCE.md` §2.2–2.3.

`checkHeader()` requires `X-Requested-With: com.mdl.arttagscanner` (the Android
package name) on `/verify/api/login` and on the CORS preflights; the desktop
client sends the same header. State-changing admin routes authenticate with the
session cookie or the `X-token` header instead.

### 2.3 Accounts, roles and sessions
A dedicated bbolt store `userdb.db` holds all accounts; `admin.db` is now only a
bootstrap/fallback source.

- **Roles** — `admin` (full access) and `operator` (the three token routes plus
  a self-service password page). The role lives in the session record, so it is
  enforced entirely server-side: **no client change was required**.
- **Accounts** — key `userID` (UUID), value `{username, rec, salt, role,
  mustChange}`; `rec = hex(HMAC-SHA256(salt, username+password))`, recomputed
  with a fresh salt on every password change. Usernames are immutable.
- **Sessions** — `sessions[token] = {userID, username, role, loginTime}`; the
  GUI cookie carries the token, the app sends it as `X-token`. 7-day expiry.
- **Login history** — `loginlog` is append-only (never deleted) and backs the
  admin "Login log" page and `/verify/api/loginrec`.
- **Bootstrap** — with an empty `users` bucket the legacy `admin`/`123456`
  default (or an existing `admin-hash`) is accepted once; the first account is
  seeded as `admin`, tagged in `meta` and flagged `mustChange`. Afterwards the
  legacy path is never consulted.
- **Guards** — `checkAdminVerify` (admin GUI, or a valid token on the operator
  whitelist) and `checkOperatorVerify` (any logged-in user). Denials are 404 on
  the GUI guard (route-existence hiding); in-handler role checks return 403.
- **Rotation** — a password or role change deletes every session of that
  account, so a demoted admin loses access immediately. Logout deletes the
  session server-side. Demoting the last admin is rejected.

### 2.4 SDM verification (`card/verify_base.go` — `VerifyPICCData`)
This is the cryptographic core for `/verify/sun`:
1. Decrypt PICC data with `SdmMetaKey` (AES-128-CBC, zero IV). LRP mode
   (24-byte PICC) is partially implemented via `lrp.go` but not fully tested.
2. Parse the config byte: expects `uidLength == 0x07` (7-byte UID). Otherwise
   returns `ErrVerifySunNotMatch` (fake SDMMAC calc added to avoid timing
   oracle).
3. Extract `uid` and `ctr` (read counter) from the mirrored bytes.
4. Look up the per-tag **file key** via `db.ReadCardFilekey(uid)`.
5. Recompute SDM MAC with `calculateSdmMac(fileKey, uid+ctr, lrp)` and compare
   to the supplied `sdmmac`. MAC uses `github.com/aead/cmac` over
   `[0x3C,0xC3,0x00,0x01,0x00,0x80] || piccData`, then a second CMAC over empty
   file data; final MAC = every odd byte (AN12196 / sdm-backend algorithm).
6. If MAC matches:
   - If `ctrReq > ctrSaved`: update stored CTR (`StatusCtrNormal`, or
     `StatusCtrJump` if gap > 2).
   - Else (`ctrReq <= ctrSaved`): `StatusCtrRepeated` — flag possible cloning /
     replay, but `LastErrorID` is set (uid returned only in debug).
7. Returns the `cardid` (uid) on success.

`VerifyPlainSUN` / `VerifyUID`/`VerifyUIDBase64` handle the simpler plaintext
SUN and the ECDSA UID-signature checks respectively.

### 2.5 Key material (`card/verify_base.go`, `card/card_db.go`)
Two **server-wide** keys (generated once, persisted to files, reused forever):
- `AppMasterKey` — loaded from `<dbPath>/appmasterkey` (16 random bytes, created
  if missing). This is **NTAG 424 DNA application key 0**, NOT a server
  login/session secret. It is the master key used to authorize tag
  administration — i.e. changing any of the tag's 5 keys (key 0/1/2...) and
  file/access settings. The **desktop app** uses it to authenticate to the
  *physical tag* (over the USB NFC reader) when provisioning or reconfiguring a
  tag; the server only stores it and returns it to the desktop in the
  `cardwrite` response so the desktop can perform the write. The server does
  NOT use `AppMasterKey` to authenticate the desktop app — client logins are
  verified against the account store `userdb.db` (§2.3).
- `SdmMetaKey` — loaded from `<dbPath>/metakey`. This is NTAG 424 DNA key 2
  (SDM Meta Read Key), used by the server to decrypt PICC data from a tapped
  tag during verification.

The **per-tag file key** (`fkey`) is a UUID-generated 16-byte value stored in the
DB record for each tag (`WriteCardData` → `fkey = hex(uuid)`). It is what the
server uses to verify that tag's SDM MAC, and what is written into the tag as
SDM File Read Key (key 1).

> Note: The repo also contains `ancientAuth/` and `source/` (legacy key
> derivation code, e.g. older password/hash schemes). Treat these as historical;
> the live path uses `loadOrCreateAESKey` + UUID fkey. Verify against current
> code before trusting any doc that references them.

### 2.6 Database (`card/card_db.go`, `card/card_user.go`)
bbolt key/value stores:
- `userdb.db` → buckets `users` (accounts), `sessions` (active tokens),
  `loginlog` (append-only history) and `meta` (bootstrap-account marker). This
  is the authoritative credential store — see §2.3.
- `admin.db` → buckets `admin-record` (legacy API sessions, no longer
  authoritative) and `admin-hash` (salt + hashed password). `CheckAdminLogin`
  falls back to a hard-coded default password hash (`pwsalt`/`password`
  constants in `card_admin.go`) when no salt record exists — now used **only**
  for the empty-userdb bootstrap.
- `carddata.db` → buckets:
  - `card-data` — key = 7-byte UID; value = JSON `{sign, ctr(base64), link,
    fkey(hex), status}`.
  - `link-data` — key = model UUID; value = JSON `{name, desc, image}`.

CTR status enum: `NORMAL` / `JUMP` / `REPEATED`. Deleting a model is blocked if
any card links to it (`ErrCardModelDelLinking`).

---

## 3. `tag-reader-desktop` (Python)

CLI menu app (`app.py` → `Menu`). Modes:
1. **Scanner** (`ui/scanner.py`) — read + verify tag, query server.
2. **Writer** (`ui/writer.py`) — admin: register tag on server, get keys, write
   them to the physical tag.
3. **ID Reader** (`ui/idreader.py`) — collect UIDs only (also supports HID
   readers with limited functionality).
4. Switch server (primary `verifynfc.top` ↔ legacy `nfc.coinllectibles.art`).
5. Reader info / beep test.

### 3.1 Server communication (`server/client.py`, `server/endpoints.py`)
`ServerClient` (requests.Session) calls exactly the endpoints in §2.2:
- `verify_signature` → POST `/verify/api` (`s`,`u`)
- `verify_sdm` → POST `/verify/sun` (`d`); includes `X-Requested-With` header,
  retries on transient 404.
- `login` → POST `/verify/api/login`
- `get_model_list`, `search_card`, `write_card` → token-protected endpoints.
- `write_card` returns `fkey`/`metakey`/`appmasterkey` which are then passed to
  the NFC writer.

### 3.2 NFC hardware read/write (`nfc/`)
- `reader.py` — `NFCReader` abstraction over a vendor C library (`library_loader`
  loads `lib/`; `hardware_detect` auto-detects ACR122U/PN532; `99-nfc-reader.rules`
  + `install_udev_rule.sh` for Linux USB perms). Provides `activate_card`,
  `get_version`, `read_signature`, `select_application`, `authenticate_ev2`,
  `change_key`, `change_file_settings`, `write_ndef`, `read_ndef`, `read_file_settings`.
- `ntag424.py` — `NTAG424DNA` high-level orchestration, mirroring the Android
  `NxpNfcPlugin.java`:
  - `read_tag()` — scanner flow: activate → GetVersion (byte[0]==0x04 ⇒ NXP) →
    read 56-byte ECDSA signature → **offline** ECDSA verify → read NDEF URI.
  - `write_tag(app_master_key_hex, file_key_hex, meta_key_hex, ...)` — writer
    flow (see §3.4).
- `sdm.py` — builds SDM file-settings bytes + NDEF URI with zero-padding
  placeholders that the tag fills at tap time.
- `ecdsa_verify.py` — offline ECDSA secp224r1 verify using NXP root key.
  **Critical detail:** NTAG 424 DNA signs the UID *without hashing* — the code
  uses `vk.verify_digest(uid_bytes.rjust(28,0), sig)` (no SHA). Same key is
  hard-coded in the server's `verify_base.go` (`PublicKeyHexRaw`, P-224).

### 3.3 Key constants (`keys/constants.py`)
- `ECDSA_PUBLIC_KEY_HEX` — NXP NTAG 424 DNA root public key (identical value in
  server `verify_base.go`). Not secret.
- `AES128_DEFAULT_KEY_HEX` = 32 zeros — factory default to authenticate new tags.
- `KEY_APP_MASTER=0`, `KEY_SDM_FILE=1`, `KEY_SDM_META=2` — the three application
  keys. `FILE_NDEF=2` holds the NDEF/SDM config.
- `DOMAIN_NEW = "192.168.12.64:4430/verify/sun?d="` (note: currently a LAN IP,
  not the public domain — likely a local/dev override), `DOMAIN_OLD =
  "nfc.coinllectibles.art/verify/sun?d="`.
- PICC/CMAC padding lengths: AES PICC=32 hex, CMAC=16 hex.

### 3.4 Writer flow (`nfc/ntag424.py::write_tag`) — how keys get onto a tag
Inputs come from the server `cardwrite` response. Three tag "shapes" handled:
1. **Current-format tag** — authenticate AppMaster(key 0) with server
   `appmasterkey` ⇒ proceed.
2. **Legacy two-key tag** — AppMaster auth fails, but old MetaKey authenticates
   key 0 ⇒ migrate: set key2=meta, key0=appmaster.
3. **Factory tag** — all keys zero ⇒ authenticate default key, then provision
   key2=meta, key1=file (`fkey`), key0=appmaster **last** (key 0 authorizes
   changes to 1 & 2).

Then (non-clear mode): build SDM config (`sdm.py`), `change_file_settings`,
`write_ndef`. SDM access rights written as `{0xFF,0x21}` ⇒ counter forbidden,
MetaRead=key2, FileRead=key1. The server verifies MAC with **key 1 (file key)**.

`clear` mode resets keys 2→1→0 back to factory default **before** writing SDM,
so the written tag uses default-key auth (used to wipe/reuse a tag).

> LRP mode and AES/LRP transition (`use_lrp`, `change_mode`) are explicitly
> **unsupported** by the desktop vendor library and will error out.

---

## 4. End-to-end: issuing a tag (Writer)
1. Desktop logs in (`/verify/api/login`) → token. Either role works here: the
   writer flow only touches `modellist`, `cardsearch` and `cardwrite`, which are
   exactly the operator-allowed routes.
2. Desktop optionally picks a model from `/verify/api/modellist`.
3. Desktop taps tag → reads UID + 56-byte signature.
4. Desktop POSTs `/verify/api/cardwrite` with `id=uid, sign=sig, link=modelId`.
5. Server creates/updates the DB record, generates `fkey` (UUID), and returns
   `fkey`, `metakey` (server `SdmMetaKey`), `appmasterkey` (server
   `AppMasterKey`).
6. Desktop writes the three keys + SDM config + NDEF URI onto the physical tag.

## 5. End-to-end: verifying a tap (Scanner / public)
1. User taps tag → phone/desktop reads dynamic URL
   `https://<domain>/verify/sun?d=<piccHex><cmacHex>`.
2. Client POSTs `d` to `/verify/sun`.
3. Server decrypts PICC with `SdmMetaKey`, looks up `fkey` by UID, recomputes
   CMAC, validates, updates CTR, returns `link` to the artefact page. If CTR is
   repeated ⇒ possible clone.

---

## 6. Caveats / things to verify before trusting
- **Docs may be stale.** `docs/` (server) and `*.md` (desktop) sometimes reflect
  older logic (e.g. legacy two-key layout, old domains). Always confirm against
  the Go/Python source above.
- `DOMAIN_NEW` in `keys/constants.py` points to a LAN IP (`192.168.12.64:4430`),
  not `verifynfc.top` — likely a local dev build. Check before assuming prod.
- `local/` (server) appears to be a standalone Node/JS local testing UI with its
  own `.db` files — separate from the main Go binary; use only for local dev.
- LRP mode is incomplete in the server and unsupported on the desktop.
- **The `admin`/`123456` default** is only accepted to seed the first account
  from an empty `userdb.db`; that account is flagged `mustChange` and shows a
  banner until the password is changed. Confirm this was done on every deploy.
- Role enforcement lives entirely in the server (`checkAdminVerify` /
  `checkOperatorVerify`). Clients are unchanged — they still send only
  username + password, so a compromised client cannot grant itself a role.
- The GUI is unusable if `userdb.db` cannot be opened (the operator guard needs
  a resolvable session). Treat that as an outage, not as "legacy mode".

## 7. File map (quick reference)
**Server (Go)** — see `SERVER_REFERENCE.md` for the full route/DB reference:
- `main.go` — entry
- `server/server_main.go`, `server_card_verify.go`, `server_admin.go`,
  `server_search.go` — routes and guards
- `card/verify_base.go` — SDM MAC + ECDSA verify, key loading
- `card/card_db.go` — bbolt card/model CRUD + legacy `admin.db` credentials
- `card/card_user.go` — `userdb.db`: accounts, sessions, login log, roles
- `card/card_admin.go` — login/session orchestration, role + password changes
- `card/lrp.go` — LRP (partial)
- `local/html`, `local/js/admin` — admin UI (security, login log, card verify)
- `ancientAuth/`, `source/` — legacy key code (historical)
- `docs/*.md`, `*.md` — possibly stale docs

**Desktop (Python):**
- `app.py`, `config.py`, `logging_config.py` — entry/config
- `nfc/ntag424.py` — high-level read/write (mirrors Android)
- `nfc/sdm.py` — SDM config + NDEF URI builder
- `nfc/reader.py`, `library_loader.py`, `hardware_detect.py`, `hid_reader.py`
- `crypto/ecdsa_verify.py` — offline signature verify
- `keys/constants.py` — all key/domain/offset constants
- `server/client.py`, `server/endpoints.py` — HTTP client to backend
- `ui/scanner.py`, `ui/writer.py`, `ui/idreader.py` — CLI modes
- `tests/` (21 files), `debug_script/` — tests & debug utilities
- `run_app.sh`, `setup_library.sh`, `install_udev_rule.sh`, `99-nfc-reader.rules`
  — run/setup scripts (Linux NFC reader support)
- `*.md` — possibly stale docs
