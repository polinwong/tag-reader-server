# MetaKey Lifecycle and Usage

> The MetaKey (`SdmMetaKey`) is a 16-byte AES-128 key used to decrypt PICC (Proximity Integrated Circuit Card) data from NFC NTAG 424 DNA tags. This document traces its complete lifecycle from generation to usage, with source code references.

---

## 1. What is the MetaKey?

The MetaKey is the **SDM Meta Read Key** defined in the NXP NTAG 424 DNA specification (AN12196). It is shared between the server and the NFC tag. When an NFC tag is tapped, the tag encrypts its PICC data (containing the card UID and read counter) using this key. The server then decrypts the PICC data to verify the tag's authenticity.

**File location:** `local/metakey` (relative to the server working directory)

**In-memory variable:** `card.SdmMetaKey` (`[]byte`, 16 bytes)

Source: `card/verify_base.go:34`
```go
SdmMetaKey []byte // Meta key is auto generate from startup
```

---

## 2. Lifecycle Events

### 2.1 Generation (First Startup)

When the server starts for the first time and the `metakey` file does not exist or is empty, a new random 16-byte key is generated and written to the file.

Source: `card/verify_base.go:61-83`
```go
func MakeMetaKeyFile(targetPath string) {
    keyFile, err := os.OpenFile(targetPath+"/metakey", os.O_CREATE|os.O_RDWR, 0640)
    if err != nil {
        panic("must create key file to verify NFC NTAG424DNA")
    }
    defer keyFile.Close()

    s, _ := keyFile.Stat()
    if s.Size() == 0 {
        raw := MakeBytes(16, 0)
        if n, err := rand.Read(raw); err != nil || n != 16 {
            panic("create key failed")
        }
        keyFile.Write(raw)
    } else if s.Size() == 16 {
        SdmMetaKey = MakeBytes(16, 0)
        if n, err := keyFile.Read(SdmMetaKey); err != nil || n != 16 {
            panic("read current key failed")
        }
    } else {
        panic("key file format error")
    }
}
```

**Key behavior:**
- File does not exist or is empty (size == 0): generates a new random key via `crypto/rand.Read()` and writes it to the file. **Note:** `SdmMetaKey` is NOT loaded into memory in this branch -- the key is only written to disk. The in-memory `SdmMetaKey` remains `nil` until the next startup reads it.
- File exists and is exactly 16 bytes: reads the key into `SdmMetaKey`.
- File exists but is not 0 or 16 bytes: **panics** with `"key file format error"`.

### 2.2 Loading (Subsequent Startups)

On every subsequent startup, `MakeMetaKeyFile` is called again. Since the file now exists with 16 bytes, the key is read from the file into the `SdmMetaKey` global variable.

The call chain is:

```
main() -> server.ServerMain() -> MakeCardPage() -> MakeVerifyCardLocal(dbPath)
```

Source: `server/server_card_verify.go:79`
```go
if cardBase = MakeVerifyCardLocal(dbPath); cardBase == nil {
    LogFatalf("Error when create card verify library\n")
}
```

Source: `card/verify_base.go:53-59`
```go
func MakeVerifyCard(dbPath string) (card *VerifyCard) {
    card = new(VerifyCard)
    card.genPublicKey()
    MakeMetaKeyFile(dbPath)
    return card
}
```

**The metakey file is never modified after initial generation.** It is only read on subsequent startups.

### 2.3 Distribution to NFC Tags (Card Write)

When a new NFC card is registered in the system via the `POST /verify/api/cardwrite` endpoint, the server returns the current `SdmMetaKey` (hex-encoded) to the client (mobile app). The mobile app then programs the NFC tag with this key.

Source: `server/server_card_verify.go:337`
```go
ctx.JSON(iris.Map{"msg": "OK", "id": id, "fkey": fkey, "metakey": hex.EncodeToString(card.SdmMetaKey)})
```

This is the **only time the metakey leaves the server**. The mobile app uses it to configure the NTAG 424 DNA tag's SDM Meta Read Key setting.

### 2.4 Usage in NFC Verification (PICC Decryption)

When an NFC tag is scanned and the verification request arrives at `POST /verify/sun`, the `SdmMetaKey` is used to decrypt the PICC data received from the tag.

