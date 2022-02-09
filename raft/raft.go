package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"os"
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
	Command interface{} // Command is the command to be committed
	Term    uint64      // Term is the term of the log entry
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

type RequestVoteArgs struct {
	Term         uint64 // Term is the term of the Raft node
	CandidateId  uint64 // CandidateId is the id of the Raft node that is requesting the vote
	LastLogIndex uint64 // LastLogIndex is the index of the last log entry
	LastLogTerm  uint64 // LastLogTerm is the term of the last log entry
}

type RequestVoteReply struct {
	Term        uint64 // Term is the term of the Raft node
	VoteGranted bool   // VoteGranted is true if the Raft node granted the vote
}

// debug logs a debug message if the debug level is set to DEBUG
func (rn *RaftNode) debug(format string, args ...interface{}) {
	if DEBUG > 0 {
		format = fmt.Sprintf("[%d] %s", rn.id, format)
		log.Printf(format, args...)
	}
}

// NewRaftNode  creates  a  new  Raft  node  with the given id, peers,
// server, database, and commit channel. The ready  channel is used to
// notify the caller that the peers have been initialized and the Raft
// node is ready to be started.

func NewRaftNode(id uint64, peers []uint64, server *Server, db *Database, ready chan interface{}, commitChan chan CommitEntry) *RaftNode {
	node := &RaftNode{
		id:                 id,                      // id is the id of the Raft node
		peers:              peers,                   // Peer is the list of peers in the Raft cluster
		server:             server,                  // Server is the server of the Raft node. Issue RPCs to the peers
		db:                 db,                      // Database is the storage of the Raft node
		commitChan:         commitChan,              // CommitChan is the channel the channel where this Raft Node is going to report committed log entries
		newCommitReady:     make(chan struct{}, 16), // NewCommitReady is an internal notification channel used to notify that new log entries may be sent on commitChan.
		trigger:            make(chan struct{}, 1),  // Trigger is the channel used to trigger the Raft node to send a AppendEntries RPC to the peers when some relevant event occurs
		currentTerm:        0,                       // CurrentTerm is the current term of the Raft node
		votedFor:           0,                       // VotedFor is the candidate id that received a vote in the current term
		log:                make([]LogEntry, 0),     // Log is the log of the Raft node
		commitIndex:        0,                       // CommitIndex is the index of the last committed log entry
		lastApplied:        0,                       // LastApplied is the index of the last applied log entry
		state:              Follower,                // State is the state of the Raft node
		electionResetEvent: time.Now(),              // ElectionResetEvent is the time at which the Raft node had last reset its election timer
		nextIndex:          make(map[uint64]uint64), // NextIndex is the index of the next log entry to send to each peer
		matchIndex:         make(map[uint64]uint64), // MatchIndex is the index of the highest log entry known to be replicated on the leader's peers
	}

	if node.db.HasData() {
		// If the database has data, load  the
		// currentTerm, votedFor, and log from
		// the database before crashing.

		node.restoreFromStorage()
	}

	// Start the Raft node
	go func() {
		<-ready                              // Wait for the peers to be initialized
		node.mu.Lock()                       // Lock the Raft node
		node.electionResetEvent = time.Now() // Reset the election timer
		node.mu.Unlock()                     // Unlock the Raft node
		node.runElectionTimer()              // Start the election timer
	}()

	go node.sendCommit() // Start the commit channel as a goroutine
	return node
}

// sendCommit  sends  committed  entries on commit channel.  It watches
// newCommitReady  for  notifications  and calculates which new entries
// are ready to be sent. This method should run in background goroutine
// Commit Channel  of  Node may be buffered and will limit how fast the
// client consumes new committed entries.

func (rn *RaftNode) sendCommit() {
	for range rn.newCommitReady {
		// Find which entries we have to apply.
		rn.mu.Lock()                       // Lock the Raft node
		savedTerm := rn.currentTerm        // Save the current term
		savedLastApplied := rn.lastApplied // Save the last applied index

		// Find the slice of entry that we have not applied yet.
		var entries []LogEntry               // Entries is the slice of log entries that we have not applied yet
		if rn.commitIndex > rn.lastApplied { // If the commit index is greater than the last applied index
			entries = rn.log[rn.lastApplied+1 : rn.commitIndex+1] // Entries is the slice of log entries that we have not applied yet
			rn.lastApplied = rn.commitIndex                       // Update the last applied index
		}
		rn.mu.Unlock() // Unlock the Raft node
		rn.debug("sendCommit entries=%v, savedLastApplied=%d", entries, savedLastApplied)

		// Send the entries to the commit channel one by one.
		for i, entry := range entries { // For each entry in the slice of log entries that we have not applied yet
			rn.commitChan <- CommitEntry{ // Send the entry to the commit channel
				Command: entry.Command,                    // Command is the command of the log entry
				Index:   savedLastApplied + uint64(i) + 1, // Index is the index of the log entry
				Term:    savedTerm,                        // Term is the term of the log entry
			}
		}
	}
	rn.debug("sendCommit completed")
}

