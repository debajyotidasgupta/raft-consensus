package main

import (
	"fmt"
	"raft-consensus/raft"
	"testing"
)

func Set(key string, value int, t *testing.T) error {
	cs := raft.CreateNewCluster(t, uint64(1))
	fmt.Println(cs)
	return nil
}
