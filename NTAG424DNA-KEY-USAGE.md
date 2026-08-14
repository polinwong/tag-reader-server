# NTAG424DNA Key Usage in NFC Verification System

> **Derived from source code analysis, verified against NXP NTAG424DNA datasheet (NT4H2421Gx) and application note (AN12196).** This document is based on direct analysis of the server (Go, v0.3.2) and app (TypeScript/Java, v0.2.2) source code. Existing documentation that does not include the term "derived from source code analysis" may be outdated and for reference only.

---

## 1. Overview

This document describes every cryptographic key used in the NFC NTAG424DNA tag system, covering four processes:

1. **Reading (verifying) NFC tags** — the scanner reads tag data and the server verifies it
2. **Writing NFC tags** — the admin app provisions new tags with keys and SDM configuration
3. **Verification of tags** — the server-side cryptographic verification of tag data
4. **Clearing tags** — resetting tags to factory-default state

The system uses the NXP NTAG424DNA tag's **Secure Dynamic Messaging (SDM)** feature, which dynamically inserts encrypted PICC data and a CMAC into the NDEF URL at read time. Two symmetric keys protect this process, and an ECDSA key provides offline tag authenticity verification.

---

## 2. Complete Key Inventory

### 2.1 NFC Tag Keys (Symmetric)

| Key Name | NTAG424DNA Slot | Size | Scope | Storage Location | Purpose |
|----------|-----------------|------|-------|-----------------|---------|
| **SdmMetaKey** (MetaKey) | AppKey0 (software-mapped; hardware role is configurable) | 16 bytes (AES-128) | Global (shared by all tags) | Server: `local/metakey` file; Tag: AppKey0; App: received from server at write time | SDM Meta Read Key — encrypts PICC data (UID + read counter) on the tag; server uses it to decrypt PICC data during verification |
| **cardFileKey** (FileKey) | AppKey1 (software-mapped; hardware role is configurable) | 16 bytes (AES-128) | Per-tag (unique per tag) | Server: `carddata.db` (hex-encoded UUID); Tag: AppKey1; App: received from server at write time | SDM File Read Key — tag uses it to generate SDM CMAC; server uses it to calculate and verify the CMAC during verification |

### 2.2 ECDSA Key (Asymmetric)

| Key Name | Curve | Size | Scope | Storage Location | Purpose |
|----------|-------|------|-------|-----------------|---------|
| **ECDSA Public Key** | secp224r1 (P-224) | 56 bytes (uncompressed point) | Global | Hardcoded in both server (`verify_base.go:87`) and app (`NxpNfcPlugin.java:28`) | Verifies the tag's UID signature to confirm the tag is a genuine NXP NTAG424DNA |

The ECDSA public key hex value (identical in both server and app):
```
048A9B380AF2EE1B98DC417FECC263F8449C7625CECE82D9B916C992DA209D68422B81EC20B65A66B5102A61596AF3379200599316A00A1410
```

### 2.3 Debug/Test Key

| Key Name | Size | Scope | Storage Location | Purpose |
|----------|------|-------|-----------------|---------|
| **sdmTestKey** | 16 bytes (all zeros) | Debug only | Hardcoded in `verify_base.go:36-39` | Replaces both SdmMetaKey and cardFileKey when `--debug` CLI flag is set. **Must never be used in production.** |

### 2.4 Admin Authentication Key

| Key Name | Algorithm | Scope | Storage Location | Purpose |
|----------|-----------|-------|-----------------|---------|
| **Admin Password Hash** | HMAC-SHA256 | Global | Server: `cardrecord.db` (admin-hash bucket); hardcoded default in `card_admin.go:31-32` | Verifies admin login credentials for write/clear operations |

### 2.5 NXP TapLinx SDK License Keys

| Key Name | Storage Location | Purpose |
|----------|-----------------|---------|
| **SDK License Key** | Hardcoded in `NxpNfcPlugin.java:78` (`d2807611153655b8185b2dddc2de4bad`) | Activates the NXP NFC SDK on Android |
| **SDK Offline Key** | Hardcoded in `NxpNfcPlugin.java:79` | Offline license verification for the SDK |

---

## 3. NTAG424DNA Key Slot Mapping

### 3.1 Hardware Facts (NTAG424DNA Product)

Per the NTAG424DNA datasheet (NT4H2421Gx) and application note (AN12196), the tag hardware provides:

**Application keys:** The NTAG424DNA MIFARE application supports **5 application keys** (AppKey0 through AppKey4). All default to `0x00...00` (16 bytes of zeros) at factory delivery.

- **AppKey0** is the **application master key** by default (controls application-level settings, key changes, and file access rights changes).
- **AppKey1 through AppKey4** are general-purpose application keys, with no fixed roles at the hardware level.

**SDM key mapping is configurable:** The NTAG424DNA's SDM (Secure Dynamic Messaging) feature defines two logical key roles — **SDM Meta Read Key** and **SDM File Read Key** — but these are **not fixed to specific key numbers**. They are configurable parameters that map to any AppKey number. The NXP AN12196 example uses AppKey2 for SDM Meta Read Key and AppKey1 for SDM File Read Key, but this is just one possible configuration.

**Files:** The NTAG424DNA MIFARE application contains **3 Standard Data files**:

| File Number | NXP Name | Default Size | Purpose |
|-------------|----------|-------------|---------|
| **01h** (File 1) | CC file | 32 bytes | NFC Forum Capability Container — describes the NDEF structure |
| **02h** (File 2) | NDEF file | 256 bytes | NDEF message data — **this is the file where SDM is enabled** |
| **03h** (File 3) | Proprietary file | 128 bytes | Application-specific proprietary data |

SDM can only be enabled on **File 02h** (the NDEF file). At read time, SDM dynamically inserts encrypted PICC data and CMAC into the NDEF URL stored in this file.

