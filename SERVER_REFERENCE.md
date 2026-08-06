# NFC Tag Reader Server - Reference Documentation

> Based on source code analysis (v0.3.2). This document is derived from the actual source code and may differ from other documentation files in the project.

---

## 1. Project Overview

The **Artefact Tag Authenticate Server** is a Go web application that verifies NFC NTAG 424 DNA tags for art/artefact authentication. It provides:

- NFC tag cryptographic verification (AES-CMAC and LRP encryption)
- ECDSA signature verification for card UIDs
- Card and model (artefact) record management via an embedded database
- Admin web UI and API endpoints for managing cards, models, and sessions
- A user-facing verification page (Vue 3 + Quasar) for scanning NFC tags
- A deprecated ancient artefact authentication module (histogram-based image comparison)

**Tech Stack:**
- Language: Go 1.17
- Web Framework: Iris v12
- Database: bbolt (embedded key-value store)
- Frontend (User): Vue 3 (UMD) + Quasar + vue-i18n
- Frontend (Admin): Iris HTML templates + Bootstrap 4.5.3 + jQuery 3.5.1
- CLI: urfave/cli v2

---

## 2. Server Functions

### 2.1 Startup and Configuration

The server entry point is `main.go`. It creates a CLI application via `climgt` and invokes `server.ServerMain`.

**CLI flags** (defined in `main.go`):

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--port` | uint | 4430 | HTTP(S) listen port |
| `--https` | bool | false | Enable HTTPS mode |
| `--cert` | string | `server.crt` | TLS certificate file path |
| `--key` | string | `server-key.pem` | TLS private key file path |
| `--ip` | string | (all interfaces) | IP address to bind |
| `--domain` | string | (empty) | Domain name for public access (deprecated) |
| `--index` | bool | false | Enable root `/` redirect to `/verify` |
| `--debug` | bool | false | Use test key (all-zero 16-byte key) for verification |

**Startup sequence** (`server/server_main.go`):
1. Parse CLI flags via `checkCli()`
2. Create Iris web application via `newIrisApp()`
3. `newIrisApp()` calls three setup functions in order:
   - `MakeAdminPage(app)` - registers admin routes, session manager, HTML templates
   - `MakeCardPage(app)` - registers card verify routes, initializes `VerifyCard` and `CardDatabase`
   - `ancientauth.MakeAncientAuthServer(app)` - registers deprecated auth routes
4. Start HTTP(S) server on the configured port
VerifyCard
**Key initialization in `MakeCardPage`**:
- `card.MakeVerifyCard(dbPath)` - creates `VerifyCard` struct, generates/loads the 16-byte `SdmMetaKey` from `local/metakey`
- `card.CreateCardDB(dbPath)` - opens `admin.db` and `carddata.db`, creates required buckets
- Creates the `source/img` directory for model image storage. The path is resolved relative to the **executable's directory** (i.e. `<exe-dir>/source/img`); an explicit directory may be set via the `IMG_DIR` environment variable. This makes image storage independent of the current working directory (safe for systemd/Docker deployment).

### 2.2 API Routes

All routes are registered in `server/server_admin.go` and `server/server_card_verify.go`.

#### User-Facing Routes

| Method | Path | Handler | Auth | Description |
|--------|------|---------|------|-------------|
| GET | `/verify` | `LoginPage` | None | Admin login page (redirects to `/verify/admin` if already logged in) |
| GET | `/verify/sun` | `verifySUN` | None | NFC verification page (renders Vue 3 UI) |
| POST | `/verify/sun` | `verifySUN` | None | NFC verification API (accepts `picc_data`, `cmac`, `d` params) |
| POST | `/verify/api` | `verifyUID` | None | ECDSA UID signature verification (accepts `s`, `u` params) |
| POST | `/verify/api/login` | `verifyApiLogin` | Cookie | API login for mobile app (returns token) |
| GET | `/verify/api/linkmodel/{id}` | `getLinkModelDetail` | None | Get model/artefact details by ID |

#### Admin Routes (Cookie or Token Auth)

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/verify/admin` | `verifyAdmin` | Card verify admin page |
| GET | `/verify/admin/security` | `adminSecurity` | Password change page |
| POST | `/verify/admin/changepw` | `adminChangePW` | Change admin password |
| POST | `/verify/admin/logout` | `adminLogout` | Logout admin session |
| POST | `/verify/admin/login` | `adminLogin` | Login admin (form POST) |
| GET | `/verify/api/loginrec` | `getLoginRecord` | List API login sessions |
| DELETE | `/verify/api/logindel/{id}` | `removeLoginRecord` | Remove API login session |
| GET | `/verify/api/carddata` | `cardData` | List card records (paginated) |
| POST | `/verify/api/cardwrite` | `cardWrite` | Create or update a card record |
| POST | `/verify/api/cardchecked` | `cardEdit` | Reset card status to NORMAL |
| POST | `/verify/api/cardpwupdate` | `cardEdit` | Update card file key |
| POST | `/verify/api/carddel` | `cardEdit` | Delete a card record |
| GET | `/verify/api/cardsearch` | `cardSearch` | Search cards by UID or keyword |
| GET | `/verify/api/modellist` | `modelList` | List models (paginated) |
| POST | `/verify/api/modelimgclean` | `modelImgClean` | Remove unused model images |
| POST | `/verify/api/modelwrite` | `modelWrite` | Create or update a model |
| GET | `/verify/api/modelsearch` | `modelSearch` | Search models by name |

