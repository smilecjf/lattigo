//Package ckks implements a RNS-accelerated version of the Homomorphic Encryption for Arithmetic for Approximate Numbers
//(HEAAN, a.k.a. CKKS) scheme. It provides approximate arithmetic over the complex numbers.package ckks
package ckks_fv

import (
	"fmt"
	"math"
	"math/big"
	"unsafe"

	"github.com/ldsec/lattigo/v2/ring"
	"github.com/ldsec/lattigo/v2/utils"
)

// GaloisGen is an integer of order N/2 modulo M and that spans Z_M with the integer -1.
// The j-th ring automorphism takes the root zeta to zeta^(5j).
const GaloisGen int = 5

var pi = "3.1415926535897932384626433832795028841971693993751058209749445923078164062862089986280348253421170679821480865132823066470938446095505822317253594081284811174502841027019385211055596446229489549303819644288109756659334461284756482337867831652712019091456485669234603486104543266482133936072602491412737245870066063155881748815209209628292540917153643678925903600113305305488204665213841469519415116094330572703657595919530921861173819326117931051185480744623799627495673518857527248912279381830119491298336733624406566430860213949463952247371907021798609437027705392171762931767523846748184676694051320005681271452635608277857713427577896091736371787214684409012249534301465495853710507922796892589235420199561121290219608640344181598136297747713099605187072113499999983729780499510597317328160963185950244594553469083026425223082533446850352619311881710100031378387528865875332083814206171776691473035982534904287554687311595628638823537875937519577818577805321712268066130019278766111959092164201989"

// Encoder is an interface implenting the encoding algorithms.
type Encoder interface {
	// FV Encoding and Decoding
	EncodeUint(coeffs []uint64, pt *Plaintext)
	EncodeUintRingT(coeffs []uint64, pt *PlaintextRingT)
	EncodeUintMul(coeffs []uint64, pt *PlaintextMul)
	EncodeInt(coeffs []int64, pt *Plaintext)
	EncodeIntRingT(coeffs []int64, pt *PlaintextRingT)
	EncodeIntMul(coeffs []int64, pt *PlaintextMul)

	FVScaleUp(*PlaintextRingT, *Plaintext)
	FVScaleDown(pt *Plaintext, ptRt *PlaintextRingT)
	RingTToMul(ptRt *PlaintextRingT, ptmul *PlaintextMul)
	MulToRingT(pt *PlaintextMul, ptRt *PlaintextRingT)

	DecodeRingT(pt interface{}, ptRt *PlaintextRingT)
	DecodeUint(pt interface{}, coeffs []uint64)
	DecodeInt(pt interface{}, coeffs []int64)
	DecodeUintNew(pt interface{}) (coeffs []uint64)
	DecodeIntNew(pt interface{}) (coeffs []int64)

	// CKKS Encoding and Decoding
	EncodeComplex(plaintext *Plaintext, values []complex128, logSlots int)
	EncodeComplexNew(values []complex128, logSlots int) (plaintext *Plaintext)
	EncodeComplexAtLvlNew(level int, values []complex128, logSlots int) (plaintext *Plaintext)

	EncodeComplexNTT(plaintext *Plaintext, values []complex128, logSlots int)
	EncodeComplexNTTNew(values []complex128, logSlots int) (plaintext *Plaintext)
	EncodeComplexNTTAtLvlNew(level int, values []complex128, logSlots int) (plaintext *Plaintext)

	EncodeDiagMatrixAtLvl(level int, vector map[int][]complex128, scale, maxM1N2Ratio float64, logSlots int) (matrix *PtDiagMatrix)

	DecodeComplex(plaintext *Plaintext, logSlots int) (res []complex128)
	DecodeComplexPublic(plaintext *Plaintext, logSlots int, sigma float64) []complex128

	EmbedComplex(values []complex128, logSlots int)
	CKKSScaleUp(pol *ring.Poly, scale float64, moduli []uint64)

	WipeInternalMemory()

	EncodeCoeffs(values []float64, plaintext *Plaintext)
	DecodeCoeffs(plaintext *Plaintext) (res []float64)
	DecodeCoeffsPublic(plaintext *Plaintext, bound float64) (res []float64)

	GetErrSTDTimeDom(valuesWant, valuesHave []complex128, scale float64) (std float64)
	GetErrSTDFreqDom(valuesWant, valuesHave []complex128, scale float64) (std float64)
}

// EncoderBigComplex is an interface implenting the encoding algorithms with arbitrary precision.
type EncoderBigComplex interface {
	EncodeComplex(plaintext *Plaintext, values []*ring.Complex, logSlots int)
	EncodeComplexNew(values []*ring.Complex, logSlots int) (plaintext *Plaintext)
	EncodeComplexAtLvlNew(level int, values []*ring.Complex, logSlots int) (plaintext *Plaintext)
	EncodeComplexNTT(plaintext *Plaintext, values []*ring.Complex, logSlots int)
	EncodeComplexNTTAtLvlNew(level int, values []*ring.Complex, logSlots int) (plaintext *Plaintext)
	DecodeComplex(plaintext *Plaintext, logSlots int) (res []*ring.Complex)
	FFT(values []*ring.Complex, N int)
	InvFFT(values []*ring.Complex, N int)

	//EncodeCoeffs(values []*big.Float, plaintext *Plaintext)
	//DecodeCoeffs(plaintext *Plaintext) (res []*big.Float)
}

// encoder is a struct storing the necessary parameters to encode a slice of complex number on a Plaintext.
type encoder struct {
	params *Parameters
	ringQ  *ring.Ring
	ringP  *ring.Ring
	ringT  *ring.Ring

	// ckks
	bigintChain  []*big.Int
	bigintCoeffs []*big.Int
	qHalf        *big.Int
	polypool     *ring.Poly
	m            int
	rotGroup     []int

	values      []complex128
	valuesfloat []float64
	roots       []complex128

	gaussianSampler *ring.GaussianSampler

	// fv
	indexMatrix []uint64
	scaler      ring.Scaler
	deltaMont   []uint64

	tmpPoly *ring.Poly
	tmpPtRt *PlaintextRingT
}

type encoderComplex128 struct {
	encoder
	values      []complex128
	valuesfloat []float64
	roots       []complex128
}

