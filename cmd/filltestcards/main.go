// Command filltestcards writes sample card records into the local database
// so the admin UI and the card link endpoint can be tested without the
// Android app / desktop app.
//
// Usage (run from the tag-reader-server directory):
//
//	go run ./cmd/filltestcards             # write 5 test cards into ./local
//	go run ./cmd/filltestcards -n 10       # write 10 test cards
//	go run ./cmd/filltestcards -db ./local # explicit db path
//
// The program reuses the real card package, so it talks to the very same
// bbolt files the server uses (local/carddata.db, local/admin.db).
package main

import (
	"flag"
	"fmt"
	"os"

	"marveldigital/tag-reader-server/card"
)

func main() {
	n := flag.Int("n", 5, "number of test cards to create")
	dbPath := flag.String("db", "./local", "path to the local database directory")
	flag.Parse()

	db, err := card.CreateCardDB(*dbPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to open database:", err)
		os.Exit(1)
	}

	// Use existing models; if there are none, create a couple of placeholder
	// models so the cards have something meaningful to link to.
	models, _ := db.ModelLinkListJSON(0, 100)
	if len(models) == 0 {
		fmt.Println("no models found, creating placeholder models...")
		for _, name := range []string{"Test Model A", "Test Model B"} {
			info, e := card.NewModelInfo(name, "auto-created for UI test", "test.png", "")
			if e != nil {
				fmt.Fprintln(os.Stderr, "create model failed:", e)
				continue
			}
			if e := db.ModelAddLink(info); e != nil {
				fmt.Fprintln(os.Stderr, "add model failed:", e)
				continue
			}
		}
		models, _ = db.ModelLinkListJSON(0, 100)
	}

	if len(models) == 0 {
		fmt.Fprintln(os.Stderr, "no models available, cannot link cards")
		os.Exit(1)
	}
	fmt.Printf("Using %d model(s) for linking:\n", len(models))
	for _, m := range models {
		fmt.Printf("  - %s  (%s)\n", m["name"], m["id"])
	}

	// Write test cards, cycling through the available models.
	baseUID := []byte{0x04, 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x00}
	written := 0
	for i := 0; i < *n; i++ {
		uid := make([]byte, 7)
		copy(uid, baseUID)
		uid[6] = byte(i) // vary the last byte so every UID is unique

		// 56-byte dummy signature. WriteCardData only stores it (it does not
		// verify); a deterministic pattern keeps records reproducible.
		sign := make([]byte, 56)
		for j := range sign {
			sign[j] = byte(0xA0 + (i*7+j)%56)
		}

		ctr := []byte{0x00, 0x00, 0x01} // read counter = 1
		link := models[i%len(models)]["id"].(string)

		if _, err := db.WriteCardData(uid, ctr, sign, link); err != nil {
			fmt.Fprintf(os.Stderr, "write card %02X failed: %v\n", uid[6], err)
			continue
		}
		fmt.Printf("written card uid=%X  link=%s\n", uid, link)
		written++
	}
	fmt.Printf("Created %d test card(s).\n", written)

	// Demonstrate the new UpdateCardLink by re-linking the first card to a
	// different model (only if more than one model exists).
	if len(models) > 1 && *n > 0 {
		uid := make([]byte, 7)
		copy(uid, baseUID)
		uid[6] = 0
		newLink := models[1]["id"].(string)
		if err := db.UpdateCardLink(uid, newLink); err != nil {
			fmt.Fprintf(os.Stderr, "relink card %02X failed: %v\n", uid[6], err)
		} else {
			fmt.Printf("re-linked first card uid=%X -> %s (UpdateCardLink OK)\n", uid, newLink)
		}
	}
}
