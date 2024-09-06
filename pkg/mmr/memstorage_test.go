// Copyright 2024 ChainSafe Systems (ON)
// SPDX-License-Identifier: LGPL-3.0-only

package mmr

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

type MMRElement []byte

func TestGetElement(t *testing.T) {
	memStorage := NewMemStorage[MMRElement]()
	elements := make(map[uint64]MMRElement)

	for i := uint64(1); i < 100; i++ {
		value := MMRElement(fmt.Sprintf("value%d", i))
		elements[i] = value
		memStorage.append(i, []MMRElement{value})
	}

	// Check all elements are in the right position
	for pos, expected := range elements {
		element, err := memStorage.getElement(pos)
		assert.NoError(t, err)
		assert.NotNil(t, element)
		assert.Equal(t, *element, expected)
	}
}

func TestGetNotFoundElement(t *testing.T) {
	memStorage := NewMemStorage[MMRElement]()

	element, err := memStorage.getElement(100)
	assert.NoError(t, err)
	assert.Nil(t, element)
}