Source: `card/verify_base.go:199-223`
```go
metaKey := SdmMetaKey
if debug {
    metaKey = sdmTestKey
}

// Decryption for transfer data
if !lrp {
    // Key must use 16 byte (AES-128)
    b, cerr := aes.NewCipher(metaKey)
    if cerr != nil {
        err = cerr
        return
    }

    // Block cipher method must CBC, IV is 16 byte array with "0"
    bm := cipher.NewCBCDecrypter(b, make([]byte, 16))
    bm.CryptBlocks(output, piccEncData)
} else {
    // LRP decryption
    piccRand := piccEncData[0:8]
    piccEncDataStripped := piccEncData[8:]

    cipher := NewLRP(metaKey, 0, piccRand, false)
    output = cipher.Decrypt(piccEncDataStripped)
}
```

**Two decryption modes:**
- **AES mode** (PICC data is 16 bytes): AES-128-CBC decryption with IV = all zeros.
- **LRP mode** (PICC data is 24 bytes): Leakage Resilient Primitive decryption using the first 8 bytes as the random value.

**Debug mode:** When the `--debug` CLI flag is set, `sdmTestKey` (all zeros) is used instead of `SdmMetaKey`:

Source: `card/verify_base.go:36-39`
```go
sdmTestKey = []byte{
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
    0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
} // Debug key, don't use debug mode on deploy server
```

---

## 3. Failure Scenarios

| Scenario | Server Behavior | Impact on NFC Verification |
|----------|----------------|---------------------------|
| `metakey` file deleted | New random key generated on next startup | All previously registered tags fail verification (PICC decryption produces garbage, SDM MAC mismatch) |
| `metakey` file truncated to 0 bytes | New random key generated (same as deleted) | All previously registered tags fail verification |
| `metakey` file corrupted (size != 0 and != 16) | Server **panics** and refuses to start | Server is down, no verification possible |
| `metakey` file has wrong 16 bytes | Server starts normally, reads the wrong key | All previously registered tags fail verification (wrong decryption key) |
| `SdmMetaKey` is `nil` (first-generation bug) | AES cipher creation would panic | Server crashes during verification |

**Important:** There is no mechanism to rotate or update the metakey. Once NFC tags are programmed with a specific metakey, that key must remain in the `local/metakey` file for those tags to remain verifiable.

---

## 4. Relationship with Other Keys

The NFC verification process uses **two separate keys**:

| Key | Purpose | Stored In | Per-Tag? |
|-----|---------|-----------|----------|
| `SdmMetaKey` (MetaKey) | Decrypt PICC data from the NFC tag | `local/metakey` file | No -- shared across all tags |
| `filekey` (SDM File Read Key) | Calculate SDM MAC to verify tag authenticity | `carddata.db` (per card record) | Yes -- unique per tag |

The verification flow is:
1. **Decrypt** PICC data using `SdmMetaKey` -> extract UID and CTR
2. **Look up** the card's `filekey` in the database by UID
3. **Calculate** SDM MAC using the card's `filekey`
4. **Compare** calculated MAC with the received CMAC

Both keys must be correct for verification to succeed. The `SdmMetaKey` is the first gate; the per-tag `filekey` is the second.

---

## 5. Backup Recommendation

The `local/metakey` file is small (16 bytes) but critical. Loss of this file renders all registered NFC tags permanently unverifiable. It should be backed up whenever:
- A new server instance is set up
- New NFC tags are programmed
- The `local/` directory is modified

A simple backup command:
```bash
cp local/metakey local/metakey.bak.$(date +%Y%m%d%H%M%S)
```

---

## 6. Source Code Reference Summary

| Event | File | Line(s) | Function |
|-------|------|---------|----------|
| Global variable declaration | `card/verify_base.go` | 34 | `SdmMetaKey []byte` |
| File generation/loading | `card/verify_base.go` | 61-83 | `MakeMetaKeyFile()` |
| Called during startup | `card/verify_base.go` | 53-59 | `MakeVerifyCard()` |
| Startup trigger | `server/server_card_verify.go` | 79 | `MakeVerifyCardLocal(dbPath)` |
| Distribution to client | `server/server_card_verify.go` | 337 | `cardWrite()` handler |
| AES decryption usage | `card/verify_base.go` | 199-215 | `VerifyPICCData()` |
| LRP decryption usage | `card/verify_base.go` | 217-223 | `VerifyPICCData()` |
| Debug key override | `card/verify_base.go` | 36-39, 200-202 | `sdmTestKey` |
| Test: generation and loading | `card/verify_base_test.go` | 19-44 | `TestMakeMetaKeyFile` |