func newEncoder(params *Parameters) encoder {

	m := 2 * params.N()

	var q *ring.Ring
	var err error
	if q, err = ring.NewRing(params.N(), params.qi); err != nil {
		panic(err)
	}

	var p *ring.Ring
	if params.PiCount() != 0 {
		if p, err = ring.NewRing(params.N(), params.pi); err != nil {
			panic(err)
		}
	}

	var ringT *ring.Ring
	if ringT, err = ring.NewRing(params.N(), []uint64{params.t}); err != nil {
		panic(err)
	}

	rotGroup := make([]int, m>>1)
	fivePows := 1
	for i := 0; i < m>>2; i++ {
		rotGroup[i] = fivePows
		fivePows *= GaloisGen
		fivePows &= (m - 1)
	}

	prng, err := utils.NewPRNG()
	if err != nil {
		panic(err)
	}

	gaussianSampler := ring.NewGaussianSampler(prng)

	indexMatrix := make([]uint64, params.N())
	logN := params.LogN()
	rowSize := params.N() >> 1

	var index1, index2, pos int
	pos = 1
	for i := 0; i < rowSize; i++ {
		index1 = (pos - 1) >> 1
		index2 = (m - pos - 1) >> 1

		indexMatrix[i] = utils.BitReverse64(uint64(index1), uint64(logN))
		indexMatrix[i|rowSize] = utils.BitReverse64(uint64(index2), uint64(logN))

		pos *= GaloisGen
		pos &= (m - 1)
	}

	return encoder{
		params:          params.Copy(),
		ringQ:           q,
		ringP:           p,
		ringT:           ringT,
		bigintChain:     genBigIntChain(params.qi),
		bigintCoeffs:    make([]*big.Int, m>>1),
		qHalf:           ring.NewUint(0),
		polypool:        q.NewPoly(),
		m:               m,
		rotGroup:        rotGroup,
		gaussianSampler: gaussianSampler,
		indexMatrix:     indexMatrix,
		scaler:          ring.NewRNSScaler(params.t, q),
		deltaMont:       GenLiftParams(q, params.t),
		tmpPoly:         ringT.NewPoly(),
		tmpPtRt:         NewPlaintextRingT(params),
	}
}

// GenLiftParams generates the lifting parameters.
func GenLiftParams(ringQ *ring.Ring, t uint64) (deltaMont []uint64) {

	delta := new(big.Int).Quo(ringQ.ModulusBigint, ring.NewUint(t))

	deltaMont = make([]uint64, len(ringQ.Modulus))

	tmp := new(big.Int)
	bredParams := ringQ.BredParams
	for i, Qi := range ringQ.Modulus {
		deltaMont[i] = tmp.Mod(delta, ring.NewUint(Qi)).Uint64()
		deltaMont[i] = ring.MForm(deltaMont[i], Qi, bredParams[i])
	}

	return
}

// NewEncoder creates a new Encoder that is used to encode a slice of complex values of size at most N/2 (the number of slots) on a Plaintext.
func NewEncoder(params *Parameters) Encoder {

	encoder := newEncoder(params)

	var angle float64
	roots := make([]complex128, encoder.m+1)
	for i := 0; i < encoder.m; i++ {
		angle = 2 * 3.141592653589793 * float64(i) / float64(encoder.m)

		roots[i] = complex(math.Cos(angle), math.Sin(angle))
	}
	roots[encoder.m] = roots[0]

	return &encoderComplex128{
		encoder:     encoder,
		roots:       roots,
		values:      make([]complex128, encoder.m>>2),
		valuesfloat: make([]float64, encoder.m>>1),
	}
}

// EncodeUintRingT encodes a slice of uint64 into a Plaintext in R_t
func (encoder *encoder) EncodeUintRingT(coeffs []uint64, p *PlaintextRingT) {
	if len(coeffs) > len(encoder.indexMatrix) {
		panic("invalid input to encode: number of coefficients must be smaller or equal to the ring degree")
	}

	if len(p.value.Coeffs[0]) != len(encoder.indexMatrix) {
		panic("invalid plaintext to receive encoding: number of coefficients does not match the ring degree")
	}

	for i := 0; i < len(coeffs); i++ {
		p.value.Coeffs[0][encoder.indexMatrix[i]] = coeffs[i]
	}

	for i := len(coeffs); i < len(encoder.indexMatrix); i++ {
		p.value.Coeffs[0][encoder.indexMatrix[i]] = 0
	}

	encoder.ringT.InvNTT(p.value, p.value)
}

// EncodeUint encodes an uint64 slice of size at most N on a plaintext.
func (encoder *encoder) EncodeUint(coeffs []uint64, p *Plaintext) {
	ptRt := &PlaintextRingT{p.Element, p.Element.value[0]}

	// Encodes the values in RingT
	encoder.EncodeUintRingT(coeffs, ptRt)

	// Scales by Q/t
	encoder.FVScaleUp(ptRt, p)
}

func (encoder *encoder) EncodeUintMul(coeffs []uint64, p *PlaintextMul) {

	ptRt := &PlaintextRingT{p.Element, p.Element.value[0]}

	// Encodes the values in RingT
	encoder.EncodeUintRingT(coeffs, ptRt)

	// Puts in NTT+Montgomery domains of ringQ
	encoder.RingTToMul(ptRt, p)
}

// EncodeInt encodes an int64 slice of size at most N on a plaintext. It also encodes the sign of the given integer (as its inverse modulo the plaintext modulus).
// The sign will correctly decode as long as the absolute value of the coefficient does not exceed half of the plaintext modulus.
func (encoder *encoder) EncodeIntRingT(coeffs []int64, p *PlaintextRingT) {

	if len(coeffs) > len(encoder.indexMatrix) {
		panic("invalid input to encode: number of coefficients must be smaller or equal to the ring degree")
	}

	if len(p.value.Coeffs[0]) != len(encoder.indexMatrix) {
		panic("invalid plaintext to receive encoding: number of coefficients does not match the ring degree")
	}

	for i := 0; i < len(coeffs); i++ {

		if coeffs[i] < 0 {
			p.value.Coeffs[0][encoder.indexMatrix[i]] = uint64(int64(encoder.params.t) + coeffs[i])
		} else {
			p.value.Coeffs[0][encoder.indexMatrix[i]] = uint64(coeffs[i])
		}
	}

	for i := len(coeffs); i < len(encoder.indexMatrix); i++ {
		p.value.Coeffs[0][encoder.indexMatrix[i]] = 0
	}

	encoder.ringT.InvNTTLazy(p.value, p.value)
}

func (encoder *encoder) EncodeInt(coeffs []int64, p *Plaintext) {
	ptRt := &PlaintextRingT{p.Element, p.value}

	// Encodes the values in RingT
	encoder.EncodeIntRingT(coeffs, ptRt)

	// Scales by Q/t
	encoder.FVScaleUp(ptRt, p)
}

func (encoder *encoder) EncodeIntMul(coeffs []int64, p *PlaintextMul) {
	ptRt := &PlaintextRingT{p.Element, p.value}

	// Encodes the values in RingT
	encoder.EncodeIntRingT(coeffs, ptRt)

	// Puts in NTT+Montgomery domains of ringQ
	encoder.RingTToMul(ptRt, p)
}

