# Migration Plan: bbolt → MySQL (RDS-ready)

> Goal: move `tag-reader-server` from local bbolt files to MySQL so it can be
> deployed on a cloud server backed by RDS. This plan is derived from reading
> `card/db_types.go`, `card/card_db.go`, `card/card_db_test.go`, `card/mock/db_mock.go`,
> and `go.mod`. **Code is source of truth.**

---

## 1. Current database shape (what we are migrating FROM)

The server uses **two bbolt files**, each with two "buckets" (key/value stores):

| File | Bucket (const) | Key | Value | Accessed via |
|------|----------------|-----|-------|--------------|
| `admin.db` | `admin-record` (`dbAdmin`) | varint session id (4-byte) | 8-byte unix-nano timestamp | `ForEach`, `Get`, `Put`, `Delete` |
| `admin.db` | `admin-hash` (`dbAdminHash`) | `"rec"` (hash), `"salt"` | raw bytes | `Get`, `Put` |
| `carddata.db` | `card-data` (`DB_CARD`) | **7-byte raw UID** | JSON `{sign, ctr(base64), link, filekey(hex), status}` | `Get`, `Put`, `ForEach`, `Delete`, `Stats` |
| `carddata.db` | `link-data` (`DB_LINK`) | model UUID string | JSON `{name, desc, image}` | `Get`, `Put`, `ForEach`, `Delete`, `Stats` |

There are effectively **4 logical stores**:
1. Admin login sessions
2. Admin credentials (salt + password hash)
3. Card/tag records
4. Model/link records

> **Out of scope — `AppMasterKey` / `SdmMetaKey` files:** These two NTAG 424 DNA
> keys live in `<dbPath>/appmasterkey` and `<dbPath>/metakey` (loaded by
> `card/verify_base.go` via `loadOrCreateAESKey`), **not** inside bbolt. They are
> NOT part of this bolt→MySQL migration. Per project decision, they remain
> unchanged for now and will later be externalized to a secrets manager (or
> equivalent) depending on the final cloud platform. On cloud deployment they
> must be carried over (copied) so previously-written tags stay verifiable — do
> **not** let them regenerate as new random keys.

---

## 2. Why this is low-risk: the existing abstraction

`card/db_types.go` already isolates all storage behind interfaces:

- `IDB` — `Close / Update / View / Batch`
- `ITx` — `CreateBucketIfNotExists / Bucket`
- `IBucket` — `ForEach / Get / Put / Delete / Stats`

And `card/card_db.go` exposes a swappable opener:

```go
var DbOpen = BboltOpen   // line ~83
```

Every `CardDatabase` method (`WriteCardData`, `ReadCardFilekey`, `ModelAddLink`,
`GetLoginRecord`, etc.) is written in terms of these interfaces only — **it never
mentions bbolt directly.** The test suite (`card_db_test.go`) already runs against
`mock.MockIDB`, proving the rest of the code is storage-agnostic.

➡ **Strategy: add a MySQL implementation of `IDB`/`ITx`/`IBucket` and flip
`DbOpen` to open it. Do NOT rewrite the `CardDatabase` methods or the tests.**

---

## 3. Target MySQL schema

One MySQL connection (or two logical schemas if you want to keep the
"admin vs card" separation). Recommended: two tables for cards/models, two for
admin. Keep keys as proper typed columns, not opaque blobs, so RDS backups and
queries are sane.

