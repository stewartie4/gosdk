package bls

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/0chain/gosdk/miracl"
	"github.com/0chain/gosdk/miracl/core"
	"io"
	"math/rand"
	"strconv"
	"unsafe"
)

func Init() error {
	if BN254.Init() == BN254.BLS_FAIL {
		return errors.New("Couldn't initialize BLS")
	}
	return nil
}

// https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L711
var sRandReader io.Reader

// Basically entirely from
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L729>
func SetRandFunc(randReader io.Reader) {
	// if nil, uses default random generator. See getRandomValue.
	sRandReader = randReader
}

// TODO: remove when done porting.
// // Reads a random value from the function set in `SetRandFunc`.
// func getRandomValue() (byte, error) {
//   var b [1]byte
//   var n int
//   var err error
//   if sRandReader == nil {
//     n, err = rand.Read(b[:])
//   } else {
//     n, err = sRandReader.Read(b[:])
//   }
//   if n > 0 {
//     return b[0], nil
//   }
//   return 0, errors.New("getRandomValue(): End of stream")
// }

// Taken directly from herumi's library:
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L29>
func hex2byte(s string) ([]byte, error) {
	if (len(s) & 1) == 1 {
		return nil, fmt.Errorf("odd length")
	}
	return hex.DecodeString(s)
}

func DeserializeHexStr(s string) (*BN254.ECP, error) {
	b, err := hex2byte(s)
	if err != nil {
		return nil, err
	}
	return BN254.ECP_fromBytes(b), nil
}

func DeserializeHexStr2(s string) (*BN254.ECP2, error) {
	b, err := hex2byte(s)
	if err != nil {
		return nil, err
	}
	return BN254.ECP2_fromBytes(b), nil
}

func ToBytes(E *BN254.ECP) []byte {
	const BFS = BN254.BFS
	const G1S = BFS + 1 /* Group 1 Size */
	var ecp [G1S]byte
	E.ToBytes(ecp[:], true /*compress*/)
	return ecp[:]
}

func ToBytes2(E *BN254.ECP2) []byte {
	const MFS = BN254.MFS
	const G1S = 2*MFS + 1 /* Group 1 Size */
	const G2S = 4*MFS + 1 /* Group 2 Size */
	var SST [G2S]byte
	E.ToBytes(SST[:], false /*compress*/)
	return SST[:]
}

func CloneFP(fp *BN254.FP) *BN254.FP {
	result := BN254.NewFP()
	result.Copy(fp)
	return result
}

func GetMasterPublicKey(msk []SecretKey) []PublicKey {
	// GetMasterPublicKey --
	n := len(msk)
	mpk := make([]PublicKey, n)
	for i := 0; i < n; i++ {
		mpk[i] = *msk[i].GetPublicKey()
	}
	return mpk
}

//-----------------------------------------------------------------------------
// Signature.
//-----------------------------------------------------------------------------

// Copied directly from herumi's source code.
// Sign: <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L449>
// blsSignature: <https://github.com/herumi/bls/blob/1b48de51f4f76deb204d108f6126c1507623f739/include/bls/bls.h#L68>
// mclBnG1: <https://github.com/herumi/mcl/blob/0114a3029f74829e79dc51de6dfb28f5da580632/include/mcl/bn.h#L96>
type Sign struct {
	v *BN254.ECP
}

func (sig *Sign) Add(rhs *Sign) {
	sig.v.Add(rhs.v)
}

// Starting from herumi's library:
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L480>
func (sig *Sign) DeserializeHexStr(s string) error {
	var err error
	sig.v, err = DeserializeHexStr(s)
	if err != nil {
		return err
	}
	return nil
}

// Starting from herumi's library:
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L454>
func (sig *Sign) Serialize() []byte {
	return ToBytes(sig.v)
}

// Starting from herumi's library:
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L475>
func (sig *Sign) SerializeToHexStr() string {
	return hex.EncodeToString(sig.Serialize())
}

// Porting over <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L553>
func (sig *Sign) Verify(pub *PublicKey, m []byte) bool {
	b := BN254.Core_Verify(ToBytes(sig.v), m, ToBytes2(pub.v))
	return b == BN254.BLS_OK
}

func (sig *Sign) Recover(shares []Sign, from []ID) error {
	N := len(shares)
	shs := make([]*core.SHARE, N)
	for i := 0; i < N; i++ {
		sh := new(core.SHARE)
		// TODO: GetHexString should do a better job of restoring the original int.
		// TODO: Write a unit test to sanity check the above.
		id, err := strconv.Atoi(from[i].GetHexString())
		if err != nil {
			return err
		}
		sh.B = ToBytes(shares[i].v)
		sh.ID = byte(id)
		sh.NSR = byte(N)
		shs[i] = sh
	}
	b := core.Recover(shs)
	sig.v = BN254.ECP_fromBytes(b)
	return nil
}

//-----------------------------------------------------------------------------
// PublicKey.
//-----------------------------------------------------------------------------