### 3.2 Software Configuration (This Project)

This project's software maps the system's keys to the tag's key slots as follows:

```
NTAG424DNA Tag
  |
  +-- Application (DF Name: D2760000850101)
        |
        +-- AppKey0 ← SdmMetaKey (software choice, NOT hardware requirement)
        |     - Software role: SDM Meta Read Key
        |     - Used for: Authenticating to the application; encrypting PICC data
        |     - Key Version: 0x01 (after personalization), 0x00 (factory default)
        |
        +-- AppKey1 ← cardFileKey (software choice, NOT hardware requirement)
        |     - Software role: SDM File Read Key
        |     - Used for: Generating SDM CMAC over PICC data
        |     - Key Version: 0x01 (after personalization), 0x00 (factory default)
        |
        +-- AppKey2 (unused by this software, remains factory default)
        +-- AppKey3 (unused by this software, remains factory default)
        +-- AppKey4 (unused by this software, remains factory default)
        |
        +-- File 01h (CC file, 32 bytes) — Capability Container
        +-- File 02h (NDEF file, 256 bytes) — SDM enabled here
        |     - SDM inserts PICC data and CMAC into NDEF URL at read time
        +-- File 03h (Proprietary file, 128 bytes) — unused by this software
```

**Important distinction:** The mapping of SdmMetaKey→AppKey0 and cardFileKey→AppKey1 is a **software design choice** in this project, not a hardware requirement. The NTAG424DNA hardware allows these SDM key roles to be mapped to any AppKey number. The code uses `writeMetaId=0` and `writeFileId=1` (see `WriterPage.vue:130-132`), which could be changed to use different key slots.

**Key version semantics:**
- `0x00` = Factory default key (all zeros `0x00...00`)
- `0x01` = Personalized key (server-generated SdmMetaKey or cardFileKey)

The `changeKey()` call in the app's `writeTagProcess()` sets the key version to `0x01` when personalizing, and back to `0x00` when clearing.

---

## 4. Key Lifecycle and Generation

### 4.1 SdmMetaKey (MetaKey)

**Generation:** On first server startup, if `local/metakey` file does not exist or is empty, a 16-byte random key is generated using Go's `crypto/rand.Read()` and written to the file.

Source: `card/verify_base.go:61-83` — `MakeMetaKeyFile()`

```
First startup:
  local/metakey does not exist or size == 0
    -> Generate 16 random bytes via crypto/rand
    -> Write to local/metakey
    -> NOTE: SdmMetaKey global variable remains nil (bug)
    -> Server must be restarted for the key to be loaded into memory

Subsequent startups:
  local/metakey exists with size == 16
    -> Read 16 bytes into SdmMetaKey global variable
```

**Distribution:** The MetaKey is sent to the mobile app only during the card write process (`POST /verify/api/cardwrite`), as a hex-encoded string in the JSON response field `"metakey"`.

Source: `server/server_card_verify.go:337`

**Usage:** Used by the server to decrypt PICC data during tag verification. Used by the tag to encrypt PICC data at read time.

**No rotation mechanism exists.** If the MetaKey file is lost or changed, all previously programmed tags become unverifiable.

### 4.2 cardFileKey (FileKey)

**Generation:** When a new card is registered via `WriteCardData()`, a UUID v4 is generated and its raw bytes (hex-encoded) become the FileKey.

Source: `card/card_db.go:665-709` — `WriteCardData()`

```
New card:
  UUID v4 generated via DbUUID.NewV4()
  fkey = hex.EncodeToString(uuid.Bytes())  // 32 hex chars = 16 bytes

Existing card (re-write):
  Existing fkey is preserved and returned
```

**Distribution:** The FileKey is sent to the mobile app during the card write process, as a hex-encoded string in the JSON response field `"fkey"`.

Source: `server/server_card_verify.go:337`

**Usage:** Used by the tag to generate the SDM CMAC. Used by the server to calculate the expected CMAC during verification.

**Update:** Can be updated via `POST /verify/api/cardpwupdate` (admin action). The tag must be reprogrammed separately after a FileKey update.

**Deletion authorization:** The FileKey serves as the authorization password for card deletion (`DelUpdateCard()` compares the provided `pw` against the stored `fkey`).

### 4.3 ECDSA Public Key

**Hardcoded** in both the server and the app. This is the NXP factory public key for NTAG424DNA tag signature verification. It cannot be changed without modifying the source code.

Source: `card/verify_base.go:87` (server), `NxpNfcPlugin.java:28` (app)

---

## 5. Process: Reading NFC Tags

This describes what happens when a user scans an NFC tag in the Scanner page.

### 5.1 On-Device (App) — No Keys Used for Reading

When a tag is tapped, the app's native plugin (`NxpNfcPlugin.cardLogic()`) performs:

1. **Card type detection** — `NxpNfcLib.getCardType()` checks if the tag is `CardType.NTAG424DNA`
2. **Version check** — `objNTAG424DNA.getVersion()`, first byte must be `0x04` (NXP)
3. **Signature read** — `tag.readSignature()` retrieves the 56-byte ECDSA signature from the tag
4. **Offline signature verification** — `verifySignature(PUBLIC_KEY, signature, uid)` using `ECDSASecp224Cryptogram`
   - Uses the **ECDSA Public Key** (hardcoded)
   - If invalid, the tag is rejected immediately ("The tag invalid!")
5. **NDEF read** — `tag.readNDEF()` reads the NDEF record from the tag
   - The NDEF URI contains SDM-generated data: `https://<domain>/verify/sun?d=<PICC_DATA><CMAC_DATA>`
   - The PICC_DATA and CMAC_DATA are dynamically inserted by the tag's SDM feature at read time
   - **No symmetric keys are used by the app during reading** — the tag handles encryption and MAC generation internally

