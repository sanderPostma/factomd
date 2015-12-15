// Copyright 2015 Factom Foundation
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package entryCreditBlock

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/FactomProject/factomd/common/interfaces"
	"github.com/FactomProject/factomd/common/primitives"
	"io"

	ed "github.com/FactomProject/ed25519"
)

const (
	// CommitChainSize = 1+6+32+32+32+1+32+64
	CommitChainSize int = 200
)

type CommitChain struct {
	Version     uint8
	MilliTime   *primitives.ByteSlice6
	ChainIDHash interfaces.IHash
	Weld        interfaces.IHash
	EntryHash   interfaces.IHash
	Credits     uint8
	ECPubKey    *primitives.ByteSlice32
	Sig         *primitives.ByteSlice64
}

var _ interfaces.Printable = (*CommitChain)(nil)
var _ interfaces.BinaryMarshallable = (*CommitChain)(nil)
var _ interfaces.ShortInterpretable = (*CommitChain)(nil)
var _ interfaces.IECBlockEntry = (*CommitChain)(nil)
var _ interfaces.ISignable = (*CommitChain)(nil)

func (c *CommitChain) MarshalledSize() uint64 {
	return uint64(CommitChainSize)
}

func NewCommitChain() *CommitChain {
	c := new(CommitChain)
	c.Version = 0
	c.MilliTime = new(primitives.ByteSlice6)
	c.ChainIDHash = primitives.NewZeroHash()
	c.Weld = primitives.NewZeroHash()
	c.EntryHash = primitives.NewZeroHash()
	c.Credits = 0
	c.ECPubKey = new(primitives.ByteSlice32)
	c.Sig = new(primitives.ByteSlice64)
	return c
}

func (e *CommitChain) Hash() interfaces.IHash {
	bin, err := e.MarshalBinary()
	if err != nil {
		panic(err)
	}
	return primitives.Sha(bin)
}

func (b *CommitChain) IsInterpretable() bool {
	return false
}

func (b *CommitChain) Interpret() string {
	return ""
}

// CommitMsg returns the binary marshaled message section of the CommitEntry
// that is covered by the CommitEntry.Sig.
func (c *CommitChain) CommitMsg() []byte {
	p, err := c.MarshalBinary()
	if err != nil {
		return []byte{byte(0)}
	}
	return p[:len(p)-64-32]
}

// Return the timestamp in milliseconds.
func (c *CommitChain) GetMilliTime() int64 {
	a := make([]byte, 2, 8)
	a = append(a, c.MilliTime[:]...)
	milli := int64(binary.BigEndian.Uint64(a))
	return milli
}

func (c *CommitChain) IsValid() bool {

	//double check the credits in the commit
	if c.Credits < 10 || c.Version != 0 {
		return false
	}

	return ed.VerifyCanonical((*[32]byte)(c.ECPubKey), c.CommitMsg(), (*[64]byte)(c.Sig))
}

func (c *CommitChain) GetHash() interfaces.IHash {
	data, _ := c.MarshalBinary()
	return primitives.Sha(data)
}

func (c *CommitChain) GetSigHash() interfaces.IHash {
	data := c.CommitMsg()
	return primitives.Sha(data)
}

func (c *CommitChain) MarshalBinarySig() ([]byte, error) {
	buf := new(bytes.Buffer)

	// 1 byte Version
	if err := binary.Write(buf, binary.BigEndian, c.Version); err != nil {
		return buf.Bytes(), err
	}

	// 6 byte MilliTime
	buf.Write(c.MilliTime[:])

	// 32 byte double sha256 hash of the ChainID
	buf.Write(c.ChainIDHash.Bytes())

	// 32 byte Commit Weld sha256(sha256(Entry Hash + ChainID))
	buf.Write(c.Weld.Bytes())

	// 32 byte Entry Hash
	buf.Write(c.EntryHash.Bytes())

	// 1 byte number of Entry Credits
	if err := binary.Write(buf, binary.BigEndian, c.Credits); err != nil {
		return buf.Bytes(), err
	}

	return buf.Bytes(), nil
}

