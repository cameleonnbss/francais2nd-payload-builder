// Extraction de l'icône d'un exécutable PE vers un .ico.
//
// Le loader du builder a besoin d'une icône crédible : plutôt que d'en
// embarquer une, on emprunte celle d'un binaire système (notepad.exe,
// msinfo32.exe…) et on l'injecte avec rsrc. L'extraction suit le format
// documenté : répertoire de ressources (RVA), RT_GROUP_ICON (14) au niveau 1,
// puis les RT_ICON (3) référencées par le groupe.
//
// Format .ico : ICONDIR (6 octets) + ICONDIRENTRY (16 × n) + images.
package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"fmt"
	"os"
)

const (
	rtIcon      = 3
	rtGroupIcon = 14
)

// resReader lit les ressources d'un PE déjà chargé en mémoire : quelques
// centaines de Ko, la lecture complète est plus simple qu'un jeu d'offsets.
type resReader struct {
	file []byte // fichier entier
	sec  *pe.Section
	base int // offset du répertoire racine dans la section
}

func extractIcon(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	f, err := pe.NewFile(bytes.NewReader(raw))
	if err != nil {
		return nil, fmt.Errorf("PE illisible : %w", err)
	}
	defer f.Close()

	var dirRVA uint32
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		dirRVA = oh.DataDirectory[2].VirtualAddress
	case *pe.OptionalHeader64:
		dirRVA = oh.DataDirectory[2].VirtualAddress
	}
	if dirRVA == 0 {
		return nil, fmt.Errorf("aucune ressource dans %s", path)
	}

	var sec *pe.Section
	for _, s := range f.Sections {
		if dirRVA >= s.VirtualAddress && dirRVA < s.VirtualAddress+s.VirtualSize {
			sec = s
			break
		}
	}
	if sec == nil {
		return nil, fmt.Errorf("section de ressources introuvable")
	}
	r := &resReader{
		file: raw,
		sec:  sec,
		base: int(dirRVA - sec.VirtualAddress),
	}

	g, err := r.firstData(rtGroupIcon, 0)
	if err != nil {
		return nil, fmt.Errorf("RT_GROUP_ICON : %w", err)
	}
	if len(g) < 6 {
		return nil, fmt.Errorf("groupe d'icônes trop court (%d octets)", len(g))
	}
	count := int(binary.LittleEndian.Uint16(g[4:6]))
	if count == 0 || len(g) < 6+14*count {
		return nil, fmt.Errorf("groupe d'icônes incomplet (%d entrées, %d octets)", count, len(g))
	}

	// Chaque entrée du groupe référence une image RT_ICON par identifiant.
	type img struct {
		entry [16]byte
		data  []byte
	}
	icons := make([]img, 0, count)
	for i := 0; i < count; i++ {
		e := g[6+14*i : 6+14*(i+1)]
		id := binary.LittleEndian.Uint16(e[12:14])
		data, err := r.firstData(rtIcon, uint32(id))
		if err != nil || len(data) == 0 {
			continue // groupe partiel : on garde les images présentes
		}
		var en [16]byte
		copy(en[0:12], e[0:12]) // largeur, hauteur, couleurs, plans, bpp, taille
		icons = append(icons, img{entry: en, data: data})
	}
	if len(icons) == 0 {
		return nil, fmt.Errorf("aucune image RT_ICON exploitable")
	}

	// Assemblage du .ico : les offsets d'image sont calculés après le
	// répertoire, dans l'ordre des entrées.
	out := make([]byte, 6+16*len(icons))
	binary.LittleEndian.PutUint16(out[0:2], 0) // réservé
	binary.LittleEndian.PutUint16(out[2:4], 1) // 1 = icône
	binary.LittleEndian.PutUint16(out[4:6], uint16(len(icons)))
	offset := 6 + 16*len(icons)
	for i := range icons {
		copy(out[6+16*i:6+16*(i+1)], icons[i].entry[:])
		binary.LittleEndian.PutUint32(out[6+16*i+8:6+16*i+12], uint32(len(icons[i].data)))
		binary.LittleEndian.PutUint32(out[6+16*i+12:6+16*i+16], uint32(offset))
		offset += len(icons[i].data)
	}
	for _, ic := range icons {
		out = append(out, ic.data...)
	}
	return out, nil
}

/* ------------------------------------------------------------------ */
/* Navigation dans le répertoire de ressources                         */
/* ------------------------------------------------------------------ */