**Key point:** The app does NOT decrypt or verify the SDM data locally. It only verifies the ECDSA signature. The SDM data (PICC + CMAC) is sent to the server for verification.

### 5.2 Data Read from Tag

| Data | Size | Description |
|------|------|-------------|
| UID | 7 bytes | Tag unique identifier |
| Signature | 56 bytes | ECDSA signature of UID (secp224r1) |
| NDEF URI | Variable | Contains SDM-injected PICC data and CMAC |

### 5.3 Data Sent to Server

The app sends data to the server in two steps:

**Step 1 — Signature verification** (`POST /verify/api`):
- `s` = Base64URL-encoded signature (56 bytes)
- `u` = Base64URL-encoded UID (7 bytes)

**Step 2 — SDM data verification** (`POST /verify/sun`):
- `d` = hex string extracted from NDEF URI after `d=`
  - AES mode: 48 hex chars = 32 hex (PICC, 16 bytes) + 16 hex (CMAC, 8 bytes)
  - LRP mode: 64 hex chars = 48 hex (PICC, 24 bytes) + 16 hex (CMAC, 8 bytes)

---

## 6. Process: Verification of Tags (Server-Side)

This is the core cryptographic verification performed by the server.

### 6.1 Step 1: ECDSA Signature Verification

**Endpoint:** `POST /verify/api`

**Key used:** ECDSA Public Key (hardcoded, P-224 curve)

**Process:**
1. Decode Base64URL-encoded `sign` and `uid`
2. Split signature into `(r, s)` — each 28 bytes (P-224 = 224 bits / 8 = 28 bytes)
3. Verify: `ecdsa.Verify(publicKey, uid, r, s)`
4. If valid, look up the card in the database by UID

Source: `card/verify_base.go:327-357`

### 6.2 Step 2: SDM Data Verification (SUN Verify)

**Endpoint:** `POST /verify/sun`

**Keys used:** SdmMetaKey (for decryption) + cardFileKey (for CMAC verification)

**Process:**

```
Input: picc_data (hex), cmac (hex) — extracted from "d" parameter

1. PARSE INPUT
   - If d length == 48 hex chars: AES mode (picc = d[0:32], cmac = d[32:48])
   - If d length == 64 hex chars: LRP mode (picc = d[0:48], cmac = d[48:64])
   - Decode hex strings to bytes

2. DECRYPT PICC DATA using SdmMetaKey
   ┌─────────────────────────────────────────────────────────┐
   │ AES mode (piccEncData is 16 bytes):                    │
   │   cipher = AES-128-CBC(SdmMetaKey, IV=0x00*16)        │
   │   plaintext = cipher.Decrypt(piccEncData)              │
   │                                                         │
   │ LRP mode (piccEncData is 24 bytes):                    │
   │   piccRand = piccEncData[0:8]                          │
   │   piccEncStripped = piccEncData[8:]                    │
   │   lrp = NewLRP(SdmMetaKey, u=0, r=piccRand, pad=false)│
   │   plaintext = lrp.Decrypt(piccEncStripped)             │
   └─────────────────────────────────────────────────────────┘

   Debug mode: SdmMetaKey is replaced with sdmTestKey (all zeros)

3. PARSE DECRYPTED PICC DATA (per AN12196 section 4.3)
   - config byte:
     - Bit 7: uidMirrored (must be 1)
     - Bit 6: sdmReadCtrMirrored (must be 1)
     - Bits 0-3: uidLength (must be 0x07)
   - uid: 7 bytes
   - ctr: 3 bytes (SDM read counter)

4. LOOK UP CARD in database
   - cardFileKey, savedCtr = db.ReadCardFilekey(uid)
   - If UID not found: verification fails

5. CALCULATE SDM MAC using cardFileKey
   ┌─────────────────────────────────────────────────────────┐
   │ AES mode:                                              │
   │   buffer = [0x3C, 0xC3, 0x00, 0x01, 0x00, 0x80]       │
   │          + uid + ctr                                   │
   │   c2 = CMAC(cardFileKey, buffer)                       │
   │   c3 = CMAC(c2, [])           // empty file data       │
   │   expectedMac = odd-indexed bytes of c3                │
   │   // i.e., c3[1], c3[3], c3[5], c3[7], c3[9],        │
   │   //       c3[11], c3[13], c3[15]  → 8 bytes          │
   │                                                         │
   │ LRP mode:                                              │
   │   buffer = [0x00, 0x01, 0x00, 0x80]                   │
   │          + uid + ctr                                   │
   │          + [0x1E, 0xE1]                                │
   │   lrpMaster = NewLRP(cardFileKey, 0, 0x00*16, true)   │
   │   masterKey = lrpMaster.cmac(buffer)                   │
   │   lrpSession = NewLRP(masterKey, 0, 0x00*16, true)    │
   │   macDigest = lrpSession.cmac([])  // empty file data  │
   │   expectedMac = odd-indexed bytes of macDigest         │
   └─────────────────────────────────────────────────────────┘

   Debug mode: cardFileKey is replaced with sdmTestKey (all zeros)

6. COMPARE MAC
   - If expectedMac == received cmac: MAC verification passes
   - If not: return ErrVerifySunNotMatch

7. VALIDATE READ COUNTER (anti-replay)
   - If request_ctr > saved_ctr:
     - delta = request_ctr - saved_ctr
     - If delta > 2: status = "JUMP" (possible tampering)
     - Else: status = "NORMAL"
     - Update CTR in database
     - Verification succeeds → return card UID
   - If request_ctr <= saved_ctr:
     - status = "REPEATED" (replay attack detected)
     - Verification fails → return ErrVerifySunCtrRepeated
```

Source: `card/verify_base.go:187-303`

### 6.3 Key Usage Summary for Verification