// FVScaleUp transforms a PlaintextRingT (R_t) into a Plaintext (R_q) by scaling up the coefficient by Q/t.
func (encoder *encoder) FVScaleUp(ptRt *PlaintextRingT, pt *Plaintext) {
	fvScaleUp(encoder.ringQ, encoder.deltaMont, ptRt.value, pt.value)
}

func fvScaleUp(ringQ *ring.Ring, deltaMont []uint64, pIn, pOut *ring.Poly) {

	for i := len(ringQ.Modulus) - 1; i >= 0; i-- {
		out := pOut.Coeffs[i]
		in := pIn.Coeffs[0]
		d := deltaMont[i]
		qi := ringQ.Modulus[i]
		mredParams := ringQ.MredParams[i]

		for j := 0; j < ringQ.N; j = j + 8 {

			x := (*[8]uint64)(unsafe.Pointer(&in[j]))
			z := (*[8]uint64)(unsafe.Pointer(&out[j]))

			z[0] = ring.MRed(x[0], d, qi, mredParams)
			z[1] = ring.MRed(x[1], d, qi, mredParams)
			z[2] = ring.MRed(x[2], d, qi, mredParams)
			z[3] = ring.MRed(x[3], d, qi, mredParams)
			z[4] = ring.MRed(x[4], d, qi, mredParams)
			z[5] = ring.MRed(x[5], d, qi, mredParams)
			z[6] = ring.MRed(x[6], d, qi, mredParams)
			z[7] = ring.MRed(x[7], d, qi, mredParams)
		}
	}
}

// FVScaleDown transforms a Plaintext (R_q) into a PlaintextRingT (R_t) by scaling down the coefficient by t/Q and rounding.
func (encoder *encoder) FVScaleDown(pt *Plaintext, ptRt *PlaintextRingT) {
	encoder.scaler.DivByQOverTRounded(pt.value, ptRt.value)
}

// RingTToMul transforms a PlaintextRingT into a PlaintextMul by operating the NTT transform
// of R_q and putting the coefficients in Montgomery form.
func (encoder *encoder) RingTToMul(ptRt *PlaintextRingT, ptMul *PlaintextMul) {
	if ptRt.value != ptMul.value {
		copy(ptMul.value.Coeffs[0], ptRt.value.Coeffs[0])
	}
	for i := 1; i < len(encoder.ringQ.Modulus); i++ {
		copy(ptMul.value.Coeffs[i], ptRt.value.Coeffs[0])
	}

	encoder.ringQ.NTTLazy(ptMul.value, ptMul.value)
	encoder.ringQ.MForm(ptMul.value, ptMul.value)
}

// MulToRingT transforms a PlaintextMul into PlaintextRingT by operating the inverse NTT transform of R_q and
// putting the coefficients out of the Montgomery form.
func (encoder *encoder) MulToRingT(pt *PlaintextMul, ptRt *PlaintextRingT) {
	encoder.ringQ.InvNTTLvl(0, pt.value, ptRt.value)
	encoder.ringQ.InvMFormLvl(0, ptRt.value, ptRt.value)
}

// DecodeRingT decodes any plaintext type into a PlaintextRingT. It panics if p is not PlaintextRingT, Plaintext or PlaintextMul.
func (encoder *encoder) DecodeRingT(p interface{}, ptRt *PlaintextRingT) {
	switch pt := p.(type) {
	case *Plaintext:
		encoder.FVScaleDown(pt, ptRt)
	case *PlaintextMul:
		encoder.MulToRingT(pt, ptRt)
	case *PlaintextRingT:
		ptRt.Copy(pt.Element)
	default:
		panic(fmt.Errorf("unsupported plaintext type (%T)", pt))
	}
}

// DecodeUint decodes a any plaintext type and write the coefficients in coeffs. It panics if p is not PlaintextRingT, Plaintext or PlaintextMul.
func (encoder *encoder) DecodeUint(p interface{}, coeffs []uint64) {

	var ptRt *PlaintextRingT
	var isInRingT bool
	if ptRt, isInRingT = p.(*PlaintextRingT); !isInRingT {
		encoder.DecodeRingT(p, encoder.tmpPtRt)
		ptRt = encoder.tmpPtRt
	}

	encoder.ringT.NTT(ptRt.value, encoder.tmpPoly)

	for i := 0; i < encoder.ringQ.N; i++ {
		coeffs[i] = encoder.tmpPoly.Coeffs[0][encoder.indexMatrix[i]]
	}
}

// DecodeUintNew decodes any plaintext type and returns the coefficients in a new []uint64.
// It panics if p is not PlaintextRingT, Plaintext or PlaintextMul.
func (encoder *encoder) DecodeUintNew(p interface{}) (coeffs []uint64) {
	coeffs = make([]uint64, encoder.ringQ.N)
	encoder.DecodeUint(p, coeffs)
	return
}

// DecodeInt decodes a any plaintext type and write the coefficients in coeffs. It also decodes the sign
// modulus (by centering the values around the plaintext). It panics if p is not PlaintextRingT, Plaintext or PlaintextMul.
func (encoder *encoder) DecodeInt(p interface{}, coeffs []int64) {

	encoder.DecodeRingT(p, encoder.tmpPtRt)

	encoder.ringT.NTT(encoder.tmpPtRt.value, encoder.tmpPoly)

	modulus := int64(encoder.params.t)
	modulusHalf := modulus >> 1
	var value int64
	for i := 0; i < encoder.ringQ.N; i++ {

		value = int64(encoder.tmpPoly.Coeffs[0][encoder.indexMatrix[i]])
		coeffs[i] = value
		if value >= modulusHalf {
			coeffs[i] -= modulus
		}
	}
}

// DecodeIntNew decodes any plaintext type and returns the coefficients in a new []int64. It also decodes the sign
// modulus (by centering the values around the plaintext). It panics if p is not PlaintextRingT, Plaintext or PlaintextMul.
func (encoder *encoder) DecodeIntNew(p interface{}) (coeffs []int64) {
	coeffs = make([]int64, encoder.ringQ.N)
	encoder.DecodeInt(p, coeffs)
	return
}

// EncodeComplexNew encodes a slice of complex128 of length slots = 2^{logSlots} on new plaintext at the maximum level.
func (encoder *encoderComplex128) EncodeComplexNew(values []complex128, logSlots int) (plaintext *Plaintext) {
	return encoder.EncodeComplexAtLvlNew(encoder.params.MaxLevel(), values, logSlots)
}

// EncodeComplexAtLvlNew encodes a slice of complex128 of length slots = 2^{logSlots} on new plaintext at the desired level.
func (encoder *encoderComplex128) EncodeComplexAtLvlNew(level int, values []complex128, logSlots int) (plaintext *Plaintext) {
	plaintext = NewPlaintextCKKS(encoder.params, level, encoder.params.scale)
	encoder.EncodeComplex(plaintext, values, logSlots)
	return
}

