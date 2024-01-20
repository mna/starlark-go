// Copyright 2017 The Bazel Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package starlark

import (
	"fmt"
	"math/big"
)

// hashtable is used to represent Starlark dict and set values.
// It is a hash table whose key/value entries form a doubly-linked list
// in the order the entries were inserted.
//
// Initialized instances of hashtable must not be copied.
type hashtable struct {
	m         map[Value]Value
	itercount uint32 // number of active iterators (ignored if frozen)
	frozen    bool

	_ noCopy // triggers vet copylock check on this type.
}

// noCopy is zero-sized type that triggers vet's copylock check.
// See https://github.com/golang/go/issues/8005#issuecomment-190753527.
type noCopy struct{}

func (*noCopy) Lock()   {}
func (*noCopy) Unlock() {}

func (ht *hashtable) init(size int) {
	if size < 0 {
		panic("size < 0")
	}
	ht.m = make(map[Value]Value, size)
}

func (ht *hashtable) freeze() {
	if !ht.frozen {
		ht.frozen = true
		for k, v := range ht.m {
			k.Freeze()
			v.Freeze()
		}
	}
}

func (ht *hashtable) insert(k, v Value) error {
	if err := ht.checkMutable("insert into"); err != nil {
		return err
	}
	if ht.m == nil {
		ht.init(1)
	}
	ht.m[k] = v
	return nil
}

func (ht *hashtable) lookup(k Value) (v Value, found bool, err error) {
	if ht.m == nil {
		return None, false, nil // empty
	}
	// TODO: handle k being not hashable and return error? currently would panic
	v, ok := ht.m[k]
	return v, ok, nil
}

// count returns the number of distinct elements of iter that are elements of ht.
func (ht *hashtable) count(iter Iterator) (int, error) {
	if ht.m == nil {
		return 0, nil // empty
	}

	var k Value
	count := 0

	// Use a bitset per table entry to record seen elements of ht.
	// Elements are identified by their bucket number and index within the bucket.
	// Each bitset gets one word initially, but may grow.
	storage := make([]big.Word, len(ht.table))
	bitsets := make([]big.Int, len(ht.table))
	for i := range bitsets {
		bitsets[i].SetBits(storage[i : i+1 : i+1])
	}
	for iter.Next(&k) && count != int(ht.len) {
		h, err := k.Hash()
		if err != nil {
			return 0, err // unhashable
		}
		if h == 0 {
			h = 1 // zero is reserved
		}

		// Inspect each bucket in the bucket list.
		bucketId := h & (uint32(len(ht.table) - 1))
		i := 0
		for p := &ht.table[bucketId]; p != nil; p = p.next {
			for j := range p.entries {
				e := &p.entries[j]
				if e.hash == h {
					if eq, err := Equal(k, e.key); err != nil {
						return 0, err
					} else if eq {
						bitIndex := i<<3 + j
						if bitsets[bucketId].Bit(bitIndex) == 0 {
							bitsets[bucketId].SetBit(&bitsets[bucketId], bitIndex, 1)
							count++
						}
					}
				}
			}
			i++
		}
	}

	return count, nil
}

// Items returns all the items in the map (as key/value pairs) in insertion order.
func (ht *hashtable) items() []Tuple {
	items := make([]Tuple, 0, ht.len)
	array := make([]Value, ht.len*2) // allocate a single backing array
	for e := ht.head; e != nil; e = e.next {
		pair := Tuple(array[:2:2])
		array = array[2:]
		pair[0] = e.key
		pair[1] = e.value
		items = append(items, pair)
	}
	return items
}

func (ht *hashtable) first() (Value, bool) {
	if ht.head != nil {
		return ht.head.key, true
	}
	return None, false
}

func (ht *hashtable) keys() []Value {
	keys := make([]Value, 0, ht.len)
	for e := ht.head; e != nil; e = e.next {
		keys = append(keys, e.key)
	}
	return keys
}

func (ht *hashtable) delete(k Value) (v Value, found bool, err error) {
	if err := ht.checkMutable("delete from"); err != nil {
		return nil, false, err
	}
	if ht.table == nil {
		return None, false, nil // empty
	}
	h, err := k.Hash()
	if err != nil {
		return nil, false, err // unhashable
	}
	if h == 0 {
		h = 1 // zero is reserved
	}

	// Inspect each bucket in the bucket list.
	for p := &ht.table[h&(uint32(len(ht.table)-1))]; p != nil; p = p.next {
		for i := range p.entries {
			e := &p.entries[i]
			if e.hash == h {
				if eq, err := Equal(k, e.key); err != nil {
					return nil, false, err
				} else if eq {
					// Remove e from doubly-linked list.
					*e.prevLink = e.next
					if e.next == nil {
						ht.tailLink = e.prevLink // deletion of last entry
					} else {
						e.next.prevLink = e.prevLink
					}

					v := e.value
					*e = entry{}
					ht.len--
					return v, true, nil // found
				}
			}
		}
	}

	// TODO(adonovan): opt: remove completely empty bucket from bucket list.

	return None, false, nil // not found
}

// checkMutable reports an error if the hash table should not be mutated.
// verb+" dict" should describe the operation.
func (ht *hashtable) checkMutable(verb string) error {
	if ht.frozen {
		return fmt.Errorf("cannot %s frozen hash table", verb)
	}
	if ht.itercount > 0 {
		return fmt.Errorf("cannot %s hash table during iteration", verb)
	}
	return nil
}

func (ht *hashtable) clear() error {
	if err := ht.checkMutable("clear"); err != nil {
		return err
	}
	if ht.table != nil {
		for i := range ht.table {
			ht.table[i] = bucket{}
		}
	}
	ht.head = nil
	ht.tailLink = &ht.head
	ht.len = 0
	return nil
}

func (ht *hashtable) addAll(other *hashtable) error {
	for e := other.head; e != nil; e = e.next {
		if err := ht.insert(e.key, e.value); err != nil {
			return err
		}
	}
	return nil
}

// dump is provided as an aid to debugging.
//
//nolint:unused
func (ht *hashtable) dump() {
	fmt.Printf("hashtable %p len=%d head=%p tailLink=%p",
		ht, ht.len, ht.head, ht.tailLink)
	if ht.tailLink != nil {
		fmt.Printf(" *tailLink=%p", *ht.tailLink)
	}
	fmt.Println()
	for j := range ht.table {
		fmt.Printf("bucket chain %d\n", j)
		for p := &ht.table[j]; p != nil; p = p.next {
			fmt.Printf("bucket %p\n", p)
			for i := range p.entries {
				e := &p.entries[i]
				fmt.Printf("\tentry %d @ %p hash=%d key=%v value=%v\n",
					i, e, e.hash, e.key, e.value)
				fmt.Printf("\t\tnext=%p &next=%p prev=%p",
					e.next, &e.next, e.prevLink)
				if e.prevLink != nil {
					fmt.Printf(" *prev=%p", *e.prevLink)
				}
				fmt.Println()
			}
		}
	}
}

func (ht *hashtable) iterate() *keyIterator {
	if !ht.frozen {
		ht.itercount++
	}
	return &keyIterator{ht: ht, e: ht.head}
}

type keyIterator struct {
	ht *hashtable
	e  *entry
}

func (it *keyIterator) Next(k *Value) bool {
	if it.e != nil {
		*k = it.e.key
		it.e = it.e.next
		return true
	}
	return false
}

func (it *keyIterator) Done() {
	if !it.ht.frozen {
		it.ht.itercount--
	}
}
