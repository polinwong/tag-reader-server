# Local Testing: Desktop App / Mobile vs. Local Server

This document explains how to test the NFC verification system against a
**locally running server** instead of the production domain (`verifynfc.top`).
It covers two separate channels that are often confused:

1. **Desktop app → server** (HTTP API calls made by the Python desktop app)
2. **Phone → server** (the phone opens the URL embedded in the NFC tag's NDEF message)

These two channels have **different reachability requirements**.

---

## 1. Two separate channels

| Channel | Who makes the request | What URL is used | Can use `localhost`? |
|---------|----------------------|------------------|----------------------|
| Desktop app → server | Python desktop app (`server/client.py`) | `SERVER_BASE_URL` env / config | ✅ Yes — runs on same machine |
| Phone → server | Phone browser/app after tapping tag | URL stored in **NDEF** of the tag | ❌ No — `localhost` = the phone itself |

**Key insight:** the NDEF URI written to a tag is *read by the phone*, not by the
desktop app. If you write `http://localhost:4430/...` to a tag, the phone will
try to open `localhost` **on the phone**, which never reaches your dev machine.

---

## 2. Desktop app → local server (HTTP, no NDEF involved)

The desktop app talks to the server over plain HTTP using `SERVER_BASE_URL`.

- Default (production): `https://www.verifynfc.top`
- Configured in `tag-reader-desktop/config.py`:
  ```python
  SERVER_BASE_URL = _get_env("SERVER_BASE_URL", "https://www.verifynfc.top")
  ```

### Run the local server

From `tag-reader-server/`:

```bash
# HTTP (no TLS) on port 4430, bound to all interfaces
go build -o sourceserver-local .
./sourceserver-local            # listens on :4430 (all interfaces)

# or with explicit flags
./sourceserver-local --port 4430 --ip 0.0.0.0
```

`servIP` defaults to empty, so `iris.Addr(":4430")` binds **all interfaces**
(`0.0.0.0`) — reachable from both the desktop app and other devices on the LAN.

If startup reports that the ancient authentication demo is disabled, this is
expected. That deprecated demo requires an optional, uncommitted reference
dataset under `source/authdata/`; it is unrelated to NFC tag verification and
the model images stored under `source/img/`.

### Point the desktop app at the local server

Either set the env var before launching the desktop app:

```bash
export SERVER_BASE_URL="http://localhost:4430"
python -m tag_reader_desktop        # or your launch command
```

or edit `config.py` (not recommended for committal). `localhost` / `127.0.0.1`
works fine here because the desktop app and server run on the **same machine**.

The desktop app then uses this base URL for all API calls
(`/verify/api`, `/verify/api/login`, `/verify/api/cardwrite`, etc.).

---

## 3. Phone → local server (NDEF URI must be phone-reachable)

When a tag is written, the desktop app builds the NDEF URI from a **domain
constant** (not from `SERVER_BASE_URL`):

```python
# tag-reader-desktop/keys/constants.py
DOMAIN_NEW = "verifynfc.top/verify/sun?d="
DOMAIN_OLD = "nfc.coinllectibles.art/verify/sun?d="
```

```python
# tag-reader-desktop/nfc/sdm.py  (line 91)
full_uri = f"{domain_url}{picc_padding}{CMAC_PADDING}"
```

The NTAG 424 stores this raw URI string and mirrors the encrypted PICC data /
CMAC at byte offsets derived from `len(domain_url)`. **The chip does not resolve
or validate the URL** — any string works as long as the offset math is correct.

So a **LAN IP is fully supported by SDM/NDEF**. The only requirement is that the
**phone can reach that address over the network**.

### Option A — LAN IP (same WiFi), `http://`

Write the tag with a LAN IP instead of the production domain. Because a bare IP
has no trusted TLS certificate, use `http://` (not `https://`) for local testing.

Example: if the dev machine's LAN IP is `192.168.1.50`:

```
http://192.168.1.50:4430/verify/sun?d=
```

To make the desktop writer emit this, temporarily set the domain constant, e.g.
in `tag-reader-desktop/keys/constants.py`:

```python
DOMAIN_NEW = "192.168.1.50:4430/verify/sun?d="
```

> Note: the NDEF URI builder in `sdm.py` prepends `https://` for the standard
> domains. For a LAN-IP test you may need the writer to use `http://` and skip
> the hardcoded `https://` prefix. Confirm the exact prefix logic in
> `nfc/sdm.py` before relying on this for a test run.

**Requirements for Option A to work:**
1. Phone is on the **same WiFi / LAN** as the dev machine.
2. Server bound to `0.0.0.0` (default) — not `127.0.0.1` only.
3. Firewall on the dev machine allows inbound TCP `4430`.
4. Use `http://` (no TLS) since a LAN IP has no valid certificate.

### Option B — Tunnel (recommended for phones)

A reverse tunnel gives you a real `https://` hostname that is reachable from the
phone without WiFi/firewall fiddling:

```bash
# cloudflared
cloudflared tunnel --url http://localhost:4430

# or ngrok
ngrok http 4430
```

Then set the NDEF domain constant to the tunnel hostname, e.g.:

```python
DOMAIN_NEW = "abc123.trycloudflare.com/verify/sun?d="
```

This keeps `https://` and avoids cert/firewall issues. Best for demos and
phone testing.

### Option C — Production domain (not local)

Writing the standard `verifynfc.top` domain means the phone hits the **production
server**, not your local one. Use this only when you intend to test against
production.

---

## 4. Quick reference matrix

| Goal | Desktop `SERVER_BASE_URL` | NDEF domain constant | Works? |
|------|---------------------------|----------------------|--------|
| App + phone both local, same WiFi | `http://localhost:4430` | `192.168.1.50:4430/verify/sun?d=` (`http://`) | ✅ |
| App local, phone via tunnel | `http://localhost:4430` | `<tunnel>.trycloudflare.com/verify/sun?d=` | ✅ |
| App local, phone on production | `http://localhost:4430` | `verifynfc.top/verify/sun?d=` | ⚠️ phone hits prod, app hits local |
| App + phone both `localhost` | `http://localhost:4430` | `localhost:4430/verify/sun?d=` | ❌ phone can't reach app's localhost |

---

## 5. Common mistakes

- **Writing `localhost` into the NDEF URI.** The phone reads it, not the desktop
  app, so `localhost` resolves to the phone itself → request always fails.
- **Server bound to `127.0.0.1` only.** Other LAN devices (and the phone) cannot
  connect. Use `0.0.0.0` (the default when `--ip` is omitted).
- **Using `https://` with a bare LAN IP.** No valid cert → phone browser rejects
  it. Use `http://` for LAN-IP testing, or a tunnel for `https://`.
- **Firewall blocking port 4430.** Allow inbound on the dev machine for LAN tests.