// Copied directly from herumi's source code.
// PublicKey: <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L334>
// blsPublicKey: <https://github.com/herumi/bls/blob/1b48de51f4f76deb204d108f6126c1507623f739/include/bls/bls.h#L60>
// mclBnG1: <https://github.com/herumi/mcl/blob/0114a3029f74829e79dc51de6dfb28f5da580632/include/mcl/bn.h#L96>
type PublicKey struct {
	v *BN254.ECP2
}

func NewPublicKey() *PublicKey {
	pk := new(PublicKey)
	pk.v = BN254.NewECP2()
	return pk
}

// Starting from herumi's library:
// <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L480>
func (pk *PublicKey) DeserializeHexStr(s string) error {
	var err error
	pk.v, err = DeserializeHexStr2(s)
	if err != nil {
		return err
	}
	return nil
}

func (pk *PublicKey) SerializeToHexStr() string {
	return hex.EncodeToString(pk.Serialize())
}

func (pk *PublicKey) ToString() string {
	return pk.v.ToString()
}

func (pk *PublicKey) Serialize() []byte {
	return ToBytes2(pk.v)
}

func (pk *PublicKey) Add(rhs *PublicKey) {
	pk.v.Add(rhs.v)
}

func (pk *PublicKey) Set(pks []PublicKey, id *ID) error {
	pk.v = BN254.NewECP2()
	if len(pks) == 0 {
		return errors.New("No secret keys given.")
	}
	pk.v.Copy(pks[len(pks)-1].v)
	if len(pks) == 1 {
		return nil
	}
	for i := len(pks) - 2; i >= 0; i-- {
		// Please note: this 'Mul' function is not the right one to use. It just
		// multiplies, it doesn't do any modulus on the CURVE_Order. The right
		// multiply, G2mul, does operations modulus CURVE_Order.
		//
		// pk.v = pk.v.Mul(id.v.GetBIG())

		pk.v = BN254.G2mul(pk.v, id.v.GetBIG())
		pk.v.Add(pks[i].v)
	}
	return nil
}

func (pk *PublicKey) IsEqual(rhs *PublicKey) bool {
	return pk.v.Equals(rhs.v)
}

//-----------------------------------------------------------------------------
// ID.
//-----------------------------------------------------------------------------

// Copied directly from herumi's source code.
// ID: <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#47>
// blsId: <https://github.com/herumi/bls/blob/1b48de51f4f76deb204d108f6126c1507623f739/include/bls/bls.h#L52>
// FP as Fr: <https://github.com/miracl/core/blob/master/go/FP.go#L26>
type ID struct {
	v *BN254.FP
}

/// TODO: hex2byte is wrong, needs to be dec2byte.
// func (id *ID) SetDecString(s string) error {
//   b, err := hex2byte(s)
// 	if err != nil {
// 		return nil
// 	}
//   id.v = BN254.FP_fromBytes(b)
//   return nil
// }

func (id *ID) SetHexString(s string) error {
	b, err := hex2byte(s)
	if err != nil {
		return err
	}
	id.v = BN254.FP_fromBytes(b)
	return nil
}

func (id *ID) GetHexString() string {
	return hex.EncodeToString(id.Serialize())
}

func (id *ID) Serialize() []byte {
	var _a BN254.Chunk
	b := make([]byte, BN254.NLEN*int(unsafe.Sizeof(_a)))
	id.v.ToBytes(b)
	return b
}

//-----------------------------------------------------------------------------
// SecretKey.
//-----------------------------------------------------------------------------

// Copied directly from herumi's source code.
// SecretKey: <https://github.com/herumi/bls-go-binary/blob/ef6a150a928bddb19cee55aec5c80585528d9a96/bls/bls.go#L154>
// blsSecretKey: <https://github.com/herumi/bls/blob/1b48de51f4f76deb204d108f6126c1507623f739/include/bls/bls.h#L56>
// FP as Fr: <https://github.com/miracl/core/blob/master/go/FP.go#L26>
type SecretKey struct {
	v *BN254.FP
}

func NewSecretKey() *SecretKey {
	sk := new(SecretKey)
	sk.v = BN254.NewFP()
	return sk
}

func SecretKey_fromBytes(b []byte) *SecretKey {
	sk := new(SecretKey)
	sk.v = BN254.FP_fromBytes(b)
	return sk
}

func SecretKey_fromFP(fp *BN254.FP) *SecretKey {
	sk := new(SecretKey)
	sk.v = fp
	return sk
}

func (sk *SecretKey) GetFP() *BN254.FP {
	return sk.v
}

func (sk *SecretKey) Clone() *SecretKey {
	result := new(SecretKey)
	result.v = sk.CloneFP()
	return result
}

func (sk *SecretKey) CloneFP() *BN254.FP {
	return BN254.NewFPcopy(sk.v)
}