| Step | Key Used | Operation | Purpose |
|------|----------|-----------|---------|
| ECDSA verify | ECDSA Public Key | Signature verification | Confirm tag is genuine NXP NTAG424DNA |
| PICC decrypt | SdmMetaKey | AES-128-CBC decrypt (or LRP decrypt) | Extract UID and read counter from encrypted PICC data |
| SDM MAC calc | cardFileKey | AES-CMAC (or LRP-CMAC) | Verify the PICC data was generated by the genuine tag |
| CTR check | (none — database comparison) | Integer comparison | Detect replay attacks |

---

## 7. Process: Writing NFC Tags

This describes the complete flow when an admin writes a new tag or re-writes an existing tag.

### 7.1 Server-Side: Card Write Preflight

**Endpoint:** `POST /verify/api/cardwrite`

**Keys involved:** ECDSA Public Key (signature verification), SdmMetaKey (returned to app), cardFileKey (generated/returned)

**Process:**
1. Receive `id` (Base64URL UID), `sign` (Base64URL signature), `link` (model ID)
2. Verify ECDSA signature: `VerifyUIDBase64(sign, id)` using the **ECDSA Public Key**
3. If signature is invalid: return FAIL
4. Write card data to database: `WriteCardData(uid, ctr=[0,0,0], signature, link)`
   - **New card:** Generate UUID v4 → hex-encode as FileKey; store in `carddata.db`
   - **Existing card:** Preserve existing FileKey; update link and signature
5. Return to app:
   ```json
   {
     "msg": "OK",
     "id": "<Base64URL UID>",
     "fkey": "<32-char hex string = cardFileKey>",
     "metakey": "<32-char hex string = SdmMetaKey>"
   }
   ```

Source: `server/server_card_verify.go:311-338`

**Critical:** Both the SdmMetaKey and cardFileKey are transmitted to the mobile app over HTTPS. These keys are then programmed into the NFC tag.

### 7.2 App-Side: Pre-Write Configuration

The app receives `fkey` and `metakey` from the server and configures the native plugin:

```typescript
// WriterPage.vue:247-249
this.writeFileKey = ret.data.fkey       // e.g., "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"
this.writeMetaKey = ret.data.metakey    // e.g., "11223344556677889900aabbccddeeff"
await this.writeDataUpdate()
```

`writeDataUpdate()` calls `NxpNfc.preWriteData()` which configures the native plugin with:
- `dataFileId` = 1 (AppKey1 slot)
- `dataFileKey` = cardFileKey (16 bytes)
- `dataMetaId` = 0 (AppKey0 slot)
- `dataMetaKey` = SdmMetaKey (16 bytes)
- `dataClear` = false
- `lrp` = user selection
- `changeMode` = user selection

### 7.3 Native Plugin: Tag Write Process

**Method:** `NxpNfcPlugin.writeTagProcess()`

**Keys involved:** SdmMetaKey (AppKey0), cardFileKey (AppKey1), KEY_AES128_DEFAULT (factory default)

**Process:**

```
1. AUTHENTICATE WITH META KEY
   try:
     authNTAG424DNAFirst(tag, writeMetaId=0, writeMetaKey=SdmMetaKey)
     // This authenticates with AppKey0 using the server's MetaKey
   catch:
     // Authentication failed — tag still has factory default keys
     authNTAG424DNAFirst(tag, 0, KEY_AES128_DEFAULT)  // Auth with default key
     tag.changeKey(0, KEY_AES128_DEFAULT, SdmMetaKey, version=0x01)
     // ↑ Change AppKey0 from default to SdmMetaKey, set version to 0x01
     authNTAG424DNAFirst(tag, 0, SdmMetaKey)           // Re-auth with new MetaKey
     tag.changeKey(1, KEY_AES128_DEFAULT, cardFileKey, version=0x01)
     // ↑ Change AppKey1 from default to cardFileKey, set version to 0x01

2. GET FILE SETTINGS for File 02h (NDEF file)
   f = tag.getFileSettings(2)   // 2 = file number 02h

3. CONFIGURE SDM
   f.setSDMEnabled(true)
   f.setUIDMirroringEnabled(true)
   f.setSDMReadCounterEnabled(true)

   Calculate offsets:
     basicOffset = 3 (new domain) or 7 (old domain)
     basicUrl = "www.verifynfc.top/verify/sun?d=" or "nfc.coinllectibles.art/verify/sun?d="
     piccPadding = "0"*48 (LRP) or "0"*32 (AES)  // placeholder for SDM-injected PICC data
     cmacPadding = "0"*16                          // placeholder for SDM-injected CMAC

     piccOffset = basicOffset + basicUrl.length()
     macOffset = piccOffset + piccPadding.length()

   f.setPiccDataOffset([piccOffset, 0, 0])
   f.setSdmMacOffset([macOffset, 0, 0])

   if AES mode:
     f.setSdmMacInputOffset([macOffset, 0, 0])    // MAC input offset = MAC offset
   if LRP mode:
     f.setSdmMacInputOffset([piccOffset, 0, 0])   // MAC input offset = PICC offset

   f.setSdmAccessRights([0xFF, 0x01])
   // 0xFF01 = REV(7-4)=F, counter(3-0)=F, Meta(15-12)=0, File(11-8)=1
   // Counter: free read, Meta key: no access, File key: read access

4. APPLY FILE SETTINGS
   tag.changeFileSettings(2, f)   // File 02h (NDEF file)

5. WRITE NDEF RECORD
   uri = "https://" + basicUrl + piccPadding + cmacPadding
   tag.writeNDEF(NdefMessage(uri))
   // The SDM feature will replace the padding with real PICC data and CMAC at read time
```

### 7.4 Authentication During Write

The `authNTAG424DNAFirst()` method performs:

