package ckks_fv

import (
	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/utils"
)

// FVEncryptor in an interface for encryptors
//
// encrypt with pk : ciphertext = [pk[0]*u + m + e_0, pk[1]*u + e_1]
// encrypt with sk : ciphertext = [-a*sk + m + e, a]
type FVEncryptor interface {
	// EncryptNew encrypts the input plaintext using the stored key and returns
	// the result on a newly created ciphertext. The encryption is done by first
	// encrypting zero in QP, dividing by P and then adding the plaintext.
	EncryptNew(plaintext *Plaintext) *Ciphertext

	// Encrypt encrypts the input plaintext using the stored key, and returns
	// the result on the receiver ciphertext. The encryption is done by first
	// encrypting zero in QP, dividing by P and then adding the plaintext.
	Encrypt(plaintext *Plaintext, ciphertext *Ciphertext)

	// EncryptFastNew encrypts the input plaintext using the stored key and returns
	// the result on a newly created ciphertext. The encryption is done by first
	// encrypting zero in Q and then adding the plaintext.
	EncryptFastNew(plaintext *Plaintext) *Ciphertext

	// EncryptFast encrypts the input plaintext using the stored-key, and returns
	// the result on the receiver ciphertext. The encryption is done by first
	// encrypting zero in Q and then adding the plaintext.
	EncryptFast(plaintext *Plaintext, ciphertext *Ciphertext)

	// EncryptFromCRPNew encrypts the input plaintext using the stored key and returns
	// the result on a newly created ciphertext. The encryption is done by first encrypting
	// zero in QP, using the provided polynomial as the uniform polynomial, dividing by P and
	// then adding the plaintext.
	EncryptFromCRPNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext

	// EncryptFromCRP encrypts the input plaintext using the stored key and returns
	// the result tge receiver ciphertext. The encryption is done by first encrypting
	// zero in QP, using the provided polynomial as the uniform polynomial, dividing by P and
	// then adding the plaintext.
	EncryptFromCRP(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly)

	// EncryptFromCRPNew encrypts the input plaintext using the stored key and returns
	// the result on a newly created ciphertext. The encryption is done by first encrypting
	// zero in Q, using the provided polynomial as the uniform polynomial, and
	// then adding the plaintext.
	EncryptFromCRPFastNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext

	// EncryptFromCRP encrypts the input plaintext using the stored key and returns
	// the result tge receiver ciphertext. The encryption is done by first encrypting
	// zero in Q, using the provided polynomial as the uniform polynomial, and
	// then adding the plaintext.
	EncryptFromCRPFast(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly)
}

// fvEncryptor is a structure that holds the parameters needed to encrypt plaintexts.
type fvEncryptor struct {
	params   *Parameters
	ringQ    *ring.Ring
	ringQP   *ring.Ring
	polypool [3]*ring.Poly

	baseconverter              *ring.FastBasisExtender
	gaussianSamplerQ           *ring.GaussianSampler
	uniformSamplerQ            *ring.UniformSampler
	ternarySamplerMontgomeryQ  *ring.TernarySampler
	gaussianSamplerQP          *ring.GaussianSampler
	uniformSamplerQP           *ring.UniformSampler
	ternarySamplerMontgomeryQP *ring.TernarySampler
}

type pkFVEncryptor struct {
	fvEncryptor
	pk *PublicKey
}

type skFVEncryptor struct {
	fvEncryptor
	sk *SecretKey
}

// NewFVEncryptorFromPk creates a new Encryptor with the provided public-key.
// This encryptor can be used to encrypt plaintexts, using the stored key.
func NewFVEncryptorFromPk(params *Parameters, pk *PublicKey) FVEncryptor {
	return &pkFVEncryptor{newFVEncryptor(params), pk}
}

// NewFVEncryptorFromSk creates a new Encryptor with the provided secret-key.
// This encryptor can be used to encrypt plaintexts, using the stored key.
func NewFVEncryptorFromSk(params *Parameters, sk *SecretKey) FVEncryptor {
	return &skFVEncryptor{newFVEncryptor(params), sk}
}