func (rn *RaftNode) runElectionTimer() {
	timeoutDuration := rn.electionTimeout()
	rn.mu.Lock()
	termStarted := rn.currentTerm
	rn.mu.Unlock()
	rn.debug("Election Timer started (%v), term=%d", timeoutDuration, termStarted)

	/** The following loop will run until either:
	* [-] We discover the election timer is no longer  needed,  (or)
	* [-] the election timer expires and this RN becomes a candidate
	*
	* In a follower, this typically keeps running in the  background
	* for the duration of the node's lifetime.  The  ticker  ensures
	* that the node responds to any change in term, and the state is
	* same as the expected state. If anything is off,  we  terminate
	* the election timer.
	 */

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		<-ticker.C // Wait for the ticker to fire

		rn.mu.Lock()

		// If we are not a candidate or follower, we are
		// in no  need of election timer in leader state

		if rn.state != Candidate && rn.state != Follower {
			rn.debug("In election timer state=%s, bailing out", rn.state)
			rn.mu.Unlock()
			return
		}

		// If the term has changed, we are no longer needed
		//of the current election timer and we can bail out

		if termStarted != rn.currentTerm {
			rn.debug("in election timer term changed from %d to %d, bailing out", termStarted, rn.currentTerm)
			rn.mu.Unlock()
			return
		}

		// Start  an election if we haven't heard from a leader
		// or haven't voted for someone for the duration of the
		// timeout.

		if elapsed := time.Since(rn.electionResetEvent); elapsed >= timeoutDuration {
			rn.startElection()
			rn.mu.Unlock()
			return
		}
		rn.mu.Unlock()
	}
}

// electionTimeout generates a pseudo-random election timeout duration
// The  duration  is  chosen  randomly between 150ms and 300ms (Ref to
// https://raft.github.io/raft.pdf [Raft] Section 9.3 [ preformance ])
//
// If RAFT_FORCE_MORE_REELECTION is set, we stress-test by deliberately
// generating a hard-coded number very often (150ms in this case). This
// will  create  collisions  between  different  servers and force more
// re-elections.

func (rn *RaftNode) electionTimeout() time.Duration {
	if os.Getenv("RAFT_FORCE_MORE_REELECTION") == "true" && rand.Intn(3) > 0 {

		// Force a re-election every 150ms with probability 2/3
		return time.Duration(150) * time.Millisecond

	} else {

		// Generate a random election timeout between 150ms and 300ms
		return time.Duration(150+rand.Intn(150)) * time.Millisecond

	}
}

// startElection starts a new  election  with  the
// current Raft Node as a candidate. This function
// expects the mutex of the Node to be locked.

