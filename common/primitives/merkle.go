// Copyright 2015 Factom Foundation
// Use of this source code is governed by the MIT
// license that can be found in the LICENSE file.

package primitives

import (
	"fmt"
	"github.com/FactomProject/factomd/common/constants"
	"github.com/FactomProject/factomd/common/interfaces"
	"math"
)

var _ = fmt.Println

// nextPowerOfTwo returns the next highest power of two from a given number if
// it is not already a power of two.  This is a helper function used during the
// calculation of a merkle tree.
func nextPowerOfTwo(n int) int {
	// Return the number if it's already a power of 2.
	if n&(n-1) == 0 {
		return n
	}

	// Figure out and return the next power of two.
	exponent := uint(math.Log2(float64(n))) + 1
	return 1 << exponent // 2^exponent
}

// HashMerkleBranches takes two hashes, treated as the left and right tree
// nodes, and returns the hash of their concatenation.  This is a helper
// function used to aid in the generation of a merkle tree.
func hashMerkleBranches(left interfaces.IHash, right interfaces.IHash) interfaces.IHash {
	// Concatenate the left and right nodes.
	var barray []byte = make([]byte, constants.ADDRESS_LENGTH*2)
	copy(barray[:constants.ADDRESS_LENGTH], left.Bytes())
	copy(barray[constants.ADDRESS_LENGTH:], right.Bytes())

	newSha := Sha(barray)
	return newSha
}

// Give a list of hashes, return the root of the Merkle Tree
func ComputeMerkleRoot(hashes []interfaces.IHash) interfaces.IHash {
	merkles := BuildMerkleTreeStore(hashes)
	return merkles[len(merkles)-1]
}

// The root of the Merkle Tree is returned in merkles[len(merkles)-1]
func BuildMerkleTreeStore(hashes []interfaces.IHash) (merkles []interfaces.IHash) {
	if len(hashes) == 0 {
		return append(make([]interfaces.IHash, 0, 1), new(Hash))
	}
	// Calculate how many entries are required to hold the binary merkle
	// tree as a linear array and create an array of that size.
	nextPoT := nextPowerOfTwo(len(hashes))
	arraySize := nextPoT*2 - 1
	//	fmt.Println("hashes.len=", len(hashes), ", nextPoT=", nextPoT, ", array.size=", arraySize)
	merkles = make([]interfaces.IHash, arraySize)

	// Create the base transaction shas and populate the array with them.
	//for i, entity := range entities {
	//merkles[i] = entity.ShaHash()
	//}
	copy(merkles[:len(hashes)], hashes[:])

	// Start the array offset after the last transaction and adjusted to the
	// next power of two.
	offset := nextPoT
	for i := 0; i < arraySize-1; i += 2 {
		switch {
		// When there is no left child node, the parent is nil too.
		case merkles[i] == nil:
			merkles[offset] = nil

		// When there is no right child, the parent is generated by
		// hashing the concatenation of the left child with itself.
		case merkles[i+1] == nil:
			newSha := hashMerkleBranches(merkles[i], merkles[i])
			merkles[offset] = newSha

		// The normal case sets the parent node to the double sha256
		// of the concatentation of the left and right children.
		default:
			newSha := hashMerkleBranches(merkles[i], merkles[i+1])
			merkles[offset] = newSha
		}
		offset++
	}
	return merkles
}
