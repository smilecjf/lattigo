package dbfv

import (
	"github.com/ldsec/lattigo/bfv"
	"github.com/ldsec/lattigo/ring"
	//"fmt"
)

type RefreshProtocol struct {
	bfvContext    *bfv.BfvContext
	tmp1          *ring.Poly
	tmp2          *ring.Poly
	hP            *ring.Poly
	baseconverter *ring.FastBasisExtender
}

type RefreshShareDecrypt *ring.Poly
type RefreshShareRecrypt *ring.Poly
type RefreshShare struct {
	RefreshShareDecrypt RefreshShareDecrypt
	RefreshShareRecrypt RefreshShareRecrypt
}

func NewRefreshProtocol(bfvContext *bfv.BfvContext) (refreshProtocol *RefreshProtocol) {
	refreshProtocol = new(RefreshProtocol)
	refreshProtocol.bfvContext = bfvContext
	refreshProtocol.tmp1 = bfvContext.ContextKeys().NewPoly()
	refreshProtocol.tmp2 = bfvContext.ContextKeys().NewPoly()
	refreshProtocol.hP = bfvContext.ContextPKeys().NewPoly()

	refreshProtocol.baseconverter = ring.NewFastBasisExtender(bfvContext.ContextQ().Modulus, bfvContext.KeySwitchPrimes())

	return
}

func (rfp *RefreshProtocol) AllocateShares() RefreshShare {
	return RefreshShare{rfp.bfvContext.ContextQ().NewPoly(),
		rfp.bfvContext.ContextQ().NewPoly()}
}

func (rfp *RefreshProtocol) GenShares(sk *ring.Poly, ciphertext *bfv.Ciphertext, crs *ring.Poly, share RefreshShare) {

	level := uint64(len(ciphertext.Value()[1].Coeffs) - 1)

	contextQ := rfp.bfvContext.ContextQ()
	contextT := rfp.bfvContext.ContextT()
	contextKeys := rfp.bfvContext.ContextKeys()
	contextP := rfp.bfvContext.ContextPKeys()
	sampler := rfp.bfvContext.ContextKeys().NewKYSampler(3.19, 19) // TODO : add smudging noise

	// h0 = s*ct[1]
	contextQ.NTT(ciphertext.Value()[1], rfp.tmp1)
	contextQ.MulCoeffsMontgomeryAndAdd(sk, rfp.tmp1, share.RefreshShareDecrypt)

	contextQ.InvNTT(share.RefreshShareDecrypt, share.RefreshShareDecrypt)

	// h0 = s*ct[1]*P
	for _, pj := range rfp.bfvContext.KeySwitchPrimes() {
		contextQ.MulScalar(share.RefreshShareDecrypt, pj, share.RefreshShareDecrypt)
	}

	// h0 = s*ct[1]*P + e
	sampler.Sample(rfp.tmp1)
	contextQ.Add(share.RefreshShareDecrypt, rfp.tmp1, share.RefreshShareDecrypt)

	for x, i := 0, uint64(len(contextQ.Modulus)); i < uint64(len(rfp.bfvContext.ContextKeys().Modulus)); x, i = x+1, i+1 {
		for j := uint64(0); j < contextQ.N; j++ {
			rfp.hP.Coeffs[x][j] += rfp.tmp1.Coeffs[i][j]
		}
	}

	// h0 = (s*ct[1]*P + e)/P
	rfp.baseconverter.ModDownSplited(contextQ, contextP, rfp.bfvContext.RescaleParamsKeys(), level, share.RefreshShareDecrypt, rfp.hP, share.RefreshShareDecrypt, rfp.tmp1)

	// h1 = -s*a
	contextKeys.NTT(crs, rfp.tmp1)
	contextKeys.MulCoeffsMontgomeryAndSub(sk, rfp.tmp1, rfp.tmp2)
	contextKeys.InvNTT(rfp.tmp2, rfp.tmp2)

	// h1 = s*a + e'
	sampler.SampleAndAdd(rfp.tmp2)

	// h1 = (-s*a + e')/P
	rfp.baseconverter.ModDown(contextKeys, rfp.bfvContext.RescaleParamsKeys(), level, rfp.tmp2, share.RefreshShareRecrypt, rfp.tmp1)

	// mask = (uniform plaintext in [0, T-1]) * floor(Q/T)
	coeffs := contextT.NewUniformPoly()
	lift(coeffs, rfp.tmp1, rfp.bfvContext)

	// h0 = (s*ct[1]*P + e)/P + mask
	contextQ.Add(share.RefreshShareDecrypt, rfp.tmp1, share.RefreshShareDecrypt)

	// h1 = (-s*a + e')/P - mask
	contextQ.Sub(share.RefreshShareRecrypt, rfp.tmp1, share.RefreshShareRecrypt)
}

func (rfp *RefreshProtocol) Aggregate(share1, share2, shareOut RefreshShare) {
	rfp.bfvContext.ContextQ().Add(share1.RefreshShareDecrypt, share2.RefreshShareDecrypt, shareOut.RefreshShareDecrypt)
	rfp.bfvContext.ContextQ().Add(share1.RefreshShareRecrypt, share2.RefreshShareRecrypt, shareOut.RefreshShareRecrypt)
}

func (rfp *RefreshProtocol) Decrypt(ciphertext *bfv.Ciphertext, shareDecrypt RefreshShareDecrypt, sharePlaintext *ring.Poly) {
	rfp.bfvContext.ContextQ().Add(ciphertext.Value()[0], shareDecrypt, sharePlaintext)
}

func (rfp *RefreshProtocol) Recode(sharePlaintext *ring.Poly, sharePlaintextOut *ring.Poly) {
	scaler := ring.NewSimpleScaler(rfp.bfvContext.T(), rfp.bfvContext.ContextQ())

	scaler.Scale(sharePlaintext, sharePlaintextOut)
	lift(sharePlaintextOut, sharePlaintextOut, rfp.bfvContext)
}

func (rfp *RefreshProtocol) Recrypt(sharePlaintext *ring.Poly, crs *ring.Poly, shareRecrypt RefreshShareRecrypt, ciphertextOut *bfv.Ciphertext) {

	// ciphertext[0] = (-crs*s + e')/P + m
	rfp.bfvContext.ContextQ().Add(sharePlaintext, shareRecrypt, ciphertextOut.Value()[0])

	// ciphertext[1] = crs/P
	rfp.baseconverter.ModDown(rfp.bfvContext.ContextKeys(), rfp.bfvContext.RescaleParamsKeys(), uint64(len(ciphertextOut.Value()[1].Coeffs)-1), crs, ciphertextOut.Value()[1], rfp.tmp1)

}

func (rfp *RefreshProtocol) Finalize(ciphertext *bfv.Ciphertext, crs *ring.Poly, share RefreshShare, ciphertextOut *bfv.Ciphertext) {
	rfp.Decrypt(ciphertext, share.RefreshShareDecrypt, rfp.tmp1)
	rfp.Recode(rfp.tmp1, rfp.tmp1)
	rfp.Recrypt(rfp.tmp1, crs, share.RefreshShareRecrypt, ciphertextOut)
}

func lift(p0, p1 *ring.Poly, bfvcontext *bfv.BfvContext) {
	for j := uint64(0); j < bfvcontext.N(); j++ {
		for i := len(bfvcontext.ContextQ().Modulus) - 1; i >= 0; i-- {
			p1.Coeffs[i][j] = ring.MRed(p0.Coeffs[0][j], bfvcontext.DeltaMont()[i], bfvcontext.ContextQ().Modulus[i], bfvcontext.ContextQ().GetMredParams()[i])
		}
	}
}