func (rn *RaftNode) startElection() {
	rn.state = Candidate               // Update the state to candidate
	rn.currentTerm += 1                // Increment the term
	savedCurrentTerm := rn.currentTerm // Save the current term
	rn.electionResetEvent = time.Now() // Set the Reset Event to current time
	rn.votedFor = rn.id                // Vote for self
	votesReceived := 1                 // We have already voted for self

	rn.debug("Becomes Candidate (currentTerm=%d); log=%v", savedCurrentTerm, rn.log)

	// Send RequestVote RPCs to all other peer servers concurrently.
	for _, peer := range rn.peers {
		go func(peer uint64) {
			rn.mu.Lock()
			savedLastLogIndex, savedLastLogTerm := rn.lastLogIndexAndTerm()
			rn.mu.Unlock()

			args := RequestVoteArgs{
				Term:         savedCurrentTerm,  // Current term
				CandidateId:  rn.id,             // Candidate's ID
				LastLogIndex: savedLastLogIndex, // Index of candidate's last log entry
				LastLogTerm:  savedLastLogTerm,  // Term of candidate's last log entry
			}

			rn.debug("Sending RequestVote to %d: %+v", peer, args)

			var reply RequestVoteReply // Reply from the server
			if err := rn.server.RPC(peer, "ConsensusModule.RequestVote", args, &reply); err == nil {
				rn.mu.Lock()         // Lock the Raft Node
				defer rn.mu.Unlock() // Unlock the Raft Node
				rn.debug("received RequestVoteReply %+v", reply)

				if rn.state != Candidate { // If we are no longer a candidate, bail out
					rn.debug("While waiting for reply, state = %v", rn.state)
					return
				}

				if reply.Term > savedCurrentTerm { // If the term is greater than ours, we are no longer a candidate
					rn.debug("Term out of date in RequestVoteReply")
					rn.becomeFollower(reply.Term) // Become a follower
					return
				} else if reply.Term == savedCurrentTerm {
					//	If the term is equal to ours, we need to check the vote
					if reply.VoteGranted {

						// If the vote is granted, increment the vote count
						votesReceived += 1                     // Increment the vote count
						if votesReceived*2 > len(rn.peers)+1 { // If we have majority votes, become leader

							// Won the election so become leader
							rn.debug("Wins election with %d votes", votesReceived)
							rn.becomeLeader() // Become leader
							return
						}
					}
				}
			}
		}(peer)
	}

	// Run another election timer, in case this election is not successful.
	go rn.runElectionTimer()
}

// becomeLeader switches Raft Node into a leader  state
// and begins process of heartbeats every 50ms.   This
// function expects the mutex of the Node to be locked

func (rn *RaftNode) becomeLeader() {
	rn.state = Leader // Update the state to leader

	for _, peer := range rn.peers {
		rn.nextIndex[peer] = uint64(len(rn.log)) + 1 // Initialize nextIndex for all peers with the last log index (leader) + 1
		rn.matchIndex[peer] = 0                      // No match index yet
	}

	rn.debug("becomes Leader; term=%d, nextIndex=%v, matchIndex=%v; log=%v", rn.currentTerm, rn.nextIndex, rn.matchIndex, rn.log)

	/**
	 * The following goroutine is the heart of the leader
	 * election. It sends AppendEntries RPCs to all peers
	 * in the  cluster  in any  of  the  following  cases:
	 *
	 * 1. There is something on the  trigger  channel  (OR)
	 * 2. Every 50ms, if no events occur on trigger channel
	 *
	 * The goroutine is terminated when the Raft Node  is  no
	 * longer a leader. This goroutine runs in the background
	 */

	go func(heartbeatTimeout time.Duration) {
		rn.leaderSendAEs() // Send AppendEntries RPCs to all peers to notify them of the leader

		t := time.NewTimer(heartbeatTimeout) // Create a new timer
		defer t.Stop()                       // Stop the timer when the goroutine terminates
		for {
			doSend := false
			select {
			case <-t.C:

				// CASE: Timer expired
				doSend = true             // If the timer expires, send an heartbeat
				t.Stop()                  // Stop the timer if we have something to send
				t.Reset(heartbeatTimeout) // Reset the timer to fire again after heartbeat timeout

			case _, ok := <-rn.trigger:

				// CASE: Trigger channel has something
				if ok {
					doSend = true // If the trigger channel has something, send an AppendEntries
				} else {
					return // If the trigger channel is closed, terminate the goroutine
				}
				if !t.Stop() { // Wait for the timer to stop 50ms for the next event
					<-t.C
				}
				t.Reset(heartbeatTimeout) // Reset the timer to fire again after heartbeat timeout
			}

			if doSend { // If we have something to send, send it
				rn.mu.Lock()            // Lock the mutex
				if rn.state != Leader { // If we are no longer a leader, bail out
					rn.mu.Unlock() // Unlock the mutex
					return         // Terminate the goroutine
				}
				rn.mu.Unlock()     // Unlock the mutex
				rn.leaderSendAEs() // Send AppendEntries to all peers
			}
		}
	}(50 * time.Millisecond)
}

func (cm *ConsensusModule) leaderSendAEs() {}

// lastLogIndexAndTerm  returns the index  of the last
// log and the last log entry's term (or 0  if there's
// no log) for this server. This function expects  the
// Node's mutex to be locked. The log index is 1 based
// hence the empty log is index 0.

func (rn *RaftNode) lastLogIndexAndTerm() (uint64, uint64) {
	if len(rn.log) > 0 {

		// Log is not empty
		lastIndex := uint64(len(rn.log)) // Index of last log entry (1 based)
		return lastIndex, rn.log[lastIndex-1].Term

	} else {

		// Empty log has index 0 and term 0
		return 0, 0

	}
}

