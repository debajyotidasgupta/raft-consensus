package main

import (
	"fmt"
	"raft-consensus/raft"
)

func Set(cs *raft.ClusterSimulator, serverId uint64, key string, value int) error {
	writeCommand := raft.Write{Key: key, Val: value}
	isLeader := cs.SubmitToServer(int(serverId), writeCommand)
	if !isLeader {
		return fmt.Errorf("server not leader")
	}
	return nil
}
