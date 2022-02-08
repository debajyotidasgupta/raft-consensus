package raft

import (
	"sync"
)

type RNState int // RNState is the state of the Raft node

const (
	Follower  RNState = iota // Follower is the state of a Raft node that is a follower
	Candidate                // Candidate is the state of a Raft node that is a candidate
	Leader                   // Leader is the state of a Raft node that is a leader
	Dead                     // Dead is the state of a Raft node that is dead
)

type CommitEntry struct {
	Command interface{} // Command is the command to be committed
	Term    uint64      // Term is the term of the command
	Index   uint64      // Index is the index of the command
}

type LogEntry struct {
	Command interface{}
	Term    uint64 // Term is the term of the log entry
}

type RaftNode struct {
	id         uint64           // id is the id of the Raft node
	mu         sync.Mutex       // Mutex protects the Raft node
	state      RNState          // State is the state of the Raft node
	peers      []uint64         // Peer is the list of peers in the Raft cluster
	server     *Server          // Server is the server of the Raft node. Issue RPCs to the peers
	db         Database         // Database is the storage of the Raft node
	commitChan chan CommitEntry // CommitChan is the channel the channel where this Raft Node is going to report committed log entries
	newCommit  chan CommitEntry // NewCommit is an internal notification channel used to notify that new log entries may be sent on commitChan.
	trigger    chan struct{}    // Trigger is the channel used to trigger the Raft node to send a AppendEntries RPC to the peers when some relevant event occurs

	// Persistent state on all servers
	currentTerm uint64     // CurrentTerm is the current term of the Raft node
	votedFor    uint64     // VotedFor is the candidate id that received a vote in the current term
	log         []LogEntry // Log is the log of the Raft node

	// Volatile state on all servers
	commitIndex uint64   // CommitIndex is the index of the last committed log entry
	lastApplied uint64   // LastApplied is the index of the last applied log entry
	nextIndex   []uint64 // NextIndex is the index of the next log entry to send to each peer
}

func (s RNState) String() string { // String returns the string representation of a Raft node state
	switch s {
	case Follower:
		return "Follower"
	case Candidate:
		return "Candidate"
	case Leader:
		return "Leader"
	case Dead:
		return "Dead"
	default: // Should never happen
		panic("Error: Unknown state")
	}
}