```
1. Select NTAG424DNA application: tag.isoSelectApplicationByDFName([D2,76,00,00,85,01,01])

2. Authenticate:
   if LRP mode:
     PCD_CAP2 = [0x02, 0x00, 0x00, 0x00, 0x00, 0x00]
     tag.authenticateLRPFirst(keyId, keyData, PCD_CAP2)
   else (AES mode):
     PCD_CAP = [0x00, 0x00]
     tag.authenticateEV2First(keyId, keyData, PCD_CAP)

3. Set capability data:
   tag.setCapabilityData(writeModeChange != writeLRP, PCD_CAP)
   // This configures the tag's communication mode for subsequent operations
```

### 7.5 Key Usage Summary for Writing

| Step | Key Used | Operation | Purpose |
|------|----------|-----------|---------|
| ECDSA verify | ECDSA Public Key | Signature verification | Confirm tag is genuine before writing |
| Auth (new tag) | KEY_AES128_DEFAULT | authenticateEV2First / authenticateLRPFirst | Authenticate with factory default key |
| Change AppKey0 | KEY_AES128_DEFAULT → SdmMetaKey | changeKey(0, old, new, 0x01) | Set MetaKey on tag |
| Change AppKey1 | KEY_AES128_DEFAULT → cardFileKey | changeKey(1, old, new, 0x01) | Set FileKey on tag |
| Auth (existing tag) | SdmMetaKey | authenticateEV2First / authenticateLRPFirst | Authenticate with existing MetaKey |
| SDM config | (none — file settings) | changeFileSettings (File 02h) | Enable SDM, set offsets and access rights |
| NDEF write | (none — plain NDEF) | writeNDEF | Write verification URL with SDM placeholders |

---

## 8. Process: Clearing Tags

Clearing a tag resets it to factory-default state: SDM is disabled, and both keys are reverted to the all-zeros default.

### 8.1 App-Side Configuration

When `writeClear = true` in WriterPage:
- The `cardWritePreflight()` sends `link: ''` (empty) to the server
- The server still returns `fkey` and `metakey` (needed to authenticate with the current keys before clearing)
- `preWriteData()` is called with `dataClear: true`

### 8.2 Native Plugin: Clear Process

**Method:** `NxpNfcPlugin.writeTagProcess()` (when `writeClear = true`)

**Keys involved:** SdmMetaKey (to authenticate before clearing), cardFileKey (to change back), KEY_AES128_DEFAULT (target state)

**Process:**

```
1. AUTHENTICATE with current MetaKey
   authNTAG424DNAFirst(tag, writeMetaId=0, writeMetaKey=SdmMetaKey)
   // Must authenticate with the current key to be allowed to change it

2. GET FILE SETTINGS for File 02h (NDEF file)
   f = tag.getFileSettings(2)   // 2 = file number 02h

3. DISABLE SDM
   f.setSDMEnabled(false)
   f.setUIDMirroringEnabled(false)
   f.setSDMReadCounterEnabled(false)
   f.setSdmAccessRights([0xFF, 0xFF])
   // 0xFFFF = fully restricted — no SDM data will be generated

4. REVERT KEYS TO DEFAULT
   // Change AppKey0 (MetaKey) back to default
   authNTAG424DNAFirst(tag, 0, SdmMetaKey)
   tag.changeKey(0, SdmMetaKey, KEY_AES128_DEFAULT, version=0x00)
   // ↑ version 0x00 indicates factory default key

   // Change AppKey1 (FileKey) back to default
   authNTAG424DNAFirst(tag, 0, KEY_AES128_DEFAULT)
   tag.changeKey(1, cardFileKey, KEY_AES128_DEFAULT, version=0x00)

5. APPLY FILE SETTINGS
   tag.changeFileSettings(2, f)

6. WRITE NDEF RECORD (with empty SDM placeholders)
   uri = "https://" + basicUrl + piccPadding + cmacPadding
   tag.writeNDEF(NdefMessage(uri))
   // Since SDM is disabled, the placeholders will NOT be replaced at read time
```

### 8.3 Key Usage Summary for Clearing

| Step | Key Used | Operation | Purpose |
|------|----------|-----------|---------|
| Auth | SdmMetaKey | authenticateEV2First / authenticateLRPFirst | Authenticate with current MetaKey to gain write access |
| Change AppKey0 | SdmMetaKey → KEY_AES128_DEFAULT | changeKey(0, old, new, 0x00) | Revert MetaKey to factory default |
| Re-auth | KEY_AES128_DEFAULT | authenticateEV2First / authenticateLRPFirst | Re-authenticate after MetaKey changed |
| Change AppKey1 | cardFileKey → KEY_AES128_DEFAULT | changeKey(1, old, new, 0x00) | Revert FileKey to factory default |
| SDM disable | (none — file settings) | changeFileSettings (File 02h) | Disable SDM, set access rights to 0xFFFF |

**Important:** After clearing, the tag will NOT generate SDM data when read. The NDEF URL will contain the literal padding zeros instead of encrypted PICC data and CMAC. The tag is effectively "de-personalized" and cannot be verified by the server until it is re-written.

---

## 9. Cryptographic Operation Details

### 9.1 AES Mode (Default)

**PICC Encryption (on tag):**
- Algorithm: AES-128-CBC
- Key: SdmMetaKey (AppKey0)
- IV: 16 bytes of `0x00`
- Plaintext: 1-byte config + 7-byte UID + 3-byte CTR = 11 bytes (padded to 16 bytes)
- Ciphertext: 16 bytes

**PICC Decryption (on server):**
- Same algorithm in reverse
- Source: `card/verify_base.go:205-215`