func (c *CommitChain) MarshalBinary() ([]byte, error) {
	buf := new(bytes.Buffer)

	b, err := c.MarshalBinarySig()
	if err != nil {
		return nil, err
	}

	buf.Write(b)

	// 32 byte Public Key
	buf.Write(c.ECPubKey[:])

	// 64 byte Signature
	buf.Write(c.Sig[:])

	return buf.Bytes(), nil
}

func (c *CommitChain) Sign(privateKey []byte) error {
	sig, err := primitives.SignSignable(privateKey, c)
	if err != nil {
		return err
	}
	if c.Sig == nil {
		c.Sig = new(primitives.ByteSlice64)
	}
	err = c.Sig.UnmarshalBinary(sig)
	if err != nil {
		return err
	}
	pub, err := primitives.PrivateKeyToPublicKey(privateKey)
	if err != nil {
		return err
	}
	if c.ECPubKey == nil {
		c.ECPubKey = new(primitives.ByteSlice32)
	}
	err = c.ECPubKey.UnmarshalBinary(pub)
	if err != nil {
		return err
	}
	return nil
}

func (c *CommitChain) ValidateSignatures() error {
	if c.ECPubKey == nil {
		return fmt.Errorf("No public key present")
	}
	if c.Sig == nil {
		return fmt.Errorf("No signature present")
	}
	data, err := c.MarshalBinarySig()
	if err != nil {
		return err
	}
	return primitives.VerifySignature(data, c.ECPubKey[:], c.Sig[:])
}

func (c *CommitChain) ECID() byte {
	return ECIDChainCommit
}

func (c *CommitChain) UnmarshalBinaryData(data []byte) (newData []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("Error unmarshalling: %v", r)
		}
	}()
	buf := bytes.NewBuffer(data)
	hash := make([]byte, 32)

	// 1 byte Version
	var b byte
	var p []byte
	if b, err = buf.ReadByte(); err != nil {
		return
	} else {
		c.Version = uint8(b)
	}

	if buf.Len() < 6 {
		err = io.EOF
		return
	}

	// 6 byte MilliTime
	if p = buf.Next(6); p == nil {
		err = fmt.Errorf("Could not read MilliTime")
		return
	} else {
		copy(c.MilliTime[:], p)
	}

	// 32 byte ChainIDHash
	if _, err = buf.Read(hash); err != nil {
		return
	} else if err = c.ChainIDHash.SetBytes(hash); err != nil {
		return
	}

	// 32 byte Weld
	if _, err = buf.Read(hash); err != nil {
		return
	} else if err = c.Weld.SetBytes(hash); err != nil {
		return
	}

	// 32 byte Entry Hash
	if _, err = buf.Read(hash); err != nil {
		return
	} else if err = c.EntryHash.SetBytes(hash); err != nil {
		return
	}

	// 1 byte number of Entry Credits
	if b, err = buf.ReadByte(); err != nil {
		return
	} else {
		c.Credits = uint8(b)
	}

	if buf.Len() < 32 {
		err = io.EOF
		return
	}

	// 32 byte Public Key
	if p := buf.Next(32); p == nil {
		err = fmt.Errorf("Could not read ECPubKey")
		return
	} else {
		copy(c.ECPubKey[:], p)
	}

	if buf.Len() < 64 {
		err = io.EOF
		return
	}

	// 64 byte Signature
	if p := buf.Next(64); p == nil {
		err = fmt.Errorf("Could not read Sig")
		return
	} else {
		copy(c.Sig[:], p)
	}

	newData = buf.Bytes()

	return
}

func (c *CommitChain) UnmarshalBinary(data []byte) (err error) {
	_, err = c.UnmarshalBinaryData(data)
	return
}

func (e *CommitChain) JSONByte() ([]byte, error) {
	return primitives.EncodeJSON(e)
}

func (e *CommitChain) JSONString() (string, error) {
	return primitives.EncodeJSONString(e)
}

func (e *CommitChain) JSONBuffer(b *bytes.Buffer) error {
	return primitives.EncodeJSONToBuffer(e, b)
}

func (e *CommitChain) String() string {
	str, _ := e.JSONString()
	return str
}