// EncodeComplex encodes a slice of complex128 of length slots = 2^{logSlots} on the input plaintext.
func (encoder *encoderComplex128) EncodeComplex(plaintext *Plaintext, values []complex128, logSlots int) {
	encoder.EmbedComplex(values, logSlots)
	encoder.CKKSScaleUp(plaintext.value, plaintext.scale, encoder.ringQ.Modulus[:plaintext.Level()+1])
	encoder.WipeInternalMemory()
	plaintext.isNTT = false
}

// EncodeComplexNTTNew encodes a slice of complex128 of length slots = 2^{logSlots} on new plaintext at the maximum level.
// Returns a plaintext in the NTT domain.
func (encoder *encoderComplex128) EncodeComplexNTTNew(values []complex128, logSlots int) (plaintext *Plaintext) {
	return encoder.EncodeComplexNTTAtLvlNew(encoder.params.MaxLevel(), values, logSlots)
}

// EncodeComplexNTTAtLvlNew encodes a slice of complex128 of length slots = 2^{logSlots} on new plaintext at the desired level.
// Returns a plaintext in the NTT domain.
func (encoder *encoderComplex128) EncodeComplexNTTAtLvlNew(level int, values []complex128, logSlots int) (plaintext *Plaintext) {
	plaintext = NewPlaintextCKKS(encoder.params, encoder.params.MaxLevel(), encoder.params.scale)
	encoder.EncodeComplexNTT(plaintext, values, logSlots)
	return
}

// EncodeComplexNTT encodes a slice of complex128 of length slots = 2^{logSlots} on the input plaintext.
// Returns a plaintext in the NTT domain.
func (encoder *encoderComplex128) EncodeComplexNTT(plaintext *Plaintext, values []complex128, logSlots int) {
	encoder.EncodeComplex(plaintext, values, logSlots)
	encoder.ringQ.NTTLvl(plaintext.Level(), plaintext.value, plaintext.value)
	plaintext.isNTT = true
}

// EmbedComplex encodes a vector and stores internally the encoded values.
// To be used in conjunction with CKKSScaleUp.
func (encoder *encoderComplex128) EmbedComplex(values []complex128, logSlots int) {

	slots := 1 << logSlots

	if len(values) > encoder.params.N()/2 || len(values) > slots || logSlots > encoder.params.LogN()-1 {
		panic("cannot Encode: too many values/slots for the given ring degree")
	}

	for i := range values {
		encoder.values[i] = values[i]
	}

	invfft(encoder.values, slots, encoder.m, encoder.rotGroup, encoder.roots)

	gap := (encoder.ringQ.N >> 1) / slots

	for i, jdx, idx := 0, encoder.ringQ.N>>1, 0; i < slots; i, jdx, idx = i+1, jdx+gap, idx+gap {
		encoder.valuesfloat[idx] = real(encoder.values[i])
		encoder.valuesfloat[jdx] = imag(encoder.values[i])
	}
}

// GetErrSTDFreqDom returns the scaled standard deviation of the difference between two complex vectors in the slot domains
func (encoder *encoderComplex128) GetErrSTDFreqDom(valuesWant, valuesHave []complex128, scale float64) (std float64) {

	var err complex128
	for i := range valuesWant {
		err = valuesWant[i] - valuesHave[i]
		encoder.valuesfloat[2*i] = real(err)
		encoder.valuesfloat[2*i+1] = imag(err)
	}

	return StandardDeviation(encoder.valuesfloat[:len(valuesWant)*2], scale)
}

// GetErrSTDTimeDom returns the scaled standard deviation of the coefficient domain of the difference between two complex vectors in the slot domains
func (encoder *encoderComplex128) GetErrSTDTimeDom(valuesWant, valuesHave []complex128, scale float64) (std float64) {

	for i := range valuesHave {
		encoder.values[i] = (valuesWant[i] - valuesHave[i])
	}

	invfft(encoder.values, len(valuesWant), encoder.m, encoder.rotGroup, encoder.roots)

	for i := range valuesWant {
		encoder.valuesfloat[2*i] = real(encoder.values[i])
		encoder.valuesfloat[2*i+1] = imag(encoder.values[i])
	}

	return StandardDeviation(encoder.valuesfloat[:len(valuesWant)*2], scale)

}

// ScaleUp writes the internaly stored encoded values on a polynomial.
func (encoder *encoderComplex128) CKKSScaleUp(pol *ring.Poly, scale float64, moduli []uint64) {
	scaleUpVecExact(encoder.valuesfloat, scale, moduli, pol.Coeffs)
}

// WipeInternalMemory sets the internally stored encoded values of the encoder to zero.
func (encoder *encoderComplex128) WipeInternalMemory() {
	for i := range encoder.values {
		encoder.values[i] = 0
	}

	for i := range encoder.valuesfloat {
		encoder.valuesfloat[i] = 0
	}
}

// DecodeComplexPublic decodes the Plaintext values to a slice of complex128 values of size at most N/2.
// Rounds the decimal part of the output (the bits under the scale) to "logPrecision" bits of precision.
func (encoder *encoderComplex128) DecodeComplexPublic(plaintext *Plaintext, logSlots int, bound float64) (res []complex128) {
	return encoder.decodeComplexPublic(plaintext, logSlots, bound)
}

// DecodeComplex decodes the Plaintext values to a slice of complex128 values of size at most N/2.
func (encoder *encoderComplex128) DecodeComplex(plaintext *Plaintext, logSlots int) (res []complex128) {
	return encoder.decodeComplexPublic(plaintext, logSlots, 0)
}

func polyToComplexNoCRT(coeffs []uint64, values []complex128, scale float64, logSlots int, Q uint64) {

	slots := 1 << logSlots
	maxSlots := len(coeffs) >> 1
	gap := maxSlots / slots

	var real, imag float64
	for i, idx := 0, 0; i < slots; i, idx = i+1, idx+gap {

		if coeffs[idx] >= Q>>1 {
			real = -float64(Q - coeffs[idx])
		} else {
			real = float64(coeffs[idx])
		}

		if coeffs[idx+maxSlots] >= Q>>1 {
			imag = -float64(Q - coeffs[idx+maxSlots])
		} else {
			imag = float64(coeffs[idx+maxSlots])
		}

		values[i] = complex(real, imag) / complex(scale, 0)
	}
}

