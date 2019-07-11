package types

import (
	"bytes"
	"math/big"
	"testing"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestSegmentHash(t *testing.T) {
	var (
		streamID      = "1"
		segmentNumber = big.NewInt(0)
		d0            = []byte("QmR9BnJQisvevpCoSVWWKyownN58nydb2zQt9Z2VtnTnKe")
	)

	segment := &Segment{
		streamID,
		segmentNumber,
		crypto.Keccak256Hash(d0),
	}

	sHash := crypto.Keccak256Hash(segment.Flatten())

	if segment.Hash() != sHash {
		t.Fatalf("Invalid segment hash")
	}
}

func TestSegmentFlatten(t *testing.T) {
	// ensure that the flatten + manual hash results are identical to the
	// segment hash results

	s := Segment{
		StreamID:              "abcdef",
		SegmentSequenceNumber: big.NewInt(1234),
		DataHash:              ethcommon.BytesToHash(ethcommon.RightPadBytes([]byte("browns"), 32)),
	}
	if !bytes.Equal(crypto.Keccak256Hash(s.Flatten()).Bytes(), s.Hash().Bytes()) {
		t.Error("Flattened segment + hash did not match segment hash function")
	}
}