**SDM CMAC Calculation (on tag):**
- Algorithm: Double AES-CMAC per AN12196
- Key: cardFileKey (AppKey1)
- Input: `[0x3C, 0xC3, 0x00, 0x01, 0x00, 0x80]` + PICC data (UID + CTR)
- Step 1: `c2 = CMAC(cardFileKey, input)`
- Step 2: `c3 = CMAC(c2, [])` — second CMAC with empty file data
- Step 3: Extract odd-indexed bytes: `result = c3[1], c3[3], c3[5], ..., c3[15]` → 8 bytes
- Source: `card/verify_base.go:119-139`

**Authentication:**
- Method: `authenticateEV2First(keyId, keyData, PCD_CAP)`
- PCD_CAP: `[0x00, 0x00]`

### 9.2 LRP Mode

**PICC Encryption (on tag):**
- Algorithm: Leakage Resilient Primitive (AN12304)
- Key: SdmMetaKey (AppKey0)
- Random: 8-byte `piccRand` prepended to ciphertext
- Ciphertext: 8 bytes (piccRand) + encrypted data = 24 bytes total

**PICC Decryption (on server):**
- Extract `piccRand = piccEncData[0:8]`
- Strip `piccEncStripped = piccEncData[8:]`
- Create LRP instance: `NewLRP(SdmMetaKey, u=0, r=piccRand, pad=false)`
- Decrypt: `lrp.Decrypt(piccEncStripped)`
- Source: `card/verify_base.go:217-223`

**SDM CMAC Calculation (on tag):**
- Algorithm: LRP-CMAC per AN12304
- Key: cardFileKey (AppKey1)
- Input: `[0x00, 0x01, 0x00, 0x80]` + PICC data + `[0x1E, 0xE1]`
- Step 1: `lrpMaster = NewLRP(cardFileKey, 0, 0x00*16, true)`
- Step 2: `masterKey = lrpMaster.cmac(input)` — derive master key
- Step 3: `lrpSession = NewLRP(masterKey, 0, 0x00*16, true)` — derive session key
- Step 4: `macDigest = lrpSession.cmac([])` — CMAC with empty file data
- Step 5: Extract odd-indexed bytes → 8 bytes
- Source: `card/verify_base.go:140-173`

**Authentication:**
- Method: `authenticateLRPFirst(keyId, keyData, PCD_CAP2)`
- PCD_CAP2: `[0x02, 0x00, 0x00, 0x00, 0x00, 0x00]`

**Warning:** LRP mode is **irreversible** — once a tag is configured for LRP, it cannot be reverted to AES mode. The app displays a warning: "Change to LRP cannot fallback to AES".

### 9.3 How the SDM CMAC "Odd-Indexed Bytes" Extraction Works

The NTAG424DNA SDM feature produces a 16-byte CMAC, but only 8 bytes are included in the NDEF URL (to save space). The extraction takes every other byte starting from index 1:

```
Full CMAC (16 bytes):  [c0, c1, c2, c3, c4, c5, c6, c7, c8, c9, c10, c11, c12, c13, c14, c15]
Extracted SDM MAC:     [c1, c3, c5, c7, c9, c11, c13, c15]  → 8 bytes
```

Source: `card/verify_base.go:109-117` — `finBytes()` function

---

## 10. SDM Configuration Details

### 10.1 NDEF URL Structure

The NDEF URL written to the tag has this structure:

```
https://<domain>/verify/sun?d=<PICC_PLACEHOLDER><CMAC_PLACEHOLDER>
```

Where:
- `<domain>` = `www.verifynfc.top` (new) or `nfc.coinllectibles.art` (old)
- `<PICC_PLACEHOLDER>` = 32 hex zeros (AES) or 48 hex zeros (LRP)
- `<CMAC_PLACEHOLDER>` = 16 hex zeros

At read time, the tag's SDM feature replaces the placeholders with real data:
- PICC placeholder → encrypted PICC data (UID + CTR, encrypted with SdmMetaKey)
- CMAC placeholder → SDM CMAC (8 bytes, generated with cardFileKey)

### 10.2 SDM Offsets

| Parameter | AES Mode | LRP Mode | Description |
|-----------|----------|----------|-------------|
| `piccDataOffset` | `basicOffset + urlLength` | `basicOffset + urlLength` | Where PICC data starts in the URL |
| `sdmMacOffset` | `piccOffset + 32` | `piccOffset + 48` | Where CMAC starts in the URL |
| `sdmMacInputOffset` | `macOffset` (same as sdmMacOffset) | `piccOffset` (same as piccDataOffset) | Input offset for MAC calculation |

The difference in `sdmMacInputOffset` between AES and LRP modes is significant:
- **AES:** The MAC input starts at the MAC position (CMAC is calculated over just the CMAC field)
- **LRP:** The MAC input starts at the PICC position (CMAC is calculated over both PICC and CMAC fields)

### 10.3 SDM Access Rights

| Mode | Value | Meaning |
|------|-------|---------|
| Write (active) | `[0xFF, 0x01]` | Counter: free read (F); Meta key: no access (0); File key: read access (1) |
| Clear (disabled) | `[0xFF, 0xFF]` | Fully restricted — no SDM data generated |

---

## 11. Complete Key Flow Diagrams

### 11.1 Write New Tag