func polyToComplexCRT(poly *ring.Poly, bigintCoeffs []*big.Int, values []complex128, scale float64, logSlots int, ringQ *ring.Ring, Q *big.Int) {

	ringQ.PolyToBigint(poly, bigintCoeffs)

	maxSlots := ringQ.N >> 1
	slots := 1 << logSlots
	gap := maxSlots / slots

	qHalf := new(big.Int)
	qHalf.Set(Q)
	qHalf.Rsh(qHalf, 1)

	var sign int

	for i, idx := 0, 0; i < slots; i, idx = i+1, idx+gap {

		// Centers the value around the current modulus
		bigintCoeffs[idx].Mod(bigintCoeffs[idx], Q)
		sign = bigintCoeffs[idx].Cmp(qHalf)
		if sign == 1 || sign == 0 {
			bigintCoeffs[idx].Sub(bigintCoeffs[idx], Q)
		}

		// Centers the value around the current modulus
		bigintCoeffs[idx+maxSlots].Mod(bigintCoeffs[idx+maxSlots], Q)
		sign = bigintCoeffs[idx+maxSlots].Cmp(qHalf)
		if sign == 1 || sign == 0 {
			bigintCoeffs[idx+maxSlots].Sub(bigintCoeffs[idx+maxSlots], Q)
		}

		values[i] = complex(scaleDown(bigintCoeffs[idx], scale), scaleDown(bigintCoeffs[idx+maxSlots], scale))
	}
}

func (encoder *encoderComplex128) plaintextToComplex(level int, scale float64, logSlots int, p *ring.Poly, values []complex128) {
	if level == 0 {
		polyToComplexNoCRT(p.Coeffs[0], encoder.values, scale, logSlots, encoder.ringQ.Modulus[0])
	} else {
		polyToComplexCRT(p, encoder.bigintCoeffs, values, scale, logSlots, encoder.ringQ, encoder.bigintChain[level])
	}
}

func roundComplexVector(values []complex128, bound float64) {
	for i := range values {
		a := math.Round(real(values[i])*bound) / bound
		b := math.Round(imag(values[i])*bound) / bound
		values[i] = complex(a, b)
	}
}

func polyToFloatNoCRT(coeffs []uint64, values []float64, scale float64, Q uint64) {

	for i, c := range coeffs {

		if c >= Q>>1 {
			values[i] = -float64(Q-c) / scale
		} else {
			values[i] = float64(c) / scale
		}
	}
}

// PtDiagMatrix is a struct storing a plaintext diagonalized matrix
// ready to be evaluated on a ciphertext using evaluator.MultiplyByDiagMatrice.
type PtDiagMatrix struct {
	LogSlots   int                   // Log of the number of slots of the plaintext (needed to compute the appropriate rotation keys)
	N1         int                   // N1 is the number of inner loops of the baby-step giant-step algo used in the evaluation.
	Level      int                   // Level is the level at which the matrix is encoded (can be circuit dependant)
	Scale      float64               // Scale is the scale at which the matrix is encoded (can be circuit dependant)
	Vec        map[int][2]*ring.Poly // Vec is the matrix, in diagonal form, where each entry of vec is an indexed non zero diagonal.
	naive      bool
	isGaussian bool // Each diagonal of the matrix is of the form [k, ..., k] for k a gaussian integer
}

func bsgsIndex(el interface{}, slots, N1 int) (index map[int][]int, rotations []int) {
	index = make(map[int][]int)
	rotations = []int{}
	switch element := el.(type) {
	case map[int][]complex128:
		for key := range element {
			key &= (slots - 1)
			idx1 := key / N1
			idx2 := key & (N1 - 1)
			if index[idx1] == nil {
				index[idx1] = []int{idx2}
			} else {
				index[idx1] = append(index[idx1], idx2)
			}

			if !utils.IsInSliceInt(idx2, rotations) {
				rotations = append(rotations, idx2)
			}
		}
	case map[int]bool:
		for key := range element {
			key &= (slots - 1)
			idx1 := key / N1
			idx2 := key & (N1 - 1)
			if index[idx1] == nil {
				index[idx1] = []int{idx2}
			} else {
				index[idx1] = append(index[idx1], idx2)
			}
			if !utils.IsInSliceInt(idx2, rotations) {
				rotations = append(rotations, idx2)
			}
		}
	case map[int][2]*ring.Poly:
		for key := range element {
			key &= (slots - 1)
			idx1 := key / N1
			idx2 := key & (N1 - 1)
			if index[idx1] == nil {
				index[idx1] = []int{idx2}
			} else {
				index[idx1] = append(index[idx1], idx2)
			}
			if !utils.IsInSliceInt(idx2, rotations) {
				rotations = append(rotations, idx2)
			}
		}
	}
	return
}

// EncodeDiagMatrixAtLvl encodes a diagonalized plaintext matrix into PtDiagMatrix struct.
// It can then be evaluated on a ciphertext using evaluator.MultiplyByDiagMatrice.
// maxM1N2Ratio is the maximum ratio between the inner and outer loop of the baby-step giant-step algorithm used in evaluator.MultiplyByDiagMatrice.
// Optimal maxM1N2Ratio value is between 4 and 16 depending on the sparsity of the matrix.
func (encoder *encoderComplex128) EncodeDiagMatrixAtLvl(level int, diagMatrix map[int][]complex128, scale, maxM1N2Ratio float64, logSlots int) (matrix *PtDiagMatrix) {

	matrix = new(PtDiagMatrix)
	matrix.LogSlots = logSlots
	slots := 1 << logSlots

	if len(diagMatrix) > 2 {

		// N1*N2 = N
		N1 := findbestbabygiantstepsplit(diagMatrix, slots, maxM1N2Ratio)
		matrix.N1 = N1

		index, _ := bsgsIndex(diagMatrix, slots, N1)

		matrix.Vec = make(map[int][2]*ring.Poly)

		matrix.Level = level
		matrix.Scale = scale

		for j := range index {

			for _, i := range index[j] {

				// manages inputs that have rotation between 0 and slots-1 or between -slots/2 and slots/2-1
				v := diagMatrix[N1*j+i]
				if len(v) == 0 {
					v = diagMatrix[(N1*j+i)-slots]
				}

				matrix.Vec[N1*j+i] = encoder.encodeDiagonal(logSlots, level, scale, rotate(v, -N1*j))
			}
		}
	} else {

		matrix.Vec = make(map[int][2]*ring.Poly)

		matrix.Level = level
		matrix.Scale = scale

		for i := range diagMatrix {

			idx := i
			if idx < 0 {
				idx += slots
			}
			matrix.Vec[idx] = encoder.encodeDiagonal(logSlots, level, scale, diagMatrix[i])
		}

		matrix.naive = true
	}

	return
}

