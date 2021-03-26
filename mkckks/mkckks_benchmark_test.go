package mkckks

import (
	"fmt"
	"testing"

	"github.com/ldsec/lattigo/v2/ckks"
	"github.com/ldsec/lattigo/v2/ring"
)

func testString(opname string, parties uint64, params *ckks.Parameters) string {
	return fmt.Sprintf("%sparties=%d/logN=%d/logQ=%d/levels=%d/alpha=%d/beta=%d",
		opname,
		parties,
		params.LogN(),
		params.LogQP(),
		params.MaxLevel()+1,
		params.Alpha(),
		params.Beta())
}

func BenchmarkMKCKKS(b *testing.B) {

	for _, p := range ckks.DefaultParams {
		benchKeyGen(b, p)
		benchAddTwoCiphertexts(b, p)
		benchEncrypt(b, p)
		benchDecrypt(b, p)
		benchPartialDecrypt(b, p)
		/*
			benchMultTwoCiphertexts(b, i)
			benchRelinCiphertext(b, i)*/

	}
}

func benchKeyGen(b *testing.B, params *ckks.Parameters) {

	crs := GenCommonPublicParam(params)

	b.Run(testString("KeyGen/", 1, params), func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			KeyGen(params, crs)
		}
	})
}

func benchAddTwoCiphertexts(b *testing.B, params *ckks.Parameters) {

	participants := setupPeers(2, params, 6.0)

	value1 := newTestValue(params, complex(-1, -1), complex(1, 1))
	value2 := newTestValue(params, complex(-1, -1), complex(1, 1))

	cipher1 := participants[0].Encrypt(value1)
	cipher2 := participants[1].Encrypt(value2)

	evaluator := NewMKEvaluator(params)

	b.Run(testString("Add/", 2, params), func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			out1, out2 := PadCiphers(cipher1, cipher2, params)
			evaluator.Add(out1, out2)
		}
	})
}

func benchEncrypt(b *testing.B, params *ckks.Parameters) {

	participants := setupPeers(1, params, 6.0)

	value1 := newTestValue(params, complex(-1, -1), complex(1, 1))

	b.Run(testString("Encrypt/", 2, params), func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			participants[0].Encrypt(value1)
		}
	})
}

func benchDecrypt(b *testing.B, params *ckks.Parameters) {

	participants := setupPeers(1, params, 6.0)

	value1 := newTestValue(params, complex(-1, -1), complex(1, 1))

	cipher1 := participants[0].Encrypt(value1)
	partialDec := participants[0].GetPartialDecryption(cipher1)

	b.Run(testString("Decrypt/", 1, params), func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			participants[0].Decrypt(cipher1, []*ring.Poly{partialDec})
		}
	})
}

func benchPartialDecrypt(b *testing.B, params *ckks.Parameters) {

	participants := setupPeers(1, params, 6.0)

	value1 := newTestValue(params, complex(-1, -1), complex(1, 1))
	cipher1 := participants[0].Encrypt(value1)

	b.Run(testString("Partial decryption/", 1, params), func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			participants[0].GetPartialDecryption(cipher1)
		}
	})
}