// RequestVote Remote Procedure Call is invoked by  candidates
// to find out if they can win an  election.  The RPC  returns
// true if the candidate is running and has a higher term than
// the current term.

func (rn *RaftNode) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state == Dead { // If the node is dead, we don't need to process this request, since it is stale
		return nil
	}

	lastLogIndex, lastLogTerm := rn.lastLogIndexAndTerm() // Get the last log index and term
	rn.debug("RequestVote: %+v [currentTerm=%d, votedFor=%d, log index/term=(%d, %d)]", args, rn.currentTerm, rn.votedFor, lastLogIndex, lastLogTerm)

	if args.Term > rn.currentTerm {
		// If the term is out of date and becoming a follower
		// If it's already a follower, the state won't change
		// but the other state fields will reset

		rn.debug("Term out of date in RequestVote")
		rn.becomeFollower(args.Term)
	}

	/**
	 * If the candidate's log is at least as up-to-date as
	 * our last log entry, and the candidate's term is  at
	 * least as recent as ours, then we can grant the vote
	 */

	if rn.currentTerm == args.Term &&
		(rn.votedFor == 0 || rn.votedFor == args.CandidateId) &&
		(args.LastLogTerm > lastLogTerm || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)) {

		// If the caller's term is aligned with ours and
		// we haven't voted for  another  candidate  yet
		// we'll grant the vote. We  never  grant a vote
		// for RPCs from older terms

		reply.VoteGranted = true           // Grant the vote to the candidate
		rn.votedFor = args.CandidateId     // Remember who we voted for
		rn.electionResetEvent = time.Now() // Set the Reset Event to current time

	} else {

		// Deny the vote to the candidate
		reply.VoteGranted = false

	}
	reply.Term = rn.currentTerm // Set the term to the current term
	rn.persistToStorage()       // Persist the state to storage
	rn.debug("RequestVote reply: %+v", reply)
	return nil // Return nil error
}

// becomeFollower makes the current RaftNode  a
// follower and resets the state. This function
// expects  the  mutex of the Node to be locked

func (rn *RaftNode) becomeFollower(term uint64) {
	rn.debug("Becomes Follower with term=%d; log=%v", term, rn.log)

	rn.state = Follower                // Update the state to follower
	rn.currentTerm = term              // Update the term
	rn.votedFor = 0                    // Reset the votedFor to 0 [1 based index]
	rn.electionResetEvent = time.Now() // Set the Reset Event to current time

	go rn.runElectionTimer() // Run another election timer, since we transitioned to follower
}

// persistToStorage saves  all  of  Raft  Node's persistent
// state in Raft Node's database  /  non  volatile  storage
// This function expects the mutex of the Node to be locked

func (rn *RaftNode) persistToStorage() {
	// Persist the currentTerm, votedFor and log to storage

	for _, data := range []struct {
		name  string
		value interface{}
	}{{"currentTerm", rn.currentTerm}, {"votedFor", rn.votedFor}, {"log", rn.log}} {

		var buf bytes.Buffer        // Buffer to hold the data
		enc := gob.NewEncoder(&buf) // Create a new encoder

		if err := enc.Encode(data.value); err != nil { // Encode the data
			log.Fatal("encode error: ", err) // If there's an error, log it
		}
		rn.db.Set(data.name, buf.Bytes()) // Save the data to the database
	}
}

// restoreFromStorage saves  all  of  Raft  Node's persistent
// state in Raft Node's database  /  non  volatile  storage
// This function expects the mutex of the Node to be locked

func (rn *RaftNode) restoreFromStorage() {
	// Persist the currentTerm, votedFor and log to storage

	for _, data := range []struct {
		name  string
		value interface{}
	}{{"currentTerm", &rn.currentTerm}, {"votedFor", &rn.votedFor}, {"log", &rn.log}} {
		if value, found := rn.db.Get(data.name); found {

			// If the data is found in the database, decode it
			dec := gob.NewDecoder(bytes.NewBuffer(value))  // Create a new decoder
			if err := dec.Decode(data.value); err != nil { // Decode the data
				log.Fatal("decode error: ", err) // If there's an error, log it
			}

		} else {

			// If the data is not found in the database, initialize it
			log.Fatal("No data found for %s", data.name)

		}
	}
}

// String returns a string representation of the Raft node state.
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