#### CORS Preflight Routes

| Method | Path | Description |
|--------|------|-------------|
| OPTIONS | `/verify/sun` | CORS preflight |
| OPTIONS | `/verify/api/cardwrite` | CORS preflight |
| OPTIONS | `/verify/api/cardsearch` | CORS preflight |
| OPTIONS | `/verify/api/modellist` | CORS preflight |

#### Deprecated Ancient Auth Routes

| Method | Path | Handler | Description |
|--------|------|---------|-------------|
| GET | `/auth` | `authBasic` | Auth main page |
| POST | `/auth/scan` | `authScan` | Scan page |
| POST | `/auth/api/chk` | `authCheck` | Check artefact ID |
| POST | `/auth/api/ref` | `authRef` | Get reference images |
| POST | `/auth/api/submit` | `authSubmit` | Submit scan for histogram comparison |

### 2.3 Authentication and Authorization

The server uses two authentication mechanisms:

#### Cookie-based Session (Web Admin)
- Managed by `card.CardAdmin` (`card/card_admin.go`)
- Uses Iris `sessions` package with bbolt-backed session storage (`adminsession.db`)
- Session cookie name: `VerifyServer`
- Session expiry: 7 days
- Login credentials verified via `CardDatabase.CheckAdminLogin()`
- Password hashing: HMAC-SHA256 with salt
- Default credentials: hardcoded salt (`2f6a4c16e84e`) and password hash in `card_admin.go`
- After password change, a new UUID salt is generated and stored in `admin-hash` bucket

#### Token-based API Auth (Mobile App)
- Managed by `AppSession` (`server/server_admin.go`) and `CardDatabase` (`card/card_db.go`)
- On API login (`/verify/api/login`), a UUID token is generated and stored in the `admin-record` bucket with a timestamp
- Token is sent via `X-token` header
- Token validity: 7 days (`API_SESSION_TIME`)
- `AppSession` maintains an in-memory cache of active tokens, periodically synced to the database (every 30 minutes)
- Tokens not used for over 1 hour are removed from the in-memory cache
- Only specific routes (`/verify/api/modellist`, `/verify/api/cardwrite`, `/verify/api/cardsearch`) accept token auth; all other admin routes require cookie session

#### CORS Handling
- Mobile app requests must include `X-Requested-With: com.mdl.arttagscanner` header
- CORS preflight (OPTIONS) requests are handled for specific routes
- `Access-Control-Allow-Origin` is set to the request origin
- `Access-Control-Allow-Credentials: true` is set for authenticated requests

#### Login Failure Logging
- Failed login attempts are logged with the client IP address
- The log format (`Login failed: <IP>`) is compatible with fail2ban

### 2.4 Card and Model Management