func (encoder *encoderComplex128) encodeDiagonal(logSlots, level int, scale float64, m []complex128) [2]*ring.Poly {

	ringQ := encoder.ringQ
	ringP := encoder.ringP

	encoder.EmbedComplex(m, logSlots)

	mQ := ringQ.NewPolyLvl(level + 1)
	encoder.CKKSScaleUp(mQ, scale, ringQ.Modulus[:level+1])
	ringQ.NTTLvl(level, mQ, mQ)
	ringQ.MFormLvl(level, mQ, mQ)

	mP := ringP.NewPoly()
	encoder.CKKSScaleUp(mP, scale, ringP.Modulus)
	ringP.NTT(mP, mP)
	ringP.MForm(mP, mP)

	encoder.WipeInternalMemory()

	return [2]*ring.Poly{mQ, mP}
}

// Finds the best N1*N2 = N for the baby-step giant-step algorithm for matrix multiplication.
func findbestbabygiantstepsplit(diagMatrix interface{}, maxN int, maxRatio float64) (minN int) {

	for N1 := 1; N1 < maxN; N1 <<= 1 {

		index, _ := bsgsIndex(diagMatrix, maxN, N1)

		if len(index[0]) > 0 {

			hoisted := len(index[0]) - 1
			normal := len(index) - 1

			// The matrice is very sparse already
			if normal == 0 {
				return N1 / 2
			}

			if hoisted > normal {
				// Finds the next split that has a ratio hoisted/normal greater or equal to maxRatio
				for float64(hoisted)/float64(normal) < maxRatio {

					if normal/2 == 0 {
						break
					}
					N1 *= 2
					hoisted = hoisted*2 + 1
					normal = normal / 2
				}
				return N1
			}
		}
	}

	return 1
}

func (encoder *encoderComplex128) decodeComplexPublic(plaintext *Plaintext, logSlots int, sigma float64) (res []complex128) {

	if logSlots > encoder.params.LogN()-1 {
		panic("cannot Decode: too many slots for the given ring degree")
	}

	slots := 1 << logSlots

	if plaintext.isNTT {
		encoder.ringQ.InvNTTLvl(plaintext.Level(), plaintext.value, encoder.polypool)
	} else {
		encoder.ringQ.CopyLvl(plaintext.Level(), plaintext.value, encoder.polypool)
	}

	// B = floor(sigma * sqrt(2*pi))
	encoder.gaussianSampler.ReadAndAddLvl(plaintext.Level(), encoder.polypool, encoder.ringQ, sigma, int(2.5066282746310002*sigma))

	encoder.plaintextToComplex(plaintext.Level(), plaintext.Scale(), logSlots, encoder.polypool, encoder.values)

	fft(encoder.values, slots, encoder.m, encoder.rotGroup, encoder.roots)

	res = make([]complex128, slots)

	for i := range res {
		res[i] = encoder.values[i]
	}

	for i := range encoder.values {
		encoder.values[i] = 0
	}

	return
}

func invfft(values []complex128, N, M int, rotGroup []int, roots []complex128) {

	var lenh, lenq, gap, idx int
	var u, v complex128

	for len := N; len >= 1; len >>= 1 {
		for i := 0; i < N; i += len {
			lenh = len >> 1
			lenq = len << 2
			gap = M / lenq
			for j := 0; j < lenh; j++ {
				idx = (lenq - (rotGroup[j] % lenq)) * gap
				u = values[i+j] + values[i+j+lenh]
				v = values[i+j] - values[i+j+lenh]
				v *= roots[idx]
				values[i+j] = u
				values[i+j+lenh] = v

			}
		}
	}

	for i := 0; i < N; i++ {
		values[i] /= complex(float64(N), 0)
	}

	sliceBitReverseInPlaceComplex128(values, N)
}

func fft(values []complex128, N, M int, rotGroup []int, roots []complex128) {

	var lenh, lenq, gap, idx int
	var u, v complex128

	sliceBitReverseInPlaceComplex128(values, N)

	for len := 2; len <= N; len <<= 1 {
		for i := 0; i < N; i += len {
			lenh = len >> 1
			lenq = len << 2
			gap = M / lenq
			for j := 0; j < lenh; j++ {
				idx = (rotGroup[j] % lenq) * gap
				u = values[i+j]
				v = values[i+j+lenh]
				v *= roots[idx]
				values[i+j] = u + v
				values[i+j+lenh] = u - v
			}
		}
	}
}

// EncodeCoeffs takes as input a polynomial a0 + a1x + a2x^2 + ... + an-1x^n-1 with float coefficient
// and returns a scaled integer plaintext polynomial. Encodes at the input plaintext level.
func (encoder *encoderComplex128) EncodeCoeffs(values []float64, plaintext *Plaintext) {

	if len(values) > encoder.params.N() {
		panic("cannot EncodeCoeffs : too many values (maximum is N)")
	}

	scaleUpVecExact(values, plaintext.scale, encoder.ringQ.Modulus[:plaintext.Level()+1], plaintext.value.Coeffs)

	plaintext.isNTT = false
}

// EncodeCoeffsNTT takes as input a polynomial a0 + a1x + a2x^2 + ... + an-1x^n-1 with float coefficient
// and returns a scaled integer plaintext polynomial in NTT. Encodes at the input plaintext level.
func (encoder *encoderComplex128) EncodeCoeffsNTT(values []float64, plaintext *Plaintext) {
	encoder.EncodeCoeffs(values, plaintext)
	encoder.ringQ.NTTLvl(plaintext.Level(), plaintext.value, plaintext.value)
	plaintext.isNTT = true
}

// DecodeCoeffsPublic takes as input a plaintext and returns the scaled down coefficient of the plaintext in float64.
// Rounds the decimal part of the output (the bits under the scale) to "logPrecision" bits of precision.
func (encoder *encoderComplex128) DecodeCoeffsPublic(plaintext *Plaintext, sigma float64) (res []float64) {
	return encoder.decodeCoeffsPublic(plaintext, sigma)
}

func (encoder *encoderComplex128) DecodeCoeffs(plaintext *Plaintext) (res []float64) {
	return encoder.decodeCoeffsPublic(plaintext, 0)
}