func (sk *SecretKey) SetByCSPRNG() error {
	b := make([]byte, BN254.MODBYTES)
	if sRandReader == nil {
		rand.Read(b)
	} else {
		err := binary.Read(sRandReader, binary.LittleEndian, b)
		/// Debug info to find out more about the given rand func.
		// fmt.Println("debug given sRandReader: ", len(b), b, err)
		if err != nil {
			fmt.Println("Couldn't read from sRandReader. Got error:", err)
			panic("Couldn't read from sRandReader. Got an error (printed out on previous lines.")
		}
	}
	sk.v = BN254.FP_fromBytes(b)
	return nil
}

// TODO: I'm pretty sure we need to work on serialization the XES too.
func (sk *SecretKey) DeserializeHexStr(s string) error {
	b, err := hex2byte(s)
	if err != nil {
		return err
	}
	sk.v = BN254.FP_fromBytes(b) // Method 2. Not sure.
	return nil
}

func (sk *SecretKey) SerializeToHexStr() string {
	return sk.v.ToString()
}

func (sk *SecretKey) Sign(m []byte) *Sign {
	// We're just using this miracl/core function to port over the Sign function.
	// func Core_Sign(SIG []byte, M []byte, S []byte) int {...}

	const BFS = BN254.BFS
	const G1S = BFS + 1 /* Group 1 Size */
	var SIG [G1S]byte

	b3 := make([]byte, int(BN254.MODBYTES))
	sk.v.ToBytes(b3)
	BN254.Core_Sign(SIG[:], m, b3)

	sig := new(Sign)
	sig.v = BN254.ECP_fromBytes(SIG[:])

	return sig
}

// Turns out this is just MPIN_GET_SERVER_SECRET
func (sk *SecretKey) GetPublicKey() *PublicKey {
	// Taken from:
	// https://github.com/miracl/core/blob/fda3416694d153f900b617d7bc42038df34a2da6/go/TestMPIN.go#L41
	// https://github.com/miracl/core/blob/fda3416694d153f900b617d7bc42038df34a2da6/go/TestMPIN.go#L79
	const MGS = BN254.MGS
	const MFS = BN254.MFS
	const G1S = 2*MFS + 1 /* Group 1 Size */
	const G2S = 4*MFS + 1 /* Group 2 Size */
	var S [MGS]byte
	var SST [G2S]byte
	sk.v.ToBytes(S[:])
	BN254.MPIN_GET_SERVER_SECRET(S[:], SST[:])
	result := new(PublicKey)
	result.v = BN254.ECP2_fromBytes(SST[:])
	return result
}

func (sk *SecretKey) Add(rhs *SecretKey) {
	sk.v.Add(rhs.v)
}

func (sk *SecretKey) SubFP(fp *BN254.FP) {
	sk.v.Sub(fp)
}

func (sk *SecretKey) GetMasterSecretKey(k int) (msk []SecretKey) {
	msk = make([]SecretKey, k)
	msk[0] = *sk
	for i := 1; i < k; i++ {
		msk[i].SetByCSPRNG()
	}
	return msk
}

// Porting over:
// blsSecretKeyShare: <https://github.com/herumi/bls/blob/3005a32a97ebdcb426d59caaa9868a074fe7b35a/src/bls_c_impl.hpp#L543>
// evaluatePolynomial: <https://github.com/herumi/mcl/blob/0114a3029f74829e79dc51de6dfb28f5da580632/include/mcl/lagrange.hpp#L64>
func (sk *SecretKey) Set(msk []SecretKey, id *ID) error {
	if len(msk) == 0 {
		return errors.New("No secret keys given.")
	}
	if len(msk) == 1 {
		sk.v = msk[0].CloneFP()
		return nil
	}
	sk.v = msk[len(msk)-1].CloneFP()

	m := BN254.NewBIGints(BN254.CURVE_Order)
	sk0 := sk.v.GetBIG()
	id0 := id.v.GetBIG()

	for i := len(msk) - 2; i >= 0; i-- {
		sk0 = BN254.Modmul(sk0, id0, m)
		sk0 = BN254.Modadd(sk0, msk[i].v.GetBIG(), m)

		// Sorry in advance for this long comment. It is necessary so that future
		// maintainers can understand the math behind what is going on here. And
		// I also spent a lot of time on this. I'd really rather not forget what
		// I learned. So this is a way for me to keep notes.
		//
		// Please note: the following Mul/Add functions are not the right ones to
		// use. This is an easy mistake to make because they are named 'Mul' and
		// 'Add'.
		//
		// What needs to happen are operations on the FP types in G1 or G2 group.
		// These two operations are modulo "Modulus" (ROM.go), the wrong constant.
		// Instead, we need to perform these operations modulo "CURVE_Order".
		// To do that, we call Modmul and Modadd with the correct BIGint, created
		// from "CURVE_Order".
		//
		// sk.v.Mul(id.v)
		// sk.v.Add(msk[i].v)
	}

	sk.v = BN254.NewFPbig(sk0)

	return nil
}

func (sk *SecretKey) IsEqual(rhs *SecretKey) bool {
	return sk.v.Equals(rhs.v)
}