#### Card Operations
- **Write Card** (`cardWrite`): Creates a new card record or updates an existing one. Requires a valid ECDSA signature (`VerifyUIDBase64`). The card UID (7 bytes, base64URL-encoded) and signature (56 bytes, base64URL-encoded) are mandatory. A UUID-based `filekey` is auto-generated for new cards. Returns the `appmasterkey`, `filekey`, and `metakey` (hex-encoded).
- **Read Card** (`ReadCardData`): Returns the card signature and linked model ID for a given 7-byte UID.
- **Edit Card Status** (`cardEdit` with `cardchecked`): Resets card status to `NORMAL`.
- **Update Card File Key** (`cardEdit` with `cardpwupdate`): Updates the card's `filekey` (must be 32 hex chars).
- **Delete Card** (`cardEdit` with `carddel`): Deletes a card if the provided `pw` matches the stored `filekey`.
- **Search Cards** (`cardSearch`): Search by UID hex string (keyword) or by base64URL-encoded UID (`id` param). Returns `EXISTED`/`NOTEXIST` for ID-based search.

#### Model Operations
- **List Models** (`modelList`): Paginated list of all models (50 per page). Each model includes `id`, `name`, `desc`, `image`, and `img` (image URL path).
- **Write Model** (`modelWrite`): Create or update a model. If `modelId` is provided, updates existing; otherwise creates new. Image is stored as a file in `<exe-dir>/source/img/` (or `IMG_DIR` if set) with SHA-256 hash as filename. Supports deletion via `modelDel=1`.
- **Delete Model** (`modelWrite` with `modelDel=1`): Fails if any card is linked to the model (`ErrCardModelDelLinking`).
- **Search Models** (`modelSearch`): Case-insensitive search by model name.
- **Clean Images** (`modelImgClean`): Removes image files from `<exe-dir>/source/img/` (or `IMG_DIR`) that are no longer referenced by any model record.

---

## 3. NFC Verification Process

### 3.1 Overview

The NFC verification process authenticates NTAG 424 DNA tags using cryptographic verification. The process supports two encryption modes: **AES** (standard) and **LRP** (Leakage Resilient Primitive). The core verification logic is in `card/verify_base.go`.

### 3.2 Verification Flow (SUN Verify)

The primary verification endpoint is `POST /verify/sun`. The flow is as follows:

```
Client (NFC tag scan)
    |
    |  POST /verify/sun  with params: picc_data, cmac, d
    |
    v
verifySUN()  [server/server_card_verify.go]
    |
    |  Parse input: either separate picc_data+cmac, or packed "d" parameter
    |  - d=48 hex chars: AES mode (picc=first 32, cmac=last 16)
    |  - d=64 hex chars: LRP mode (picc=first 48, cmac=last 16)
    |
    |  Decode hex strings to bytes
    |
    v
VerifyPICCData(bPicc, bCmac, cardDB, debug)  [card/verify_base.go]
    |
    |  Step 1: Determine encryption mode
    |  - piccEncData length == 16 bytes -> AES mode
    |  - piccEncData length == 24 bytes -> LRP mode
    |
    |  Step 2: Decrypt PICC data using SdmMetaKey
    |  - AES mode: AES-128-CBC with IV=0x00*16
    |  - LRP mode: LRP decryption with piccRand=piccEncData[0:8]
    |
    |  Step 3: Parse decrypted PICC data (per AN12196 4.3)
    |  - Read config byte:
    |    - Bit 7: uidMirrored
    |    - Bit 6: sdmReadCtrMirrored
    |    - Bits 0-3: uidLength (must be 0x07)
    |  - Read uid (7 bytes) if uidMirrored
    |  - Read ctr (3 bytes) if sdmReadCtrMirrored
    |
    |  Step 4: Look up card in database by uid
    |  - ReadCardFilekey(uid) -> returns filekey, saved_ctr
    |
    |  Step 5: Calculate SDM MAC
    |  - AES mode: calculateSdmMac(filekey, [uid+ctr], false)
    |    - Build buffer: [0x3c, 0xc3, 0x00, 0x01, 0x00, 0x80] + piccData
    |    - CMAC(filekey, buffer) -> c2
    |    - CMAC(c2, []) -> c3
    |    - Extract even-indexed bytes: ret[1,3,5...]
    |  - LRP mode: calculateSdmMac(filekey, [uid+ctr], true)
    |    - Build buffer: [0x00, 0x01, 0x00, 0x80] + piccData + [0x1E, 0xE1]
    |    - LRP master key = LRP(filekey).cmac(buffer)
    |    - LRP session MACing key = LRP(masterKey)
    |    - macDigest = sessionKey.cmac([])
    |    - Extract even-indexed bytes
    |
    |  Step 6: Compare calculated MAC with received CMAC
    |  - If match: proceed to CTR validation
    |  - If not match: return ErrVerifySunNotMatch
    |
    |  Step 7: Validate CTR (counter)
    |  - If request_ctr > saved_ctr:
    |    - If delta > 2: status = "JUMP"
    |    - Else: status = "NORMAL"
    |    - Update CTR in database: UpdateCardCTR(uid, ctrBuf, status)
    |    - Return card UID
    |  - If request_ctr <= saved_ctr:
    |    - Status = "REPEATED" (replay attack detected)
    |    - Update CTR in database: UpdateCardCTR(uid, cardCtr, "REPEATED")
    |    - Return ErrVerifySunCtrRepeated
    |
    v
verifySUN() continues
    |
    |  If verification succeeded:
    |    - Look up linked model: ReadCardData(cid) -> link
    |    - Return {"msg": "OK", "info": "data valid", "link": "<model_id>"}
    |
    |  If verification failed:
    |    - Return {"msg": "FAIL", "info": "data invalid"}
    |    - Log CTR replay: "Card id: <uid_hex>, the CTR is repeated"
    |
    |  For GET requests: render verify-page.html with result
    |  For POST requests: return JSON response
```

