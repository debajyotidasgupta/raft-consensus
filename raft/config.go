package raft

type Configuration struct {
	///peerIds []uint64
}

// As per paper, new configuration request first goes to leader
// Say a new node gets added to the cluster
// Should all nodes connect to this node and vice versa
// before the configuration change request goes to the leader?
// or connection can happen at any point of time?
