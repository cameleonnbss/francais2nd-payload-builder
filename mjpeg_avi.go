// mjpeg_avi.go — assemblage d'une vidéo AVI MJPEG en pur Go.
//
// La capture vidéo ne doit dépendre de rien : ni ffmpeg, ni codec installé.
// Une webcam produit du JPEG ; un conteneur AVI avec des images JPEG plein
// cadre (MJPEG) est lu par Windows Media Player, VLC et le lecteur Photos sans
// réencodage. On écrit donc l'en-tête RIFF à la main.
//
// Structure produite :
//
//	RIFF 'AVI '
//	  LIST 'hdrl' : avih + LIST 'strl' (strh + strf)
//	  LIST 'movi' : n × ('00dc' + JPEG + padding)
//	  'idx1'      : n × 16 octets
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
)

type aviWriter struct {
	width, height int
	fps           int
	frames        [][]byte
	maxFrames     int
}

func newAVIWriter(w, h, fps, maxFrames int) *aviWriter {
	if fps < 1 {
		fps = 1
	}
	if fps > 30 {
		fps = 30
	}
	return &aviWriter{width: w, height: h, fps: fps, maxFrames: maxFrames}
}

// add ajoute une image JPEG (ignorée au-delà du plafond).
func (a *aviWriter) add(jpeg []byte) bool {
	if len(jpeg) == 0 || len(a.frames) >= a.maxFrames {
		return false
	}
	a.frames = append(a.frames, jpeg)
	return true
}

func (a *aviWriter) count() int { return len(a.frames) }

// build assemble le fichier AVI complet.
func (a *aviWriter) build() ([]byte, error) {
	n := len(a.frames)
	if n == 0 {
		return nil, fmt.Errorf("aucune image")
	}

	var moviLen, maxFrame int
	for _, f := range a.frames {
		moviLen += 8 + len(f)
		if len(f)%2 == 1 {
			moviLen++ // padding des blocs RIFF
		}
		if len(f) > maxFrame {
			maxFrame = len(f)
		}
	}
	const (
		avihSize = 56
		strhSize = 56
		strfSize = 40
	)
	strlSize := 4 + 8 + strhSize + 8 + strfSize // 'strl' + strh + strf
	hdrlSize := 4 + 8 + avihSize + 8 + strlSize // 'hdrl' + avih + LIST strl
	idxLen := 16 * n
	// 'AVI ' + LIST hdrl + LIST movi + idx1
	riffSize := 4 + (8 + hdrlSize) + (8 + moviLen) + (8 + idxLen)

	var b bytes.Buffer
	u32 := func(v uint32) { _ = binary.Write(&b, binary.LittleEndian, v) }
	i32 := func(v int32) { _ = binary.Write(&b, binary.LittleEndian, v) }
	u16 := func(v uint16) { _ = binary.Write(&b, binary.LittleEndian, v) }
	i16 := func(v int16) { _ = binary.Write(&b, binary.LittleEndian, v) }

	b.WriteString("RIFF")
	u32(uint32(riffSize))
	b.WriteString("AVI ")

	/* --- LIST hdrl --- */
	b.WriteString("LIST")
	u32(uint32(hdrlSize))
	b.WriteString("hdrl")

	b.WriteString("avih")
	u32(avihSize)
	u32(uint32(1000000 / a.fps)) // µs par image
	u32(uint32(maxFrame * a.fps))
	u32(0)    // padding
	u32(0x10) // AVIF_HASINDEX
	u32(uint32(n))
	u32(0) // images initiales
	u32(1) // flux
	u32(uint32(maxFrame))
	u32(uint32(a.width))
	u32(uint32(a.height))
	for i := 0; i < 4; i++ {
		u32(0)
	}

	/* --- LIST strl --- */
	b.WriteString("LIST")
	u32(uint32(strlSize))
	b.WriteString("strl")

	b.WriteString("strh")
	u32(strhSize)
	b.WriteString("vids")
	b.WriteString("MJPG")
	u32(0) // flags
	u16(0) // priorité
	u16(0) // langue
	u32(0) // images initiales
	u32(1) // échelle
	u32(uint32(a.fps))
	u32(0) // début
	u32(uint32(n))
	u32(uint32(maxFrame))
	i32(-1) // qualité par défaut
	u32(0)  // taille d'échantillon
	i16(0)
	i16(0)
	i16(int16(a.width))
	i16(int16(a.height))

	b.WriteString("strf")
	u32(strfSize)
	u32(40) // biSize
	i32(int32(a.width))
	i32(int32(a.height))
	u16(1) // biPlanes
	u16(24)
	b.WriteString("MJPG") // biCompression
	u32(uint32(a.width * a.height * 3))
	i32(0)
	i32(0)
	u32(0)
	u32(0)

	/* --- LIST movi --- */
	b.WriteString("LIST")
	u32(uint32(4 + moviLen))
	b.WriteString("movi")
	// Les offsets d'idx1 se comptent depuis le fourcc 'movi' : la première
	// image a donc l'offset 4 (juste après le fourcc).
	moviRef := b.Len() - 4

	type idxEntry struct {
		offset uint32
		size   uint32
	}
	idx := make([]idxEntry, 0, n)
	for _, f := range a.frames {
		chunkPos := b.Len()
		idx = append(idx, idxEntry{
			offset: uint32(chunkPos - moviRef),
			size:   uint32(len(f)),
		})
		b.WriteString("00dc")
		u32(uint32(len(f)))
		b.Write(f)
		if len(f)%2 == 1 {
			b.WriteByte(0)
		}
	}

	/* --- idx1 --- */
	b.WriteString("idx1")
	u32(uint32(idxLen))
	for _, e := range idx {
		b.WriteString("00dc")
		u32(0x10) // AVIIF_KEYFRAME
		u32(e.offset)
		u32(e.size)
	}
	return b.Bytes(), nil
}