### 3.3 UID Signature Verification

The `POST /verify/api` endpoint performs ECDSA signature verification:

```
verifyUID()  [server/server_card_verify.go]
    |
    |  Parse params: s (signature, base64URL), u (uid, base64URL)
    |
    v
VerifyUIDBase64(sign, uid)  [card/verify_base.go]
    |
    |  Decode base64URL strings
    |
    v
VerifyUID(sign, uid)  [card/verify_base.go]
    |
    |  Split signature into (r, s) big integers
    |  - Expected signature length: 2 * ((224+7)/8) = 56 bytes
    |  - r = sign[0:28], s = sign[28:56]
    |
    |  Verify using ECDSA with P-224 curve
    |  - Public key is hardcoded (compressed format, 57 bytes hex)
    |  - ecdsa.Verify(publicKey, uid, r, s)
    |
    |  If valid: look up card in database, return link
    |  If invalid: return result=false
```

### 3.4 Cryptographic Keys

| Key | Location | Purpose | Size |
|-----|----------|---------|------|
| `SdmMetaKey` | `local/metakey` | AES/LRP key for decrypting PICC data from NFC tags | 16 bytes (AES-128) |
| `AppMasterKey` | `local/appmasterkey` | Key 0 used to authorize tag administration | 16 bytes (AES-128) |
| `cardFileKey` | `carddata.db` (per card) | SDM MAC calculation key, unique per NFC tag | 16 bytes (hex-encoded UUID) |
| ECDSA Public Key | Hardcoded in `verify_base.go` | Verifying card UID signatures | P-224 curve |
| Admin Password Hash | `admin.db` (admin-hash bucket) | Admin login verification | HMAC-SHA256 |
| Debug Key | Hardcoded (all zeros) | Used when `--debug` flag is set | 16 bytes |

### 3.5 Card Status Values

| Status | Description |
|--------|-------------|
| `NORMAL` | Card CTR incremented normally (delta <= 2) |
| `JUMP` | Card CTR jumped by more than 2 (possible tampering) |
| `REPEATED` | Card CTR was replayed (request CTR <= saved CTR) |

Once a card is marked `REPEATED`, the status cannot be overwritten back to `NORMAL` or `JUMP` by the CTR update logic. Only the admin `cardchecked` action can reset it to `NORMAL`.

---

## 4. Database Schema

The server uses **bbolt** (BoltDB), an embedded key-value database. Data is organized into buckets within database files. There is no SQL schema; all structure is defined in Go code.

### 4.1 Database Files

| File | Created By | Description |
|------|-----------|-------------|
| `admin.db` | `CreateCardDB()` | Admin records: login sessions and credentials |
| `carddata.db` | `CreateCardDB()` | Card data and model/artefact records |
| `adminsession.db` | `MakeCardAdmin()` | Iris web framework session storage |
| `local/metakey` | `MakeMetaKeyFile()` | 16-byte AES key for PICC decryption (not a bbolt database) |
| `local/appmasterkey` | `MakeAppMasterKeyFile()` | 16-byte AES key provisioned as NTAG application key 0 |

