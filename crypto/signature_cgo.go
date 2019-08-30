// Copyright 2017 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

// +build !nacl,!js,!nocgo

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"github.com/decred/dcrd/dcrec/secp256k1"
)

// Ecrecover returns the uncompressed public key that created the given signature.
func Ecrecover(hash, sig []byte) ([]byte, error) {
	pub, _, err := secp256k1.RecoverCompact(sig, hash)
	if err != nil {
		return nil, err
	}
	return FromECDSAPub(pub.ToECDSA()), nil
}

// SigToPub returns the public key that created the given signature.
func SigToPub(hash, sig []byte) (*ecdsa.PublicKey, error) {
	s, err := Ecrecover(hash, sig)
	if err != nil {
		return nil, err
	}

	x, y := elliptic.Unmarshal(S256(), s)
	return &ecdsa.PublicKey{Curve: S256(), X: x, Y: y}, nil
}

// Sign calculates an ECDSA signature.
//
// This function is susceptible to chosen plaintext attacks that can leak
// information about the private key that is used for signing. Callers must
// be aware that the given hash cannot be chosen by an adversery. Common
// solution is to hash any input before calculating the signature.
//
// The produced signature is in the [R || S || V] format where V is 0 or 1.
//func Sign(hash []byte, prv *ecdsa.PrivateKey) (sig []byte, err error) {
//	if len(hash) != 32 {
//		return nil, fmt.Errorf("hash is required to be exactly 32 bytes (%d)", len(hash))
//	}
//	seckey := math.PaddedBigBytes(prv.D, prv.Params().BitSize/8)
//	defer zeroBytes(seckey)
//
//	key, _ := secp256k1.PrivKeyFromBytes(seckey)
//	signature, err := key.Sign(hash)
//	if err != nil {
//		return nil, err
//	}
//	return signature.Serialize(), nil
//}

// VerifySignature checks that the given public key created signature over hash.
// The public key should be in compressed (33 bytes) or uncompressed (65 bytes) format.
// The signature should have the 64 byte [R || S] format.
func VerifySignature(pubkey, hash, signature []byte) bool {
	p, err := secp256k1.ParsePubKey(pubkey)
	if err != nil {
		return false
	}
	pub2, _, err := secp256k1.RecoverCompact(signature, hash)
	if err != nil {
		return false
	}

	return p.X.Cmp(pub2.X) == 0 && p.Y.Cmp(pub2.Y) == 0
}

// DecompressPubkey parses a public key in the 33-byte compressed format.
func DecompressPubkey(pubkey []byte) (*ecdsa.PublicKey, error) {
	pubKey, err := secp256k1.ParsePubKey(pubkey)
	if err != nil {
		return nil, err
	}
	pubKey.SerializeCompressed()

	return &ecdsa.PublicKey{X: pubKey.X, Y: pubKey.Y, Curve: S256()}, nil
}

// CompressPubkey encodes a public key to the 33-byte compressed format.
func CompressPubkey(pubkey *ecdsa.PublicKey) []byte {
	return secp256k1.NewPublicKey(pubkey.X, pubkey.Y).SerializeCompressed()
}

// S256_Curve returns an instance of the secp256k1 curve.
func S256() elliptic.Curve {
	return secp256k1.S256()
}