// imgOff convertit un offset relatif au répertoire racine en offset dans le
// fichier complet.
func (r *resReader) imgOff(rel int) (int, error) {
	off := int(r.sec.Offset) + r.base + rel
	if off < 0 || off > len(r.file) {
		return 0, fmt.Errorf("offset de ressource hors fichier (%d)", off)
	}
	return off, nil
}

// rvaOff convertit une RVA en offset fichier via la section des ressources.
func (r *resReader) rvaOff(rva uint32) (int, error) {
	if rva < r.sec.VirtualAddress {
		return 0, fmt.Errorf("RVA 0x%x hors de la section des ressources", rva)
	}
	off := int(r.sec.Offset) + int(rva-r.sec.VirtualAddress)
	if off < 0 || off > len(r.file) {
		return 0, fmt.Errorf("RVA 0x%x hors fichier", rva)
	}
	return off, nil
}

type resEntry struct {
	id    uint32
	named bool
	child int // offset relatif au répertoire racine
}

// entries lit la table d'entrées d'un répertoire situé à l'offset relatif rel.
func (r *resReader) entries(rel int) ([]resEntry, error) {
	off, err := r.imgOff(rel)
	if err != nil {
		return nil, err
	}
	if off+16 > len(r.file) {
		return nil, fmt.Errorf("répertoire tronqué")
	}
	named := int(binary.LittleEndian.Uint16(r.file[off+12 : off+14]))
	ids := int(binary.LittleEndian.Uint16(r.file[off+14 : off+16]))
	total := named + ids
	if total < 0 || total > 4096 {
		return nil, fmt.Errorf("nombre d'entrées invraisemblable (%d)", total)
	}
	if off+16+8*total > len(r.file) {
		return nil, fmt.Errorf("table d'entrées tronquée")
	}
	out := make([]resEntry, 0, total)
	for i := 0; i < total; i++ {
		base := off + 16 + 8*i
		nameField := binary.LittleEndian.Uint32(r.file[base : base+4])
		childField := binary.LittleEndian.Uint32(r.file[base+4 : base+8])
		out = append(out, resEntry{
			id:    nameField & 0x7fffffff,
			named: nameField&0x80000000 != 0,
			child: int(childField & 0x7fffffff),
		})
	}
	return out, nil
}

// childByID rend l'offset du sous-répertoire d'identifiant id. Si id vaut 0,
// on prend la première entrée (identifiant non nommée ou nommée).
func (r *resReader) childByID(rel int, id uint32) (int, error) {
	ents, err := r.entries(rel)
	if err != nil {
		return 0, err
	}
	if id == 0 {
		if len(ents) == 0 {
			return 0, fmt.Errorf("répertoire vide")
		}
		return ents[0].child, nil
	}
	for _, e := range ents {
		if !e.named && e.id == id {
			return e.child, nil
		}
	}
	return 0, fmt.Errorf("entrée %d absente", id)
}

// firstChild rend l'offset de la première entrée d'un répertoire.
func (r *resReader) firstChild(rel int) (int, error) {
	ents, err := r.entries(rel)
	if err != nil {
		return 0, err
	}
	if len(ents) == 0 {
		return 0, fmt.Errorf("répertoire vide")
	}
	return ents[0].child, nil
}

// firstData suit type → nom → langue jusqu'à l'entrée de données et rend les
// octets de la ressource.
func (r *resReader) firstData(typ, nameID uint32) ([]byte, error) {
	typeDir, err := r.childByID(r.base, typ)
	if err != nil {
		return nil, err
	}
	nameDir, err := r.childByID(typeDir, nameID)
	if err != nil {
		return nil, err
	}
	langRel, err := r.firstChild(nameDir)
	if err != nil {
		return nil, err
	}
	off, err := r.imgOff(langRel)
	if err != nil {
		return nil, err
	}
	if off+16 > len(r.file) {
		return nil, fmt.Errorf("entrée de données tronquée")
	}
	dataRVA := binary.LittleEndian.Uint32(r.file[off : off+4])
	size := binary.LittleEndian.Uint32(r.file[off+4 : off+8])
	if size == 0 || size > 16*1024*1024 {
		return nil, fmt.Errorf("taille de ressource invalide (%d)", size)
	}
	dataOff, err := r.rvaOff(dataRVA)
	if err != nil {
		return nil, err
	}
	if dataOff+int(size) > len(r.file) {
		return nil, fmt.Errorf("ressource hors fichier")
	}
	return r.file[dataOff : dataOff+int(size)], nil
}