### 4.2 Bucket Structure

#### `admin.db`

**Bucket: `admin-record`**

Stores API login session tokens.

| Key | Value | Description |
|-----|-------|-------------|
| UUID string (session ID) | 8-byte varint (Unix timestamp) | API login session with creation time |

- Sessions expire after 7 days (`API_SESSION_TIME`)
- `OnLoginUser()` creates a new session entry
- `CheckLoginSession()` validates and checks expiry
- `GetLoginRecord()` returns all sessions as JSON
- `RemoveLoginRecord()` deletes a session by ID
- `OnLoginUpdate()` batch-updates session timestamps (used by `AppSession` periodic sync)

**Bucket: `admin-hash`**

Stores admin credentials.

| Key | Value | Description |
|-----|-------|-------------|
| `rec` | HMAC-SHA256 hex string | Admin password hash |
| `salt` | UUID string | Salt used for password hashing |

- Initial/default credentials use a hardcoded salt (`2f6a4c16e84e`) and hash
- After password change, a new UUID salt is generated
- Password hash is computed as: `HMAC-SHA256(salt, username + password)` formatted as hex
- `CheckAdminLogin()` first checks the `admin-hash` bucket; if no `rec` key exists, falls back to the hardcoded default

#### `carddata.db`

**Bucket: `card-data`**

Stores NFC card records. Each card is identified by its 7-byte UID.

| Key | Value (JSON) | Description |
|-----|-------------|-------------|
| 7-byte card UID (raw bytes) | `{"sign": "...", "ctr": "...", "link": "...", "filekey": "...", "status": "..."}` | Card record |

**JSON field details:**

| Field | Type | Description |
|-------|------|-------------|
| `sign` | string (base64) | ECDSA signature of the card UID (56 bytes, base64-encoded) |
| `ctr` | string (base64) | SDM Read Counter (3 bytes, base64-encoded) |
| `link` | string (UUID) | ID of the linked model/artefact record |
| `filekey` | string (hex) | SDM File Read Key for MAC calculation (UUID bytes, hex-encoded) |
| `status` | string | Card status: `NORMAL`, `JUMP`, or `REPEATED` |

**Key operations:**
- `WriteCardData()`: Create new card (auto-generates UUID filekey) or update existing card (preserves filekey)
- `ReadCardData()`: Read signature and link by UID
- `ReadCardFilekey()`: Read filekey and CTR by UID (used during verification)
- `UpdateCardCTR()`: Update CTR and status (status cannot change from `REPEATED` to other values)
- `CheckedCardStatus()`: Reset status to `NORMAL` (admin action)
- `UpdateCardPW()`: Update filekey (must be valid 32-char hex string)
- `DelUpdateCard()`: Delete card if provided password matches filekey
- `CardSearchJSON()`: Search cards by UID hex string (max 50 results)
- `GetCardList()`: Paginated card listing (50 per page)

**Bucket: `link-data`**

Stores model/artefact records. Each model is identified by a UUID.

| Key | Value (JSON) | Description |
|-----|-------------|-------------|
| UUID string | `{"name": "...", "desc": "...", "image": "..."}` | Model/artefact record |

**JSON field details:**

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Model/artefact name |
| `desc` | string | Model/artefact description |
| `image` | string | Image filename (SHA-256 hash of image content) stored in `<exe-dir>/source/img/` (or `IMG_DIR`) |

**Key operations:**
- `ModelAddLink()`: Create or update a model record
- `ModelGetLink()`: Read a model by ID
- `ModelDelLink()`: Delete a model (fails if any card links to it)
- `ModelLinkListJSON()`: Paginated model listing (50 per page)
- `ModelSearchJSON()`: Search models by name (case-insensitive, max 50 results)
- `ModelGetRemainImages()`: Returns map of unreferenced image filenames (for cleanup)

#### `adminsession.db`

**Bucket: `irissession`**

Managed by the Iris sessions package (`github.com/kataras/iris/v12/sessions/sessiondb/boltdb`). Stores serialized session data for web admin authentication. Structure is internal to the Iris framework.

### 4.3 Data Relationships