```
Server                          App (WriterPage)              Tag (NTAG424DNA)
  │                                  │                            │
  │  POST /verify/api/cardwrite      │                            │
  │  ← {id, sign, link}              │                            │
  │                                  │                            │
  │  VerifyUID(sign, uid)            │                            │
  │  [ECDSA Public Key]              │                            │
  │                                  │                            │
  │  WriteCardData()                 │                            │
  │  → Generate UUID v4 as FileKey   │                            │
  │  → Store {uid, fkey, ctr=0,     │                            │
  │     sign, link} in carddata.db   │                            │
  │                                  │                            │
  │  → {fkey, metakey} ─────────────│                            │
  │                                  │                            │
  │                                  │  preWriteData()            │
  │                                  │  [Store fkey, metakey]     │
  │                                  │                            │
  │                                  │  writeTagProcess()         │
  │                                  │                            │
  │                                  │  authNTAG424DNAFirst()     │
  │                                  │  [KEY_AES128_DEFAULT]  ────│── auth EV2 First
  │                                  │                            │
  │                                  │  changeKey(0, DEFAULT, ────│── AppKey0 = SdmMetaKey
  │                                  │    SdmMetaKey, 0x01)       │   (version 0x01)
  │                                  │                            │
  │                                  │  changeKey(1, DEFAULT, ────│── AppKey1 = cardFileKey
  │                                  │    cardFileKey, 0x01)      │   (version 0x01)
  │                                  │                            │
  │                                  │  changeFileSettings(2) ────│── SDM enabled
  │                                  │  [SDM config]              │   offsets set
  │                                  │                            │
  │                                  │  writeNDEF(url) ───────────│── URL with placeholders
  │                                  │                            │
```

### 11.2 Read and Verify Tag

```
Tag (NTAG424DNA)                App (ScannerPage)              Server
  │                                  │                            │
  │  [SDM generates at read time]    │                            │
  │  PICC = Encrypt(SdmMetaKey,      │                            │
  │          UID + CTR)              │                            │
  │  CMAC = SDM_MAC(cardFileKey,     │                            │
  │          UID + CTR)              │                            │
  │                                  │                            │
  │  NDEF URL with real PICC+CMAC ───│                            │
  │                                  │                            │
  │  readSignature() ────────────────│                            │
  │                                  │  verifySignature()         │
  │                                  │  [ECDSA Public Key]        │
  │                                  │  (offline check)           │
  │                                  │                            │
  │                                  │  POST /verify/api          │
  │                                  │  {s: sign, u: uid} ────────│
  │                                  │                            │  VerifyUID(sign, uid)
  │                                  │                            │  [ECDSA Public Key]
  │                                  │  ← {result: true} ────────│
  │                                  │                            │
  │                                  │  POST /verify/sun          │
  │                                  │  {d: picc+cmac} ───────────│
  │                                  │                            │
  │                                  │                            │  1. Decrypt PICC
  │                                  │                            │     [SdmMetaKey]
  │                                  │                            │     → UID, CTR
  │                                  │                            │
  │                                  │                            │  2. Lookup FileKey
  │                                  │                            │     [cardFileKey from DB]
  │                                  │                            │
  │                                  │                            │  3. Calculate SDM MAC
  │                                  │                            │     [cardFileKey]
  │                                  │                            │
  │                                  │                            │  4. Compare MACs
  │                                  │                            │
  │                                  │                            │  5. Check CTR
  │                                  │                            │     (anti-replay)
  │                                  │  ← {msg: "OK", link} ─────│
  │                                  │                            │
```

### 11.3 Clear Tag

```
Server                          App (WriterPage)              Tag (NTAG424DNA)
  │                                  │                            │
  │  POST /verify/api/cardwrite      │                            │
  │  ← {id, sign, link=""}           │                            │
  │  → {fkey, metakey} ─────────────│                            │
  │                                  │                            │
  │                                  │  writeTagProcess()         │
  │                                  │  [writeClear = true]       │
  │                                  │                            │
  │                                  │  authNTAG424DNAFirst()     │
  │                                  │  [SdmMetaKey] ─────────────│── auth EV2 First
  │                                  │                            │
  │                                  │  changeFileSettings(2) ────│── SDM disabled
  │                                  │  [SDM access = 0xFFFF]     │
  │                                  │                            │
  │                                  │  changeKey(0, SdmMetaKey, ─│── AppKey0 = DEFAULT
  │                                  │    DEFAULT, 0x00)           │   (version 0x00)
  │                                  │                            │
  │                                  │  authNTAG424DNAFirst()     │
  │                                  │  [DEFAULT] ────────────────│── re-auth
  │                                  │                            │
  │                                  │  changeKey(1, cardFileKey, ─│── AppKey1 = DEFAULT
  │                                  │    DEFAULT, 0x00)           │   (version 0x00)
  │                                  │                            │
  │                                  │  writeNDEF(url) ───────────│── URL with placeholders
  │                                  │                            │  (SDM won't fill them)
```

---

## 12. Debug Mode

When the server is started with `--debug` flag (`appdebug = true` in `server_main.go:123`):

- **SdmMetaKey** is replaced with `sdmTestKey` (16 bytes of `0x00`) for PICC decryption
- **cardFileKey** is replaced with `sdmTestKey` for SDM MAC calculation
- This allows verification of tags programmed with the all-zeros default key

Source: `card/verify_base.go:200-202` and `card/verify_base.go:262-264`

**Warning:** Debug mode completely bypasses the cryptographic security. It must never be used in production.

---

## 13. Security Observations

### 13.1 Key Management

| Observation | Severity | Detail |
|-------------|----------|--------|
| MetaKey shared across all tags | Medium | A single MetaKey compromise affects all tags. Per-tag MetaKeys would be more secure but are not supported by the current design. |
| FileKey is UUID v4, not crypto-derived | Low | UUID v4 has 122 bits of randomness, which is sufficient for AES-128, but it's not generated via a KDF. |
| Both keys transmitted to app | Medium | The MetaKey and FileKey leave the server during the write process. Compromise of the app or network could expose these keys. Mitigated by HTTPS. |
| No key rotation | Medium | There is no mechanism to rotate the MetaKey. If compromised, all tags must be reprogrammed. |
| First-startup MetaKey bug | Low | On first startup, the MetaKey is written to disk but not loaded into memory. The server must be restarted. Source: `verify_base.go:69-74` |
| LRP mode not fully tested | Medium | Code comment explicitly states "LRP case, not fully tested" (`verify_base.go:193`) |
| EncFileData not implemented | Low | SDM MAC only covers PICC data, not file data. The `encFile` parameter in `calculateSdmMac` is unused. |