func newFVEncryptor(params *Parameters) fvEncryptor {

	var ringQ, ringQP *ring.Ring
	var err error

	if ringQ, err = ring.NewRing(params.N(), params.qi); err != nil {
		panic(err)
	}

	if ringQP, err = ring.NewRing(params.N(), append(params.qi, params.pi...)); err != nil {
		panic(err)
	}

	var baseconverter *ring.FastBasisExtender
	if len(params.pi) != 0 {
		var ringP *ring.Ring
		if ringP, err = ring.NewRing(params.N(), params.pi); err != nil {
			panic(err)
		}
		baseconverter = ring.NewFastBasisExtender(ringQ, ringP)
	}

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}

	return fvEncryptor{
		params:                     params.Copy(),
		ringQ:                      ringQ,
		ringQP:                     ringQP,
		polypool:                   [3]*ring.Poly{ringQP.NewPoly(), ringQP.NewPoly(), ringQP.NewPoly()},
		baseconverter:              baseconverter,
		gaussianSamplerQ:           ring.NewGaussianSampler(prng),
		uniformSamplerQ:            ring.NewUniformSampler(prng, ringQ),
		ternarySamplerMontgomeryQ:  ring.NewTernarySampler(prng, ringQ, 0.5, true),
		gaussianSamplerQP:          ring.NewGaussianSampler(prng),
		uniformSamplerQP:           ring.NewUniformSampler(prng, ringQP),
		ternarySamplerMontgomeryQP: ring.NewTernarySampler(prng, ringQP, 0.5, true),
	}
}

func (encryptor *pkFVEncryptor) EncryptNew(plaintext *Plaintext) *Ciphertext {
	ciphertext := NewCiphertextFV(encryptor.params, 1)
	encryptor.encrypt(plaintext, ciphertext, false)
	return ciphertext
}

func (encryptor *pkFVEncryptor) Encrypt(plaintext *Plaintext, ciphertext *Ciphertext) {

	if encryptor.baseconverter == nil {
		panic("Cannot Encrypt : modulus P is empty -> use instead EncryptFast")
	}

	encryptor.encrypt(plaintext, ciphertext, false)
}

func (encryptor *pkFVEncryptor) EncryptFastNew(plaintext *Plaintext) *Ciphertext {
	ciphertext := NewCiphertextFV(encryptor.params, 1)
	encryptor.encrypt(plaintext, ciphertext, true)

	return ciphertext
}

func (encryptor *pkFVEncryptor) EncryptFast(plaintext *Plaintext, ciphertext *Ciphertext) {
	encryptor.encrypt(plaintext, ciphertext, true)
}