```
link-data (Model)
    |
    | 1:N (one model can be linked by many cards)
    |
card-data (Card)
    |
    | Each card has:
    | - sign: ECDSA signature (verifies card authenticity)
    | - filekey: SDM MAC key (used during NFC verification)
    | - ctr: Read counter (anti-replay protection)
    | - link: Foreign key to link-data
    | - status: Verification status
    |
    v
Verification Process
    Uses: SdmMetaKey (from local/metakey) to decrypt PICC data
    Uses: card's filekey to calculate and verify SDM MAC
    Uses: card's ctr to detect replay attacks
```

### 4.4 File-Based Data

| Path | Description |
|------|-------------|
| `local/metakey` | 16-byte AES key for PICC data decryption. Auto-generated on first startup. |
| `local/appmasterkey` | 16-byte application master key for tag administration. Auto-generated on first startup. |
| `source/img/<hash>` | Model/artefact images. Filename is the SHA-256 hash of the image content. Located at `<exe-dir>/source/img/` (or `IMG_DIR`). |
| `config/` | Runtime configuration directory (gitignored). Contains log files. |

---

## 5. Source Code Structure

```
tag-reader-server/
|-- main.go                          # Entry point, CLI app setup
|-- go.mod / go.sum                  # Go module definition
|-- build-bin.sh                     # Cross-compile build script
|-- nfc-server-ip.service            # systemd service unit
|
|-- card/                            # Core NFC verification & database logic
|   |-- card_admin.go                # Admin session management (login/logout/cookies)
|   |-- card_db.go                   # Card & model database operations (bbolt)
|   |-- db_types.go                  # Interfaces (ICardDB, IDB, IBucket, ITx, Clock, UUID)
|   |-- lrp.go                       # LRP encryption implementation (AN12304)
|   |-- verify_base.go               # NFC tag verification (AES & LRP), SDM MAC, ECDSA UID verify
|   |-- mock/db_mock.go              # Generated GoMock mocks
|
|-- server/                          # HTTP server & API route handlers
|   |-- server_main.go               # Server initialization, CLI parsing, Iris app setup
|   |-- server_admin.go              # Admin page routes, session management, AppSession
|   |-- server_card_verify.go        # Card verify API endpoints (SUN verify, card/model CRUD)
|   |-- server_search.go             # Search API endpoints (model search, card search)
|
|-- ancientAuth/                     # DEPRECATED - Ancient artefact authentication
|   |-- authdb.go                    # Hardcoded demo data, histogram comparison
|   |-- server_auth.go               # Auth server routes (scan, check, submit)
|
|-- local/                           # Frontend static assets
|   |-- html/                        # HTML templates (admin pages, verify page)
|   |-- js/                          # JavaScript (admin, verify, i18n)
|   |-- css/                         # CSS stylesheets
|
|-- docs/                            # Documentation
|-- climgt-remote/                   # Git submodule: CLI management library
```

---

## 6. Security Considerations

- **Debug mode** (`--debug`): Uses an all-zero test key for verification. Must not be used in production.
- **MetaKey file** (`local/metakey`): This 16-byte key is critical for PICC data decryption. If compromised, an attacker could decrypt NFC tag data. The file is created with mode `0640`.
- **Default admin credentials**: A hardcoded salt and password hash exist in `card_admin.go`. After the first password change, the new credentials are stored in the database.
- **CTR replay protection**: The server detects and flags replay attacks by comparing the request counter with the stored counter. Repeated CTR values result in `REPEATED` status.
- **fail2ban integration**: Failed login attempts are logged in a format compatible with fail2ban, allowing automatic IP banning.
- **CORS**: Only requests with the `X-Requested-With: com.mdl.arttagscimulator` header are allowed for cross-origin API access.

---

## 7. References

- [AN12196: NTAG 424 DNA and NTAG 424 DNA TagTamper features and hints](https://www.nxp.com/docs/en/application-note/AN12196.pdf) - NXP application note for NTAG 424 DNA tag verification
- [AN12304: Leakage Resilient Primitive (LRP) Specification](https://www.nxp.com/docs/en/application-note/AN12304.pdf) - NXP application note for LRP encryption
- [sdm-backend](https://github.com/icedevml/sdm-backend) - Reference implementation for SDM backend
- [bbolt](https://github.com/etcd-io/bbolt) - Embedded key-value database for Go
- [Iris Web Framework](https://github.com/kataras/iris) - Go web framework