// DecodeCoeffs takes as input a plaintext and returns the scaled down coefficient of the plaintext in float64.
func (encoder *encoderComplex128) decodeCoeffsPublic(plaintext *Plaintext, sigma float64) (res []float64) {

	if plaintext.isNTT {
		encoder.ringQ.InvNTTLvl(plaintext.Level(), plaintext.value, encoder.polypool)
	} else {
		encoder.ringQ.CopyLvl(plaintext.Level(), plaintext.value, encoder.polypool)
	}

	if sigma != 0 {
		// B = floor(sigma * sqrt(2*pi))
		encoder.gaussianSampler.ReadAndAddLvl(plaintext.Level(), encoder.polypool, encoder.ringQ, sigma, int(2.5066282746310002*sigma))
	}

	res = make([]float64, encoder.params.N())

	// We have more than one moduli and need the CRT reconstruction
	if plaintext.Level() > 0 {

		encoder.ringQ.PolyToBigint(encoder.polypool, encoder.bigintCoeffs)

		Q := encoder.bigintChain[plaintext.Level()]

		encoder.qHalf.Set(Q)
		encoder.qHalf.Rsh(encoder.qHalf, 1)

		var sign int

		for i := range res {

			// Centers the value around the current modulus
			encoder.bigintCoeffs[i].Mod(encoder.bigintCoeffs[i], Q)

			sign = encoder.bigintCoeffs[i].Cmp(encoder.qHalf)
			if sign == 1 || sign == 0 {
				encoder.bigintCoeffs[i].Sub(encoder.bigintCoeffs[i], Q)
			}

			res[i] = scaleDown(encoder.bigintCoeffs[i], plaintext.scale)
		}
		// We can directly get the coefficients
	} else {

		Q := encoder.ringQ.Modulus[0]
		coeffs := encoder.polypool.Coeffs[0]

		for i := range res {

			if coeffs[i] >= Q>>1 {
				res[i] = -float64(Q - coeffs[i])
			} else {
				res[i] = float64(coeffs[i])
			}

			res[i] /= plaintext.scale
		}
	}

	return
}

type encoderBigComplex struct {
	encoder
	zero            *big.Float
	cMul            *ring.ComplexMultiplier
	logPrecision    int
	values          []*ring.Complex
	valuesfloat     []*big.Float
	roots           []*ring.Complex
	gaussianSampler *ring.GaussianSampler
}

// NewEncoderBigComplex creates a new encoder using arbitrary precision complex arithmetic.
func NewEncoderBigComplex(params *Parameters, logPrecision int) EncoderBigComplex {
	encoder := newEncoder(params)

	var PI = new(big.Float)
	PI.SetPrec(uint(logPrecision))
	PI.SetString(pi)

	var PIHalf = new(big.Float)
	PIHalf.SetPrec(uint(logPrecision))
	PIHalf.SetString(pi)
	PIHalf.Quo(PIHalf, ring.NewFloat(2, logPrecision))

	var angle *big.Float
	roots := make([]*ring.Complex, encoder.m+1)
	for i := 0; i < encoder.m; i++ {
		angle = ring.NewFloat(2, logPrecision)
		angle.Mul(angle, PI)
		angle.Mul(angle, ring.NewFloat(float64(i), logPrecision))
		angle.Quo(angle, ring.NewFloat(float64(encoder.m), logPrecision))

		real := ring.Cos(angle)
		angle.Sub(PIHalf, angle)
		imag := ring.Cos(angle)

		roots[i] = ring.NewComplex(real, imag)
	}

	roots[encoder.m] = roots[0].Copy()

	values := make([]*ring.Complex, encoder.m>>2)
	valuesfloat := make([]*big.Float, encoder.m>>1)

	for i := 0; i < encoder.m>>2; i++ {

		values[i] = ring.NewComplex(ring.NewFloat(0, logPrecision), ring.NewFloat(0, logPrecision))
		valuesfloat[i*2] = ring.NewFloat(0, logPrecision)
		valuesfloat[(i*2)+1] = ring.NewFloat(0, logPrecision)
	}

	return &encoderBigComplex{
		encoder:      encoder,
		zero:         ring.NewFloat(0, logPrecision),
		cMul:         ring.NewComplexMultiplier(),
		logPrecision: logPrecision,
		roots:        roots,
		values:       values,
		valuesfloat:  valuesfloat,
	}
}

// EncodeComplexNew encodes a slice of ring.Complex of length slots = 2^{logSlots} on a new plaintext at the maximum level.
func (encoder *encoderBigComplex) EncodeComplexNew(values []*ring.Complex, logSlots int) (plaintext *Plaintext) {
	return encoder.EncodeComplexAtLvlNew(encoder.params.MaxLevel(), values, logSlots)
}

// EncodeComplexAtLvlNew encodes a slice of ring.Complex of length slots = 2^{logSlots} on a new plaintext at the desired level.
func (encoder *encoderBigComplex) EncodeComplexAtLvlNew(level int, values []*ring.Complex, logSlots int) (plaintext *Plaintext) {
	plaintext = NewPlaintextCKKS(encoder.params, level, encoder.params.scale)
	encoder.EncodeComplex(plaintext, values, logSlots)
	return
}

// EncodeComplexNTTNew encodes a slice of ring.Complex of length slots = 2^{logSlots} on a plaintext at the maximum level.
// Returns a plaintext in the NTT domain.
func (encoder *encoderBigComplex) EncodeComplexNTTNew(values []*ring.Complex, logSlots int) (plaintext *Plaintext) {
	return encoder.EncodeComplexNTTAtLvlNew(encoder.params.MaxLevel(), values, logSlots)
}

// EncodeComplexNTTAtLvlNew encodes a slice of ring.Complex of length slots = 2^{logSlots} on a plaintext at the desired level.
// Returns a plaintext in the NTT domain.
func (encoder *encoderBigComplex) EncodeComplexNTTAtLvlNew(level int, values []*ring.Complex, logSlots int) (plaintext *Plaintext) {
	plaintext = NewPlaintextCKKS(encoder.params, encoder.params.MaxLevel(), encoder.params.scale)
	encoder.EncodeComplexNTT(plaintext, values, logSlots)
	return
}

// EncodeComplexNTT encodes a slice of ring.Complex of length slots = 2^{logSlots} on a plaintext at the input plaintext level.
// Returns a plaintext in the NTT domain.
func (encoder *encoderBigComplex) EncodeComplexNTT(plaintext *Plaintext, values []*ring.Complex, logSlots int) {
	encoder.EncodeComplex(plaintext, values, logSlots)
	encoder.ringQ.NTTLvl(plaintext.Level(), plaintext.value, plaintext.value)
	plaintext.isNTT = true
}

// EncodeComplex encodes a slice of ring.Complex of length slots = 2^{logSlots} on a plaintext at the input plaintext level.
func (encoder *encoderBigComplex) EncodeComplex(plaintext *Plaintext, values []*ring.Complex, logSlots int) {

	slots := 1 << logSlots

	if len(values) > encoder.params.N()/2 || len(values) > slots || logSlots > encoder.params.LogN()-1 {
		panic("cannot Encode: too many values/slots for the given ring degree")
	}

	if len(values) != slots {
		panic("cannot Encode: number of values must be equal to slots")
	}

	for i := 0; i < slots; i++ {
		encoder.values[i].Set(values[i])
	}

	encoder.InvFFT(encoder.values, slots)

	gap := (encoder.ringQ.N >> 1) / slots

	for i, jdx, idx := 0, (encoder.ringQ.N >> 1), 0; i < slots; i, jdx, idx = i+1, jdx+gap, idx+gap {
		encoder.valuesfloat[idx].Set(encoder.values[i].Real())
		encoder.valuesfloat[jdx].Set(encoder.values[i].Imag())
	}

	scaleUpVecExactBigFloat(encoder.valuesfloat, plaintext.scale, encoder.ringQ.Modulus[:plaintext.Level()+1], plaintext.value.Coeffs)

	coeffsBigInt := make([]*big.Int, encoder.params.N())

	encoder.ringQ.PolyToBigint(plaintext.value, coeffsBigInt)

	for i := 0; i < (encoder.ringQ.N >> 1); i++ {
		encoder.values[i].Real().Set(encoder.zero)
		encoder.values[i].Imag().Set(encoder.zero)
	}

	for i := 0; i < encoder.ringQ.N; i++ {
		encoder.valuesfloat[i].Set(encoder.zero)
	}
}

