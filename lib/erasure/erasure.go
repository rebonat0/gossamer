// Copyright 2023 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package erasure

// #cgo CFLAGS: -I.
// #cgo LDFLAGS: -L. -lerasure
// #include "./rustlib.h"
import (
	"C"
)
import (
	"errors"
	"unsafe"

	"github.com/ChainSafe/gossamer/lib/common"
	"github.com/ChainSafe/gossamer/lib/trie"
	"github.com/ChainSafe/gossamer/pkg/scale"
)

var (
	ZeroSizedData   = errors.New("Data can't be zero sized")
	ZeroSizedChunks = errors.New("Chunks can't be zero sized")
)

// ObtainChunks obtains erasure-coded chunks, one for each validator.
// This works only up to 65536 validators, and `n_validators` must be non-zero and accepts
// number of validators and scale encoded data.
func ObtainChunks(nValidators uint, data []byte) ([][]byte, error) {
	if len(data) == 0 {
		return nil, ZeroSizedData
	}

	var cFlattenedChunks *C.uchar
	var cFlattenedChunksLen C.size_t

	cnValidators := C.size_t(nValidators)
	cData := (*C.uchar)(unsafe.Pointer(&data[0]))
	cLen := C.size_t(len(data))

	cErr := C.obtain_chunks(cnValidators, cData, cLen, &cFlattenedChunks, &cFlattenedChunksLen)
	errStr := C.GoString(cErr)
	C.free(unsafe.Pointer(cErr))

	if len(errStr) > 0 {
		return nil, errors.New(errStr)
	}

	resData := C.GoBytes(unsafe.Pointer(cFlattenedChunks), C.int(cFlattenedChunksLen))
	C.free(unsafe.Pointer(cFlattenedChunks))

	chunkSize := uint(len(resData)) / nValidators
	chunks := make([][]byte, nValidators)

	start := uint(0)
	for i := start; i < nValidators; i++ {
		end := start + chunkSize
		chunks[i] = resData[start:end]
		start = end
	}

	return chunks, nil
}

// Reconstruct decodable data from a set of chunks.
//
// Provide an iterator containing chunk data and the corresponding index.
// The indices of the present chunks must be indicated. If too few chunks
// are provided, recovery is not possible.
//
// Works only up to 65536 validators, and `n_validators` must be non-zero
func Reconstruct(nValidators uint, chunks [][]byte) ([]byte, error) {
	if len(chunks) == 0 {
		return nil, ZeroSizedChunks
	}

	var cReconstructedData *C.uchar
	var cReconstructedDataLen C.size_t
	var flattenedChunks []byte

	for _, chunk := range chunks {
		flattenedChunks = append(flattenedChunks, chunk...)
	}

	cChunkSize := C.size_t(len(chunks[0]))
	cFlattenedChunks := (*C.uchar)(unsafe.Pointer(&flattenedChunks[0]))
	cFlattenedChunksLen := C.size_t(len(flattenedChunks))

	cErr := C.reconstruct(
		C.size_t(nValidators),
		cFlattenedChunks, cFlattenedChunksLen,
		cChunkSize,
		&cReconstructedData, &cReconstructedDataLen,
	)
	errStr := C.GoString(cErr)
	C.free(unsafe.Pointer(cErr))

	if len(errStr) > 0 {
		return nil, errors.New(errStr)
	}

	res := C.GoBytes(unsafe.Pointer(cReconstructedData), C.int(cReconstructedDataLen))
	C.free(unsafe.Pointer(cReconstructedData))

	return res, nil
}

func ChunksToTrie(chunks [][]byte) (*trie.Trie, error) {
	chunkTrie := trie.NewEmptyTrie()
	for i, chunk := range chunks {
		encodedI, err := scale.Marshal(uint32(i))
		if err != nil {
			return nil, err
		}

		chunkHash, err := common.Blake2bHash(chunk)
		if err != nil {
			return nil, err
		}

		err = chunkTrie.Put(encodedI, chunkHash[:])
		if err != nil {
			return nil, err
		}
	}
	return chunkTrie, nil
}
