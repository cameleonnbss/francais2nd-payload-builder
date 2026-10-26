// seal â€” transforme payload.json en payload.enc (AES-256-GCM).
//
// Usage : go run ./cmd/seal
// Ã€ lancer avant chaque compilation : l'exÃ©cutable embarque payload.enc,
// jamais payload.json en clair.
package main

import (
	"fmt"
	"os"

	"payload-builder/internal/seal"
)

func main() {
	plain, err := os.ReadFile("payload.json")
	if err != nil {
		fmt.Fprintln(os.Stderr, "seal: payload.json introuvable :", err)
		os.Exit(1)
	}
	sealed, err := seal.Seal(plain)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seal:", err)
		os.Exit(1)
	}
	if err := os.WriteFile("payload.enc", sealed, 0600); err != nil {
		fmt.Fprintln(os.Stderr, "seal: Ã©criture impossible :", err)
		os.Exit(1)
	}
	// ContrÃ´le aller-retour : une config illisible dÃ©sactiverait tout en silence.
	back, err := seal.Open(sealed)
	if err != nil || string(back) != string(plain) {
		fmt.Fprintln(os.Stderr, "seal: vÃ©rification aller-retour Ã©chouÃ©e")
		os.Exit(1)
	}
	fmt.Printf("seal: payload.enc Ã©crit (%d octets, config %d octets chiffrÃ©s)\n",
		len(sealed), len(plain))
}