func (encryptor *pkFVEncryptor) EncryptFromCRP(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {
	panic("Cannot encrypt with CRP using an encryptor created with the public-key")
}

func (encryptor *pkFVEncryptor) EncryptFromCRPNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext {
	panic("Cannot encrypt with CRP using an encryptor created with the public-key")
}

func (encryptor *pkFVEncryptor) EncryptFromCRPFast(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {
	panic("Cannot encrypt with CRP using an encryptor created with the public-key")
}

func (encryptor *pkFVEncryptor) EncryptFromCRPFastNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext {
	panic("Cannot encrypt with CRP using an encryptor created with the public-key")
}

func (encryptor *pkFVEncryptor) encrypt(p *Plaintext, ciphertext *Ciphertext, fast bool) {

	ringQ := encryptor.ringQ

	if fast {

		encryptor.ternarySamplerMontgomeryQ.Read(encryptor.polypool[2])
		ringQ.NTTLazy(encryptor.polypool[2], encryptor.polypool[2])

		ringQ.MulCoeffsMontgomery(encryptor.polypool[2], encryptor.pk.Value[0], encryptor.polypool[0])
		ringQ.MulCoeffsMontgomery(encryptor.polypool[2], encryptor.pk.Value[1], encryptor.polypool[1])

		ringQ.InvNTT(encryptor.polypool[0], ciphertext.value[0])
		ringQ.InvNTT(encryptor.polypool[1], ciphertext.value[1])

		// ct[0] = pk[0]*u + e0
		encryptor.gaussianSamplerQ.ReadAndAdd(ciphertext.value[0], ringQ, encryptor.params.Sigma(), int(6*encryptor.params.Sigma()))

		// ct[1] = pk[1]*u + e1
		encryptor.gaussianSamplerQ.ReadAndAdd(ciphertext.value[1], ringQ, encryptor.params.Sigma(), int(6*encryptor.params.Sigma()))

	} else {

		ringQP := encryptor.ringQP

		// u
		encryptor.ternarySamplerMontgomeryQP.Read(encryptor.polypool[2])
		ringQP.NTTLazy(encryptor.polypool[2], encryptor.polypool[2])

		// ct[0] = pk[0]*u
		// ct[1] = pk[1]*u
		ringQP.MulCoeffsMontgomery(encryptor.polypool[2], encryptor.pk.Value[0], encryptor.polypool[0])
		ringQP.MulCoeffsMontgomery(encryptor.polypool[2], encryptor.pk.Value[1], encryptor.polypool[1])

		ringQP.InvNTTLazy(encryptor.polypool[0], encryptor.polypool[0])
		ringQP.InvNTTLazy(encryptor.polypool[1], encryptor.polypool[1])

		// ct[0] = pk[0]*u + e0
		encryptor.gaussianSamplerQP.ReadAndAdd(encryptor.polypool[0], ringQP, encryptor.params.Sigma(), int(6*encryptor.params.Sigma()))

		// ct[1] = pk[1]*u + e1
		encryptor.gaussianSamplerQP.ReadAndAdd(encryptor.polypool[1], ringQP, encryptor.params.Sigma(), int(6*encryptor.params.Sigma()))

		// We rescale the encryption of zero by the special prime, dividing the error by this prime
		encryptor.baseconverter.ModDownPQ(len(ringQ.Modulus)-1, encryptor.polypool[0], ciphertext.value[0])
		encryptor.baseconverter.ModDownPQ(len(ringQ.Modulus)-1, encryptor.polypool[1], ciphertext.value[1])
	}
	// ct[0] = pk[0]*u + e0 + m
	// ct[1] = pk[1]*u + e1
	encryptor.ringQ.Add(ciphertext.value[0], p.value, ciphertext.value[0])
}

func (encryptor *skFVEncryptor) EncryptNew(plaintext *Plaintext) *Ciphertext {
	ciphertext := NewCiphertextFV(encryptor.params, 1)
	encryptor.Encrypt(plaintext, ciphertext)
	return ciphertext
}

func (encryptor *skFVEncryptor) Encrypt(plaintext *Plaintext, ciphertext *Ciphertext) {
	encryptor.encryptSample(plaintext, ciphertext)
}

func (encryptor *skFVEncryptor) EncryptFastNew(plaintext *Plaintext) *Ciphertext {
	panic("Cannot EncryptFastNew: not supported by sk encryptor -> use EncryptFastNew instead")
}

func (encryptor *skFVEncryptor) EncryptFast(plaintext *Plaintext, ciphertext *Ciphertext) {
	panic("Cannot EncryptFast: not supported by sk encryptor -> use Encrypt instead")
}

func (encryptor *skFVEncryptor) EncryptFromCRPNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext {
	ciphertext := NewCiphertextFV(encryptor.params, 1)
	encryptor.EncryptFromCRP(plaintext, ciphertext, crp)
	return ciphertext
}

func (encryptor *skFVEncryptor) EncryptFromCRP(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {
	encryptor.encryptFromCRP(plaintext, ciphertext, crp)
}

func (encryptor *skFVEncryptor) EncryptFromCRPFastNew(plaintext *Plaintext, crp *ring.Poly) *Ciphertext {
	panic("Cannot EncryptFromCRPFastNew: not supported by sk encryptor -> use EncryptFromCRPNew instead")
}

func (encryptor *skFVEncryptor) EncryptFromCRPFast(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {
	panic("Cannot EncryptFromCRPFast: not supported by sk encryptor -> use EncryptFromCRP instead")
}

func (encryptor *skFVEncryptor) encryptSample(plaintext *Plaintext, ciphertext *Ciphertext) {
	encryptor.uniformSamplerQ.Read(encryptor.polypool[1])
	encryptor.encrypt(plaintext, ciphertext, encryptor.polypool[1])
}

func (encryptor *skFVEncryptor) encryptFromCRP(plaintext *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {
	encryptor.ringQ.Copy(crp, encryptor.polypool[1])
	encryptor.encrypt(plaintext, ciphertext, encryptor.polypool[1])
}

func (encryptor *skFVEncryptor) encrypt(p *Plaintext, ciphertext *Ciphertext, crp *ring.Poly) {

	ringQ := encryptor.ringQ

	ringQ.MulCoeffsMontgomery(crp, encryptor.sk.Value, ciphertext.value[0])
	ringQ.Neg(ciphertext.value[0], ciphertext.value[0])

	ringQ.InvNTT(ciphertext.value[0], ciphertext.value[0])
	ringQ.InvNTT(crp, ciphertext.value[1])

	encryptor.gaussianSamplerQ.ReadAndAdd(ciphertext.value[0], ringQ, encryptor.params.Sigma(), int(6*encryptor.params.Sigma()))

	// ct = [-a*s + m + e , a]
	encryptor.ringQ.Add(ciphertext.value[0], p.value, ciphertext.value[0])
}