### 13.2 Tag State After Operations

| Operation | AppKey0 | AppKey1 | SDM | Key Version | Verifiable? |
|-----------|---------|---------|-----|-------------|-------------|
| Factory default | `0x00*16` | `0x00*16` | Disabled | `0x00` | No |
| After write | SdmMetaKey | cardFileKey | Enabled | `0x01` | Yes |
| After clear | `0x00*16` | `0x00*16` | Disabled | `0x00` | No |
| After LRP write | SdmMetaKey | cardFileKey | Enabled (LRP) | `0x01` | Yes (LRP only) |

---

## 14. Source Code Reference

### Server (Go)

| File | Key-Related Functions | Description |
|------|----------------------|-------------|
| `card/verify_base.go:34` | `SdmMetaKey` | Global MetaKey variable |
| `card/verify_base.go:36-39` | `sdmTestKey` | Debug test key (all zeros) |
| `card/verify_base.go:53-59` | `MakeVerifyCard()` | Initialize VerifyCard, generate/load MetaKey |
| `card/verify_base.go:61-83` | `MakeMetaKeyFile()` | Generate or load MetaKey from file |
| `card/verify_base.go:86-100` | `genPublicKey()` | Initialize ECDSA public key |
| `card/verify_base.go:102-176` | `calculateSdmMac()` | Calculate SDM MAC (AES or LRP mode) |
| `card/verify_base.go:178-185` | `makeCMAC()` | AES-CMAC calculation |
| `card/verify_base.go:187-303` | `VerifyPICCData()` | Main verification: decrypt PICC, verify MAC, check CTR |
| `card/verify_base.go:305-324` | `VerifyPlainSUN()` | Plaintext SUN verify (debug only) |
| `card/verify_base.go:327-357` | `VerifyUIDBase64()` / `VerifyUID()` | ECDSA signature verification |
| `card/lrp.go:47-63` | `NewLRP()` | Create LRP instance |
| `card/lrp.go:68-91` | `LRP.Decrypt()` | LRP decryption |
| `card/lrp.go:191-264` | `LRP.cmac()` | LRP-CMAC calculation |
| `card/card_db.go:665-709` | `WriteCardData()` | Create/update card, generate FileKey |
| `card/card_db.go:735-754` | `ReadCardFilekey()` | Read FileKey and CTR from database |
| `card/card_db.go:779-803` | `UpdateCardPW()` | Update FileKey |
| `card/card_db.go:833-853` | `DelUpdateCard()` | Delete card (FileKey as auth) |
| `server/server_card_verify.go:311-338` | `cardWrite()` | Card write endpoint, returns fkey + metakey |
| `server/server_card_verify.go:428-527` | `verifySUN()` | SUN verify endpoint |
| `server/server_card_verify.go:543-578` | `verifyUID()` | UID signature verify endpoint |

### App (TypeScript/Java)

| File | Key-Related Functions | Description |
|------|----------------------|-------------|
| `NxpNfcPlugin.java:28` | `PUBLIC_KEY` | Hardcoded ECDSA public key |
| `NxpNfcPlugin.java:78-79` | SDK license keys | TapLinx SDK activation |
| `NxpNfcPlugin.java:131-133` | `writeFileKey`, `writeMetaKey` | Write key storage |
| `NxpNfcPlugin.java:169-179` | `intentEvent` listener | NFC tag detection handler |
| `NxpNfcPlugin.java:210-259` | `cardWritePreflight()` | Server write request, receive keys |
| `NxpNfcPlugin.java:328-343` | `writeDataUpdate()` | Configure native plugin with keys |
| `NxpNfcPlugin.java:writeTagProcess()` | `writeTagProcess()` | Full NFC write: auth, changeKey, SDM config, NDEF write |
| `NxpNfcPlugin.java:authNTAG424DNAFirst()` | `authNTAG424DNAFirst()` | Authenticate with AES or LRP |
| `NxpNfcPlugin.java:verifySignature()` | `verifySignature()` | ECDSA signature verification |
| `WriterPage.vue:131-133` | `writeFileKey`, `writeMetaKey` | Vue data properties for keys |
| `WriterPage.vue:247-249` | Key assignment from server response | Store keys from cardwrite response |
| `ScannerPage.vue:170-196` | `connectWork()` | Step 1: Send signature to server |
| `ScannerPage.vue:197-227` | `connectWorkSun()` | Step 2: Send SDM data to server |

---

## 15. Reference Documents

The following NXP reference documents are available in the `Reference/` folder:

| Document | Relevance |
|----------|-----------|
| `AN12196_NTAG424DNA_Application_note_Rev2.0.pdf` | Primary reference for SDM, PICC data format, CMAC calculation, and tag verification flow |
| `AN12304_Leakage_Resilient_Primitive_(LRP)_Specification.pdf` | LRP algorithm specification used in LRP mode encryption and CMAC |
| `AN12321_NTAG_424_DNA_(TagTamper)_features_and_hints_-_LRP_mode.pdf` | TagTamper features in LRP mode (not used in this system) |
| `NT4H2421Gx_NTAG424DNA_Product_data_sheet.pdf` | Tag hardware specification, memory layout, key settings |
| `NT4H2421Tx_NT4H2421Gx_NTAG424DNATT_Product_data_sheet.pdf` | TagTamper variant data sheet |
| `UG10044_Starting_development_with_TapLink_Android_SDK.pdf` | TapLink Android SDK (alternative to TapLinx, not used) |
| `UG10046_Starting_development_with_TapLinx_Java_SDK.pdf` | TapLinx Java SDK guide (the SDK used by this app) |
| `UM11133_TagXplorer_Quick_start-up_guide.pdf` | TagXplorer tool for testing tag operations |