```sql
CREATE DATABASE nfcverify CHARACTER SET utf8mb4;

-- 3. Card / tag records
CREATE TABLE card_record (
    uid      BINARY(7)   PRIMARY KEY,        -- the 7-byte NTAG UID
    sign     VARCHAR(120) NOT NULL,          -- base64 ECDSA signature (56 bytes)
    ctr      VARCHAR(16)  NOT NULL,          -- base64 3-byte read counter
    link     VARCHAR(64)  NULL,              -- model UUID (FK to model_record)
    filekey  CHAR(32)     NOT NULL,          -- 16-byte hex (UUID) per-tag key
    status   ENUM('NORMAL','JUMP','REPEATED') NOT NULL DEFAULT 'NORMAL'
) ENGINE=InnoDB;

-- 4. Model / link records
CREATE TABLE model_record (
    id    CHAR(36) PRIMARY KEY,              -- UUID
    name  VARCHAR(255) NOT NULL,
    desc  TEXT,
    image VARCHAR(512)
) ENGINE=InnoDB;

-- 1. Admin login sessions
CREATE TABLE admin_session (
    id    INT UNSIGNED PRIMARY KEY,           -- varint session id
    ts    BIGINT NOT NULL                    -- unix nano timestamp
) ENGINE=InnoDB;

-- 2. Admin credentials
CREATE TABLE admin_credential (
    k     VARCHAR(16) PRIMARY KEY,           -- 'rec' or 'salt'
    v     VARBINARY(255) NOT NULL
) ENGINE=InnoDB;
```

Notes:
- `card-data` keys are **raw 7-byte binary** → `BINARY(7)`. If you prefer human-
  readable keys, hex-encode the UID to `CHAR(14)` and decode on read — but
  `BINARY(7)` is the faithful 1:1 mapping.
- `filekey`, `ctr`, `sign` were JSON string fields; they are now first-class
  columns. **This is a behavior-neutral change** because the current JSON already
  stored them as those exact strings.
- The `status` enum mirrors the three constants in `card_db.go` (`StatusCtrNormal/
  Jump/Repeated`). If a value outside the enum ever appears, use `VARCHAR(16)`
  instead and validate in app code.
- `ForEach` order: bbolt iterates keys in byte order. For listing endpoints this
  order is not contractually required (UI paginates), so `ORDER BY uid` /
  `ORDER BY id` is fine.

---

## 4. The MySQL `IDB` implementation (new file: `card/db_mysql.go`)

Keep the same interface signatures so `CardDatabase` is untouched.

```go
package card

import (
    "database/sql"
    _ "github.com/go-sql-driver/mysql"
)

type MysqlDB struct {
    db *sql.DB
    // bucket name -> table+keycol+valcol metadata
    tables map[string]tableMeta
}

type tableMeta struct {
    table  string
    keyCol string
    // for card-data link-data the value is JSON-ish per-field;
    // for admin buckets the value is a single raw column
    kind   string // "json" | "raw"
}
```

Mapping each interface method to SQL:

| Interface call | MySQL equivalent |
|----------------|-----------------|
| `CreateBucketIfNotExists(name)` | no-op (tables pre-created by migration SQL) — optionally verify via `information_schema` |
| `Bucket(name)` | returns a `MysqlBucket` bound to that table |
| `Bucket.Get(key)` | `SELECT <cols> FROM <table> WHERE <keycol> = ?` |
| `Bucket.Put(key,val)` | `INSERT ... ON DUPLICATE KEY UPDATE` (card-data/link-data) or `REPLACE` (admin) |
| `Bucket.Delete(key)` | `DELETE FROM <table> WHERE <keycol> = ?` |
| `Bucket.ForEach(fn)` | `SELECT <keycol>,<valcol> FROM <table>` then call `fn(keyBytes, valBytes)` |
| `Bucket.Stats().KeyN` | `SELECT COUNT(*) FROM <table>` |
| `IDB.View/Update/Batch(fn)` | `db.Begin()` tx, run `fn(tx)`, commit (Batch can just call Update) |

**Critical detail — value encoding for `ForEach`/`Get`:**
The 15 `CardDatabase` methods expect `Bucket.Get`/`ForEach` to return the **exact
same `[]byte`** bbolt returned. Today that's:
- card-data / link-data → the **JSON string bytes** of the record.
- admin-hash → raw `[]byte` value.
- admin-record → 8-byte big-endian timestamp.

