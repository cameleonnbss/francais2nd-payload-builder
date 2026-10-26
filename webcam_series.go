// webcam_series.go — capture d'une série d'images en un seul processus.
//
// Le script « une photo » initialise la caméra à chaque exécution (3 à 10 s
// de latence) : assembler une vidéo en appelant cette fonction N fois ne
// produisait qu'une image toutes les quelques secondes. Ici le script ouvre la
// caméra une fois et capture N images à intervalle fixe, ce qui donne une
// vraie cadence.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// singleCaptureBlock est le bloc du script unitaire remplacé par la boucle.
// Il doit correspondre exactement à webcam.go (le script est la source) :
// toute divergence fait échouer la substitution, jamais la capture.
const singleCaptureBlock = `  foreach ($d in $devices) {
    $name = "wmc-" + [Guid]::NewGuid().ToString('N').Substring(0,8) + ".jpg"
    try {
      if ($cfop.GetParameters().Count -eq 2) { $sf = AwaitOp ($cfop.Invoke($folder, @($name, 0))) $fileT }
      else { $sf = AwaitOp ($cfop.Invoke($folder, @($name))) $fileT }
      $init = [Activator]::CreateInstance($initT)
      $initT.GetProperty('VideoDeviceId').SetValue($init, $d.Id)
      $sm = $initT.GetProperty('StreamingCaptureMode')
      if ($sm) { $sm.SetValue($init, [System.Enum]::Parse($sm.PropertyType, 'Video')) }
      $pcs = $initT.GetProperty('PhotoCaptureSource')
      if ($pcs) { $pcs.SetValue($init, [System.Enum]::Parse($pcs.PropertyType, 'VideoPreview')) }
      $cap = [Activator]::CreateInstance($capT)
      AwaitAction ($cap.InitializeAsync($init))
      AwaitAction ($shot.Invoke($cap, @($props, $sf)))
      try { $cap.Dispose() } catch { }
      $len = (Get-Item $sf.Path).Length
      if ($len -gt 1024) {
        Copy-Item $sf.Path $Out -Force
        Remove-Item $sf.Path -Force -ErrorAction SilentlyContinue
        Write-Output ("OK " + $len)
        exit 0
      }
      $trash += $sf.Path
    } catch {
      $trash += $name
      continue
    }
  }
  Write-Output 'FAIL aucune image'
`

// seriesCaptureBlock ouvre la caméra une seule fois et capture en boucle.
const seriesCaptureBlock = `  $captured = 0
  foreach ($d in $devices) {
    try {
      $init = [Activator]::CreateInstance($initT)
      $initT.GetProperty('VideoDeviceId').SetValue($init, $d.Id)
      $sm = $initT.GetProperty('StreamingCaptureMode')
      if ($sm) { $sm.SetValue($init, [System.Enum]::Parse($sm.PropertyType, 'Video')) }
      $pcs = $initT.GetProperty('PhotoCaptureSource')
      if ($pcs) { $pcs.SetValue($init, [System.Enum]::Parse($pcs.PropertyType, 'VideoPreview')) }
      $cap = [Activator]::CreateInstance($capT)
      AwaitAction ($cap.InitializeAsync($init))
      for ($i = 0; $i -lt $Count; $i++) {
        $name = "wsc-" + [Guid]::NewGuid().ToString('N').Substring(0,8) + ".jpg"
        try {
          if ($cfop.GetParameters().Count -eq 2) { $sf = AwaitOp ($cfop.Invoke($folder, @($name, 0))) $fileT }
          else { $sf = AwaitOp ($cfop.Invoke($folder, @($name))) $fileT }
          AwaitAction ($shot.Invoke($cap, @($props, $sf)))
          if ((Get-Item $sf.Path).Length -gt 1024) {
            Copy-Item $sf.Path (Join-Path $OutDir ("frame-{0:d4}.jpg" -f $i)) -Force
            $captured++
          }
          Remove-Item $sf.Path -Force -ErrorAction SilentlyContinue
        } catch { }
        Start-Sleep -Milliseconds $Delay
      }
      try { $cap.Dispose() } catch { }
      if ($captured -gt 0) { Write-Output ("OK " + $captured); exit 0 }
    } catch {
      continue
    }
  }
  Write-Output 'FAIL aucune image'
`

// webcamCaptureSeries capture count images dans outDir, une toutes les
// delayMs millisecondes. Rend le nombre d'images réellement écrites.
func webcamCaptureSeries(outDir string, count, delayMs int) (int, error) {
	webcamMu.Lock()
	defer webcamMu.Unlock()

	if err := os.MkdirAll(outDir, 0700); err != nil {
		return 0, err
	}
	script := strings.Replace(webcamScript, "param([string]$Out)",
		"param([string]$OutDir, [int]$Count, [int]$Delay)", 1)
	script = strings.Replace(script, singleCaptureBlock, seriesCaptureBlock, 1)
	header := "param([string]$OutDir, [int]$Count, [int]$Delay)\r\n" +
		"$OutDir = '" + strings.ReplaceAll(outDir, "'", "''") + "'\r\n" +
		"$Count = " + strconv.Itoa(count) + "\r\n" +
		"$Delay = " + strconv.Itoa(delayMs)
	script = strings.Replace(script,
		"param([string]$OutDir, [int]$Count, [int]$Delay)", header, 1)

	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
		"-WindowStyle", "Hidden", "-EncodedCommand", psEncoded(script))
	cmd.SysProcAttr = hiddenAttr()
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	_ = cmd.Run()

	files, _ := os.ReadDir(outDir)
	n := 0
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "frame-") {
			n++
		}
	}
	if n == 0 {
		return 0, fmt.Errorf("aucune image (%s)", truncate(strings.TrimSpace(buf.String()), 160))
	}
	return n, nil
}

// seriesFrames relit les images capturées, dans l'ordre.
func seriesFrames(dir string) [][]byte {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), "frame-") {
			names = append(names, f.Name())
		}
	}
	// ReadDir trie déjà par nom : frame-0000, frame-0001…
	var out [][]byte
	for _, n := range names {
		if b, err := os.ReadFile(filepath.Join(dir, n)); err == nil && len(b) > 1024 {
			out = append(out, b)
		}
	}
	return out
}
