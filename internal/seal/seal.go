// Chiffrement AES-256-GCM de la configuration du payload.
//
// La config n'est plus embarquée en clair dans l'exécutable : payload.enc
// contient nonce(12) || chiffré. La clé, fixe et de 32 octets, n'a d'effet
// qu'associée à ce code ; la compilation la passe par garble (-literals) qui
// transforme les littéraux en routine de déchiffrement.
package seal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"io"
)

var key = []byte{
	0x8c, 0x2f, 0x51, 0xa7, 0x13, 0xd9, 0x64, 0x0e,
	0xb5, 0x47, 0x9a, 0x2c, 0xf1, 0x68, 0x3d, 0x70,
	0xe4, 0x09, 0xba, 0x56, 0xc3, 0x1f, 0x88, 0x25,
	0x7d, 0xaf, 0x31, 0x6e, 0x04, 0xd2, 0x59, 0xb8,
}

// Key renvoie la clé de chiffrement. cmd/cover l'injecte dans le lanceur
// camouflé, qui déchiffre sans importer ce package (le lanceur est compilé
// comme module autonome, hors de l'arbre internal).
func Key() []byte {
	out := make([]byte, len(key))
	copy(out, key)
	return out
}

// Seal chiffre un contenu, renvoyant nonce||chiffré.
func Seal(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

// Open déchiffre ce que Seal a produit.
func Open(data []byte) ([]byte, error) {
	if len(data) < 12+16 {
		return nil, errors.New("contenu chiffré trop court")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	return gcm.Open(nil, data[:ns], data[ns:], nil)
}
