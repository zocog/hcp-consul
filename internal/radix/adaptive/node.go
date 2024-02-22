// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: BUSL-1.1

package adaptive

import "sync"

type Node[T any] interface {
	getPartialLen() uint32
	setPartialLen(uint32)
	getArtNodeType() nodeType
	setArtNodeType(nodeType)
	getNumChildren() uint8
	setNumChildren(uint8)
	getPartial() []byte
	setPartial([]byte)
	isLeaf() bool
	Iterator() *Iterator[T]
	PathIterator([]byte) *PathIterator[T]
	matchPrefix([]byte) bool
	getChild(int) *Node[T]
	Clone() *Node[T]
}