So the MySQL bucket must **re-serialize rows back into those exact byte formats**:
- For `card_record`/`model_record`: marshal the row into the same JSON shape
  `{"sign":..,"ctr":..,"link":..,"filekey":..,"status":..}` before returning the
  bytes. (This keeps `card_db.go`'s `json.Unmarshal` working unchanged.)
- For `admin_credential`: return `v` as the raw `[]byte`.
- For `admin_session`: return the 8-byte big-endian encoding of `ts`.

This "JSON-as-value" compatibility shim means **zero changes to `card_db.go`**.

> Alternative (cleaner long-term): rewrite `CardDatabase` to use typed rows and
> drop the JSON round-trip. That is a larger change and touches tested code, so
> it is **out of scope for the minimal migration**; do it as a follow-up if desired.

---

## 5. Wiring / configuration

`card/card_db.go`:

```go
// replace
var DbOpen = BboltOpen
// with a configurable opener
var DbOpen = openDatabase   // picks mysql or bbolt by config
```

Add a config flag (env var or config constant in `main.go`):
- `DB_TYPE=bolt` (default, current behavior) or `DB_TYPE=mysql`.
- `DB_DSN="user:pass@tcp(rds-endpoint:3306)/nfcverify"`.

`MakeCardDatabase` / `CreateCardDB(dbpath)` currently takes a *path*. For MySQL,
`dbpath` becomes the DSN, or better: change `CreateCardDB` to accept a DSN and
only build file paths when `DB_TYPE=bolt`. Keep `CreateCardDBRaw` as-is (used by
tests, still works with mocks).

`go.mod`: add `github.com/go-sql-driver/mysql` (and run `go mod tidy`).

---

## 6. Migration of existing data (bolt → MySQL)

Write a small one-off migration tool (e.g. `cmd/migrate/main.go`) that:
1. Opens the bbolt files with `BboltOpen`.
2. Iterates each bucket with `ForEach`.
3. Inserts each key/value into the corresponding MySQL table, using the same
   JSON re-serialization described in §4.

Because bbolt stores card-data as JSON and we store it as columns, the migrator
should parse the JSON and `INSERT` column-by-column (not blindly copy bytes),
so the schema in §3 is populated correctly. For admin buckets, copy raw bytes.

Run order: `model_record` → `card_record` → `admin_credential` → `admin_session`.
Keep the bbolt files as backup until the MySQL deployment is verified.

---

## 7. Test strategy

- The existing `card_db_test.go` already runs fully against `mock.MockIDB` — those
  tests are storage-agnostic and **keep passing** with no change.
- Add a new test `card_db_mysql_test.go` that implements the `IDB`/`ITx`/`IBucket`
  interfaces backed by a real MySQL (or `go-sqlmock`) and runs the same
  `CardDatabase` method assertions. This locks in the compatibility shim.
- Add a test that opens a bbolt file, migrates to MySQL, and reads back via
  `CardDatabase` to prove parity.

---

## 8. Risks / gotchas

1. **Binary UID keys.** `BINARY(7)` is order-sensitive and case-sensitive in
   MySQL. Ensure the app always writes the exact 7 bytes; never hex-encode
   inconsistently between read and write.
2. **`ForEach` returning rows vs bbolt ordering.** Listing endpoints assume
   `ForEach` returns every record; SQL `SELECT` does too. Order differs but is
   not contractually required.
3. **Transaction semantics.** bbolt `Update` is a single writable tx. Map to a
   `*sql.Tx`; ensure `Batch` ≠ concurrent writes (it currently just calls Update
   in bbolt, so a plain tx is fine).
4. **`Stats().KeyN` → COUNT(*).** Used for pagination total. Replace precisely;
   the tests assert `size` matches record count.
5. **Admin password reset.** `admin_credential` starts empty in a fresh DB, so
   the code's fallback default-password path in `card_admin.go` will trigger
   until `ChangeAdminPW` is called on the new DB. Re-run admin setup on deploy.
6. **Connection pooling / timeouts.** For RDS, set `SetMaxOpenConns`,
   `SetConnMaxLifetime`, and handle `driver.ErrBadConn` reconnects (sql.DB does
   this automatically, but verify under load).
7. **No migration tooling today.** `go.mod` has no SQL driver — adding
   `go-sql-driver/mysql` is the only new dependency.

---

## 8b. Prerequisites & Local Testing Readiness (findings, 2026-09-02)

Before the migration can be **implemented and tested end-to-end locally**, the
following must be in place. As of this writing, the workspace machine does NOT
meet them.

### Environment check (run on the dev machine)
```
where mysql      # MySQL client
where mysqld     # MySQL server
where docker     # Docker (to run a MySQL container)
```
**Result on 2026-09-02:** `mysql`, `mysqld`, and `docker` were all **not found**.
So a real MySQL instance is unavailable locally.

### What is and isn't testable without MySQL
- **NOT testable against real MySQL:** the `card/db_mysql.go` backend, the
  `BINARY(7)` UID handling, `INSERT ... ON DUPLICATE KEY UPDATE`, tx semantics,
  and the `cmd/migrate` tool cannot be exercised without an actual MySQL/RDS
  instance.
- **Still testable without MySQL:** the existing unit tests run against
  `mock.MockIDB` (in-memory) and validate business logic, not the storage
  engine. They will stay green but do **not** prove MySQL compatibility.

### Prerequisites to begin local implementation + testing
Pick ONE of:
1. **Install MySQL Server locally** (e.g. MySQL 8.x). Heavier install (~400 MB+),
   needs admin/elevated rights. Then create a test DB and set
   `DB_TYPE=mysql`, `DB_DSN='root:pass@tcp(127.0.0.1:3306)/nfcverify_test'`.
2. **Install Docker Desktop** and run a container:
   `docker run -d --name nfc-mysql -p 3306:3306 -e MYSQL_ROOT_PASSWORD=test -e MYSQL_DATABASE=nfcverify_test mysql:8`
   Then point `DB_DSN` at `127.0.0.1:3306`.
3. **Defer real testing** until the cloud RDS environment exists; do
   compile-check + mock tests only for now.

### Other prerequisites / gotchas
- **New dependency:** `github.com/go-sql-driver/mysql` must be added to
  `go.mod` (`go get` + `go mod tidy`). No SQL driver is present today.
- **Schema must be applied first:** `card/db_schema.sql` (§3) must be executed
  against the target DB before the server starts in `mysql` mode (or the code
  must auto-create tables on first run — decide during implementation).
- **Carry over key files:** `appmasterkey` / `metakey` (see out-of-scope note
  in §1) are NOT in MySQL; copy them to the new environment so old tags stay
  verifiable. Do not let them regenerate.
- **Fresh-DB admin setup:** a new MySQL DB starts with empty `admin_credential`,
  so re-run the admin password setup step on first deploy (see risk #5).

> Decision pending: implement backend now (mock-test only) vs. wait for a MySQL
> instance. As of 2026-09-02 the user chose to **stop at doc updates** until a
> local/cloud MySQL is available.

---

## 9. Step-by-step execution checklist

1. `go get github.com/go-sql-driver/mysql` + `go mod tidy`.
2. Create `card/db_mysql.go` implementing `IDB`/`ITx`/`IBucket` over `database/sql`
   with the JSON re-serialization shim (§4).
3. Add `card/db_schema.sql` (§3) and a `cmd/migrate` tool (§6).
4. Make `DbOpen` configurable via `DB_TYPE`/`DB_DSN` (§5); default stays `bolt`.
5. Add `card_db_mysql_test.go` + migration parity test (§7).
6. Smoke test locally: run with `DB_TYPE=mysql`, exercise `/verify/sun`,
   `/verify/api`, login, model CRUD.
7. Run migrator against current `carddata.db`/`admin.db`; verify row counts.
8. Deploy to RDS-backed server; keep bbolt files as rollback backup.
