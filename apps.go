// apps.go — inventaire des applications, ouverture et fermeture.
//
// La liste vient de PowerShell (`Get-Process`) plutôt que de `tasklist` : seul
// Get-Process expose le chemin de l'exécutable et le titre de fenêtre, ce qui
// permet de reconnaître l'application derrière un nom de processus.
package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

type procInfo struct {
	PID   int    `json:"Id"`
	Name  string `json:"ProcessName"`
	Path  string `json:"Path"`
	Title string `json:"MainWindowTitle"`
}

// appsScript rend du JSON compact : identifiant, nom, chemin, titre.
const appsScript = `Get-Process | Where-Object { $_.Path -or $_.MainWindowTitle } | ` +
	`Select-Object Id,ProcessName,Path,MainWindowTitle | ConvertTo-Json -Compress -Depth 2`

// listApps rend un tableau lisible des processus visibles, filtrable par
// sous-chaîne (nom, chemin ou titre).
func listApps(filter string) string {
	out, err := runPSUTF8(appsScript)
	if err != nil && strings.TrimSpace(out) == "" {
		return "erreur : " + err.Error()
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return "aucun processus lisible (droits insuffisants sur cet utilisateur ?)."
	}
	// ConvertTo-Json rend un objet seul quand il n'y a qu'une entrée.
	if strings.HasPrefix(out, "{") {
		out = "[" + out + "]"
	}
	var list []procInfo
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		return "lecture impossible : " + err.Error()
	}
	f := strings.ToLower(strings.TrimSpace(filter))
	var b strings.Builder
	b.WriteString("```\n PID   processus                 chemin\n")
	n := 0
	for _, p := range list {
		blob := strings.ToLower(p.Name + " " + p.Path + " " + p.Title)
		if f != "" && !strings.Contains(blob, f) {
			continue
		}
		path := p.Path
		if path == "" {
			path = "(" + p.Title + ")"
		}
		if len(path) > 70 {
			path = "…" + path[len(path)-69:]
		}
		fmt.Fprintf(&b, "%5d  %-24s %s\n", p.PID, truncStr(p.Name, 24), path)
		n++
		if n >= 60 {
			b.WriteString("… (60 premières lignes)\n")
			break
		}
	}
	b.WriteString("```")
	if n == 0 {
		return "aucune application ne correspond à `" + filter + "`."
	}
	return b.String()
}

// truncStr coupe proprement un libellé trop long.
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// openTarget ouvre un fichier, un dossier ou une URL avec l'association
// Windows par défaut, sans fenêtre de console.
func openTarget(target string) string {
	if strings.TrimSpace(target) == "" {
		return "usage : !open <fichier|dossier|url>"
	}
	ps := "Start-Process -FilePath " + qPS(target)
	if _, err := runPSUTF8(ps); err != nil {
		return "ouverture impossible : " + err.Error()
	}
	return "ouvert : " + target
}

// killTarget ferme un processus par identifiant ou par nom.
func killTarget(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "usage : !kill <pid|nom.exe>"
	}
	var ps string
	if pid, err := strconv.Atoi(arg); err == nil {
		ps = fmt.Sprintf("Stop-Process -Id %d -Force", pid)
		arg = "PID " + arg
	} else {
		ps = "Stop-Process -Name " + qPS(strings.TrimSuffix(arg, ".exe")) + " -Force"
	}
	if _, err := runPSUTF8(ps); err != nil {
		return "fermeture impossible (" + arg + ") : " + err.Error()
	}
	return "fermé : " + arg
}

// closeWindow ferme la fenêtre principale d'un processus sans le tuer : le
// programme peut enregistrer son travail, comme un clic sur la croix.
func closeWindow(arg string) string {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return "usage : !close <pid|nom>"
	}
	var ps string
	if pid, err := strconv.Atoi(arg); err == nil {
		ps = fmt.Sprintf("(Get-Process -Id %d).CloseMainWindow() | Out-Null", pid)
	} else {
		ps = "(Get-Process -Name " + qPS(strings.TrimSuffix(arg, ".exe")) +
			").CloseMainWindow() | Out-Null"
	}
	if _, err := runPSUTF8(ps); err != nil {
		return "fermeture impossible : " + err.Error()
	}
	return "fenêtre fermée : " + arg
}
