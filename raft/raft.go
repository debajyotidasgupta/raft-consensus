package raft

import (
	"fmt"
	"log"
	"sync"
	"time"
)

type RNState int // RNState is the state of the Raft node

const DEBUG = 1 // DEBUG is the debug level
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
	Command  interface{}
	Term uint64 // Term is the term of the log entry
}

type RaftNode struct {
	id             uint64           // id is the id of the Raft node
	mu             sync.Mutex       // Mutex protects the Raft node
	peers          []uint64         // Peer is the list of peers in the Raft cluster
	server         *Server          // Server is the server of the Raft node. Issue RPCs to the peers
	db             *Database        // Database is the storage of the Raft node
	commitChan     chan CommitEntry // CommitChan is the channel the channel where this Raft Node is going to report committed log entries
	newCommitReady chan struct{}    // NewCommitReady is an internal notification channel used to notify that new log entries may be sent on commitChan.
	trigger        chan struct{}    // Trigger is the channel used to trigger the Raft node to send a AppendEntries RPC to the peers when some relevant event occurs

	// Persistent state on all servers
	currentTerm uint64     // CurrentTerm is the current term of the Raft node
	votedFor    uint64     // VotedFor is the candidate id that received a vote in the current term
	log         []LogEntry // Log is the log of the Raft node

	// IMPORTANT: Use 1 based indexing for log entries

	// Volatile state on all servers
	commitIndex        uint64    // CommitIndex is the index of the last committed log entry
	lastApplied        uint64    // LastApplied is the index of the last applied log entry
	state              RNState   // State is the state of the Raft node
	electionResetEvent time.Time // ElectionResetEvent is the time at which the Raft node had last reset its election timer

	// Volatile state on leaders
	nextIndex  map[uint64]uint64 // NextIndex is the index of the next log entry to send to each peer
	matchIndex map[uint64]uint64 // MatchIndex is the index of the highest log entry known to be replicated on the leader's peers
}

// NewRaftNode  creates  a  new  Raft  node  with the given id, peers,
// server, database, and commit channel. The ready  channel is used to
// notify the caller that the peers have been initialized and the Raft
// node is ready to be started.

func NewRaftNode(id uint64, peers []uint64, server *Server, db *Database, ready chan interface{}, commitChan chan CommitEntry) *RaftNode {
	node := &RaftNode{
		id:                 id,
		peers:              peers,
		server:             server,
		db:                 db,
		commitChan:         commitChan,
		newCommitReady:     make(chan struct{}, 16),
		trigger:            make(chan struct{}, 1),
		currentTerm:        0,
		votedFor:           0,
		log:                make([]LogEntry, 0),
		commitIndex:        0,
		lastApplied:        0,
		state:              Follower,
		electionResetEvent: time.Now(),
		nextIndex:          make(map[uint64]uint64),
		matchIndex:         make(map[uint64]uint64),
	}

	// Start the Raft node
	go func() {
		<-ready
		node.mu.Lock()
		defer node.mu.Unlock()
		node.electionResetEvent = time.Now()
	}()

	go node.sendCommit()
	return node
}

// debug logs a debug message if the debug level is set to DEBUG
func (rn *RaftNode) debug(format string, args ...interface{}) {
	if DEBUG > 0 {
		format = fmt.Sprintf("[%d] %s", rn.id, format)
		log.Printf(format, args...)
	}
}

// sendCommit  sends  committed  entries on commit channel.  It watches
// newCommitReady  for  notifications  and calculates which new entries
// are ready to be sent. This method should run in background goroutine
// Commit Channel  of  Node may be buffered and will limit how fast the
// client consumes new committed entries.

func (rn *RaftNode) sendCommit() {
	for range rn.newCommitReady {
		// Find which entries we have to apply.
		rn.mu.Lock()
		savedTerm := rn.currentTerm
		savedLastApplied := rn.lastApplied

		// Find the slice of entry that we have not applied yet.
		var entries []LogEntry
		if rn.commitIndex > rn.lastApplied {
			entries = rn.log[rn.lastApplied+1 : rn.commitIndex+1]
			rn.lastApplied = rn.commitIndex
		}
		rn.mu.Unlock()
		rn.debug("sendCommit entries=%v, savedLastApplied=%d", entries, savedLastApplied)

		// Send the entries to the commit channel one by one.
		for i, entry := range entries {
			rn.commitChan <- CommitEntry{
				Command: entry.Command,
				Index:   savedLastApplied + uint64(i) + 1,
				Term:    savedTerm,
			}
		}
	}
	rn.debug("sendCommit completed")
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