// DecodeComplexPublic decodes the Plaintext values to a slice of complex128 values of size at most N/2.
// Rounds the decimal part of the output (the bits under the scale) to "logPrecision" bits of precision.
func (encoder *encoderBigComplex) DecodeComplexPublic(plaintext *Plaintext, logSlots int, sigma float64) (res []*ring.Complex) {
	return encoder.decodeComplexPublic(plaintext, logSlots, sigma)
}

func (encoder *encoderBigComplex) DecodeComplex(plaintext *Plaintext, logSlots int) (res []*ring.Complex) {
	return encoder.decodeComplexPublic(plaintext, logSlots, 0)
}

// Decode decodes the Plaintext values to a slice of complex128 values of size at most N/2.
func (encoder *encoderBigComplex) decodeComplexPublic(plaintext *Plaintext, logSlots int, sigma float64) (res []*ring.Complex) {

	slots := 1 << logSlots

	if logSlots > encoder.params.LogN()-1 {
		panic("cannot Decode: too many slots for the given ring degree")
	}

	encoder.ringQ.InvNTTLvl(plaintext.Level(), plaintext.value, encoder.polypool)

	if sigma != 0 {
		// B = floor(sigma * sqrt(2*pi))
		encoder.gaussianSampler.ReadAndAddLvl(plaintext.Level(), encoder.polypool, encoder.ringQ, sigma, int(2.5066282746310002*sigma+0.5))
	}

	encoder.ringQ.PolyToBigint(encoder.polypool, encoder.bigintCoeffs)

	Q := encoder.bigintChain[plaintext.Level()]

	maxSlots := encoder.ringQ.N >> 1

	scaleFlo := ring.NewFloat(plaintext.Scale(), encoder.logPrecision)

	encoder.qHalf.Set(Q)
	encoder.qHalf.Rsh(encoder.qHalf, 1)

	gap := maxSlots / slots

	var sign int

	for i, idx := 0, 0; i < slots; i, idx = i+1, idx+gap {

		// Centers the value around the current modulus
		encoder.bigintCoeffs[idx].Mod(encoder.bigintCoeffs[idx], Q)
		sign = encoder.bigintCoeffs[idx].Cmp(encoder.qHalf)
		if sign == 1 || sign == 0 {
			encoder.bigintCoeffs[idx].Sub(encoder.bigintCoeffs[idx], Q)
		}

		// Centers the value around the current modulus
		encoder.bigintCoeffs[idx+maxSlots].Mod(encoder.bigintCoeffs[idx+maxSlots], Q)
		sign = encoder.bigintCoeffs[idx+maxSlots].Cmp(encoder.qHalf)
		if sign == 1 || sign == 0 {
			encoder.bigintCoeffs[idx+maxSlots].Sub(encoder.bigintCoeffs[idx+maxSlots], Q)
		}

		encoder.values[i].Real().SetInt(encoder.bigintCoeffs[idx])
		encoder.values[i].Real().Quo(encoder.values[i].Real(), scaleFlo)

		encoder.values[i].Imag().SetInt(encoder.bigintCoeffs[idx+maxSlots])
		encoder.values[i].Imag().Quo(encoder.values[i].Imag(), scaleFlo)
	}

	encoder.FFT(encoder.values, slots)

	res = make([]*ring.Complex, slots)

	for i := range res {
		res[i] = encoder.values[i].Copy()
	}

	for i := 0; i < maxSlots; i++ {
		encoder.values[i].Real().Set(encoder.zero)
		encoder.values[i].Imag().Set(encoder.zero)
	}

	return
}

// InvFFT evaluates the encoding matrix on a slice fo ring.Complex values.
func (encoder *encoderBigComplex) InvFFT(values []*ring.Complex, N int) {

	var lenh, lenq, gap, idx int
	u := ring.NewComplex(nil, nil)
	v := ring.NewComplex(nil, nil)

	for len := N; len >= 1; len >>= 1 {
		for i := 0; i < N; i += len {
			lenh = len >> 1
			lenq = len << 2
			gap = encoder.m / lenq
			for j := 0; j < lenh; j++ {
				idx = (lenq - (encoder.rotGroup[j] % lenq)) * gap
				u.Add(values[i+j], values[i+j+lenh])
				v.Sub(values[i+j], values[i+j+lenh])
				encoder.cMul.Mul(v, encoder.roots[idx], v)
				values[i+j].Set(u)
				values[i+j+lenh].Set(v)
			}
		}
	}

	NBig := ring.NewFloat(float64(N), encoder.logPrecision)
	for i := range values {
		values[i][0].Quo(values[i][0], NBig)
		values[i][1].Quo(values[i][1], NBig)
	}

	sliceBitReverseInPlaceRingComplex(values, N)
}

// FFT evaluates the decoding matrix on a slice fo ring.Complex values.
func (encoder *encoderBigComplex) FFT(values []*ring.Complex, N int) {

	var lenh, lenq, gap, idx int

	u := ring.NewComplex(nil, nil)
	v := ring.NewComplex(nil, nil)

	sliceBitReverseInPlaceRingComplex(values, N)

	for len := 2; len <= N; len <<= 1 {
		for i := 0; i < N; i += len {
			lenh = len >> 1
			lenq = len << 2
			gap = encoder.m / lenq
			for j := 0; j < lenh; j++ {
				idx = (encoder.rotGroup[j] % lenq) * gap
				u.Set(values[i+j])
				v.Set(values[i+j+lenh])
				encoder.cMul.Mul(v, encoder.roots[idx], v)
				values[i+j].Add(u, v)
				values[i+j+lenh].Sub(u, v)
			}
		}
	}
}

// TODO: move to bootstrap_params.go
func rotate(x []complex128, n int) (y []complex128) {

	y = make([]complex128, len(x))

	mask := int(len(x) - 1)

	// Rotates to the left
	for i := 0; i < len(x); i++ {
		y[i] = x[(i+n)&mask]
	}

	return
}