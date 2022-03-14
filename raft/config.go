package raft

import "fmt"

// Basic Data Structure and Associated operations to
// Maintain cluster configuration

// Set implementation from https://golangbyexample.com/set-implementation-in-golang/
type Set struct {
	peerSet map[uint64]struct{}
}

func makeSet() Set {
	return Set{
		peerSet: make(map[uint64]struct{}),
	}
}

func (c *Set) Exists(key uint64) bool {
	_, exists := c.peerSet[key]
	return exists
}

func (c *Set) Add(key uint64) {
	c.peerSet[key] = struct{}{}
}

func (c *Set) Remove(key uint64) error {
	_, exists := c.peerSet[key]
	if !exists {
		return fmt.Errorf("Remove Error: Item doesn't exist in set")
	}
	delete(c.peerSet, key)
	return nil
}

func (c *Set) Size() int {
	return len(c.peerSet)
}
