package raft

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"
)

const DEBUG = 1  // DEBUG is the debug level
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
	Command interface{} // Command is the command to be committed
	Term    uint64      // Term is the term of the log entry
}

type RaftNode struct {
	id             uint64           // id is the id of the Raft node
	mu             sync.Mutex       // Mutex protects the Raft node
	peerList       Set              // Peer is the list of peers in the Raft cluster
	server         *Server          // Server is the server of the Raft node. Issue RPCs to the peers
	db             *Database        // Database is the storage of the Raft node
	commitChan     chan CommitEntry // CommitChan is the channel the channel where this Raft Node is going to report committed log entries
	newCommitReady chan struct{}    // NewCommitReady is an internal notification channel used to notify that new log entries may be sent on commitChan.
	trigger        chan struct{}    // Trigger is the channel used to trigger the Raft node to send a AppendEntries RPC to the peers when some relevant event occurs

	// Persistent state on all servers
	currentTerm uint64     // CurrentTerm is the current term of the Raft node
	votedFor    int        // VotedFor is the candidate id that received a vote in the current term
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

type AppendEntriesArgs struct {
	Term     uint64 // Term is the term of the Raft node
	LeaderId uint64 // LeaderId is the id of the Raft node that is sending the AppendEntries RPC

	PrevLogIndex uint64     // PrevLogIndex is the index of the log entry immediately preceding the new ones
	PrevLogTerm  uint64     // PrevLogTerm is the term of the log entry immediately preceding the new ones
	Entries      []LogEntry // Entries is the slice of log entries to be appended
	LeaderCommit uint64     // LeaderCommit is the index of the log entry to be committed
}

type AppendEntriesReply struct {
	Term    uint64 // Term is the term of the Raft node
	Success bool   // true if follower contained entry matching prevLogIndex and prevLogTerm

	// Faster conflict resolution optimization
	// (described  near the end of section 5.3
	// in the  https://raft.github.io/raft.pdf
	// [RAFT] paper)

	ConflictIndex uint64 // ConflictIndex is the index of the conflicting log entry
	ConflictTerm  uint64 // ConflictTerm is the term of the conflicting log entry
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

func NewRaftNode(id uint64, peerList Set, server *Server, db *Database, ready <-chan interface{}, commitChan chan CommitEntry) *RaftNode {
	node := &RaftNode{
		id:                 id,                      // id is the id of the Raft node
		peerList:           peerList,                // List of peers of this Raft node
		server:             server,                  // Server is the server of the Raft node. Issue RPCs to the peers
		db:                 db,                      // Database is the storage of the Raft node
		commitChan:         commitChan,              // CommitChan is the channel the channel where this Raft Node is going to report committed log entries
		newCommitReady:     make(chan struct{}, 16), // NewCommitReady is an internal notification channel used to notify that new log entries may be sent on commitChan.
		trigger:            make(chan struct{}, 1),  // Trigger is the channel used to trigger the Raft node to send a AppendEntries RPC to the peers when some relevant event occurs
		currentTerm:        0,                       // CurrentTerm is the current term of the Raft node
		votedFor:           -1,                      // VotedFor is the candidate id that received a vote in the current term
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
			entries = rn.log[rn.lastApplied:rn.commitIndex] // Entries is the slice of log entries that we have not applied yet
			rn.lastApplied = rn.commitIndex                 // Update the last applied index
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
	rn.votedFor = int(rn.id)           // Vote for self
	votesReceived := 1                 // We have already voted for self

	rn.debug("Becomes Candidate (currentTerm=%d); log=%v", savedCurrentTerm, rn.log)

	// Check majority, helpful in case of only 1 node in cluster
	go func() {
		rn.mu.Lock()
		defer rn.mu.Unlock()

		if rn.state != Candidate {
			rn.debug("while waiting for majority, state = %v", rn.state)
			return
		}
		if votesReceived*2 > rn.peerList.Size()+1 { // If we have majority votes, become leader

			// Won the election so become leader
			rn.debug("Wins election with %d votes", votesReceived)
			rn.becomeLeader() // Become leader
			return
		}
	}()

	// Send RequestVote RPCs to all other peer servers concurrently.
	for peer := range rn.peerList.peerSet {
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
			if err := rn.server.RPC(peer, "RaftNode.RequestVote", args, &reply); err == nil {
				rn.mu.Lock()         // Lock the Raft Node
				defer rn.mu.Unlock() // Unlock the Raft Node
				rn.debug("received RequestVoteReply %+v from %v", reply, peer)

				if rn.state != Candidate { // If we are no longer a candidate, bail out
					rn.debug("While waiting for reply, state = %v", rn.state)
					return
				}

				if reply.Term > savedCurrentTerm { // If the term is greater than ours, we are no longer a candidate
					rn.debug("Term out of date in RequestVoteReply from %v", peer)
					rn.becomeFollower(reply.Term) // Become a follower
					return
				} else if reply.Term == savedCurrentTerm {
					//	If the term is equal to ours, we need to check the vote
					/*fmt.Println("Candi Id: ", rn.id)
					fmt.Println(peer)
					fmt.Println(reply)*/
					if reply.VoteGranted {

						// If the vote is granted, increment the vote count
						votesReceived += 1                          // Increment the vote count
						if votesReceived*2 > rn.peerList.Size()+1 { // If we have majority votes, become leader

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

	for peer := range rn.peerList.peerSet {
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

// leaderSendAEs sends AppendEntries RPCs to all peers
// in the cluster, collects responses, and updates the
// state  of  the Raft Node accordingly.

func (rn *RaftNode) leaderSendAEs() {
	rn.mu.Lock()                       // Lock the mutex
	savedCurrentTerm := rn.currentTerm // Save the current term
	rn.mu.Unlock()                     // Unlock the mutex

	// handling the case for a single node cluster
	go func(peer uint64) {
		if rn.peerList.Size() == 0 {
			if uint64(len(rn.log)) > rn.commitIndex {
				savedCommitIndex := rn.commitIndex
				for i := rn.commitIndex + 1; i <= uint64(len(rn.log)); i++ {
					if rn.log[i-1].Term == rn.currentTerm { //	If the term is the same as the current term, update the commit index
						rn.commitIndex = i

					}
				}
				if savedCommitIndex != rn.commitIndex {
					rn.debug("Leader sets commitIndex := %d", rn.commitIndex)
					// Commit index changed:  the  leader considers new  entries
					// to be committed. Send  new  entries on the commit channel
					// to this leader's clients, and notify followers by sending
					// them Append Entries.

					rn.newCommitReady <- struct{}{}
					rn.trigger <- struct{}{}
				}
			}
		}
	}(rn.id)

	for peer := range rn.peerList.peerSet {

		// The following goroutine is used to send AppendEntries RPCs
		// to one peer  in  the  cluster  and  collect  responses  to
		// determine and update the current state  of  the  Raft Node

		go func(peer uint64) {
			rn.mu.Lock()                       //	Lock the mutex
			nextIndex := rn.nextIndex[peer]    //	Get the next index for this peer
			prevLogIndex := int(nextIndex) - 1 //	Get the previous log index for this peer
			prevLogTerm := uint64(0)           //	Get the previous log term for this peer

			if prevLogIndex > 0 {
				//	If the previous log index is greater than 0, get the previous log term from the log
				prevLogTerm = rn.log[uint64(prevLogIndex)-1].Term
			}
			entries := rn.log[int(nextIndex)-1:] //	Get the entries for this peer

			args := AppendEntriesArgs{
				Term:         savedCurrentTerm,     //	Get the current term
				LeaderId:     rn.id,                //	Get the id of the leader
				PrevLogIndex: uint64(prevLogIndex), //	Get the previous log index
				PrevLogTerm:  prevLogTerm,          //	Get the previous log term
				Entries:      entries,              //	Get the entries
				LeaderCommit: rn.commitIndex,       //	Get the leader commit index
			}

			rn.mu.Unlock() //	Unlock the mutex before sending the RPC
			rn.debug("sending AppendEntries to %v: ni=%d, args=%+v", peer, nextIndex, args)

			var reply AppendEntriesReply
			if err := rn.server.RPC(peer, "RaftNode.AppendEntries", args, &reply); err == nil {
				rn.mu.Lock()                       //	Lock the mutex before updating the state
				defer rn.mu.Unlock()               //	Unlock the mutex after updating the state
				if reply.Term > savedCurrentTerm { //	If the reply term is greater than the current term, update the current term

					rn.debug("Term out of date in heartbeat reply")
					rn.becomeFollower(reply.Term) //	Update the state to follower since the term is out of date
					return
				}

				if rn.state == Leader && savedCurrentTerm == reply.Term { //	If we are still a leader and the term is the same, update the next index and match index
					if reply.Success { // If follower contained entry matching prevLogIndex and prevLogTerm
						rn.nextIndex[peer] = nextIndex + uint64(len(entries)) //	Update the next index
						rn.matchIndex[peer] = rn.nextIndex[peer] - 1          //	Update the match index

						savedCommitIndex := rn.commitIndex //	Save the current commit index
						for i := rn.commitIndex + 1; i <= uint64(len(rn.log)); i++ {
							if rn.log[i-1].Term == rn.currentTerm { //	If the term is the same as the current term, update the commit index
								matchCount := 1 //	Initialize the match count to single match

								for peer := range rn.peerList.peerSet {
									if rn.matchIndex[peer] >= i {

										// If  the  match  index  is greater than or equal
										// to the current index, increment the match count
										matchCount++
									}
								}
								if matchCount*2 > rn.peerList.Size()+1 {

									// If the match count is greater than the
									// number of peers plus 1,  that  is  got
									// the majority,  update the commit index
									rn.commitIndex = i
								}
							}
						}
						rn.debug("AppendEntries reply from %d success: nextIndex := %v, matchIndex := %v; commitIndex := %d", peer, rn.nextIndex, rn.matchIndex, rn.commitIndex)

						if rn.commitIndex != savedCommitIndex {
							rn.debug("Leader sets commitIndex := %d", rn.commitIndex)
							// Commit index changed:  the  leader considers new  entries
							// to be committed. Send  new  entries on the commit channel
							// to this leader's clients, and notify followers by sending
							// them Append Entries.

							rn.newCommitReady <- struct{}{}
							rn.trigger <- struct{}{}
						}
					} else {
						// Success is false: follower contained conflicting entry

						if reply.ConflictTerm > 0 {
							lastIndexOfTerm := uint64(0) //	Initialize the last index of the term to 0
							for i := uint64(len(rn.log)); i > 0; i-- {
								if rn.log[i-1].Term == reply.ConflictTerm {
									// If the term is the same as the conflict term, update the last index of the term
									lastIndexOfTerm = i
									break
								}
							}
							if lastIndexOfTerm > 0 {
								// If there are entries after the conflicting entry in the log
								// update the next index to the index of the last entry in the
								rn.nextIndex[peer] = lastIndexOfTerm + 1
							} else {
								// If there are no entries after the conflicting entry in the log
								// update the next index to the index  of the  conflicting  entry
								rn.nextIndex[peer] = reply.ConflictIndex
							}
						} else {
							// Success is false and conflict term is 0: follower contained conflicting entry
							rn.nextIndex[peer] = reply.ConflictIndex
						}
						rn.debug("AppendEntries reply from %d !success: nextIndex := %d", peer, nextIndex-1)
					}
				}
			}
		}(peer)
	}
}

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

// AppendEntries  is  the  RPC  handler  for  AppendEntries
// RPCs. This function is used to send entries to followers
// This function expects the  Node's  mutex  to  be  locked

func (rn *RaftNode) AppendEntries(args AppendEntriesArgs, reply *AppendEntriesReply) error {
	rn.mu.Lock()                                                // Lock the mutex before updating the state
	defer rn.mu.Unlock()                                        // Unlock the mutex after updating the state
	if rn.state == Dead || !rn.peerList.Exists(args.LeaderId) { // If the node is dead, return false
		return nil //	Return no error
	}
	rn.debug("AppendEntries: %+v", args)

	if args.Term > rn.currentTerm { // If the term is greater than the current term, update the current term
		rn.debug("Term out of date in AppendEntries")
		rn.becomeFollower(args.Term) //	Update the state to follower since the term is out of date
	}

	reply.Success = false //	Initialize the reply to false
	if args.Term == rn.currentTerm {
		if rn.state != Follower {

			// Raft guarantees that only a single leader exists  in
			// any given term. If we  carefully follow the logic of
			// RequestVote and the code in startElection that sends
			// RVs, we'll see that two leaders can't exist  in  the
			// cluster  with  the  same  term.  This  condition  is
			// important for candidates that find out that  another
			// peer won the election for this term.

			rn.becomeFollower(args.Term) //	Update the state to follower since it received an AE from a leader
		}
		rn.electionResetEvent = time.Now() //	Reset the election timer

		// Does  our log contain an entry at  PrevLogIndex whose
		// term  matches PrevLogTerm? Note  that in the  extreme
		// case of PrevLogIndex=0 (empty) this is vacuously true

		if args.PrevLogIndex == 0 ||
			(args.PrevLogIndex <= uint64(len(rn.log)) && args.PrevLogTerm == rn.log[args.PrevLogIndex-1].Term) {
			reply.Success = true

			// Find an insertion point - where there's a term mismatch between
			// the existing log starting at PrevLogIndex+1 and the new entries
			// sent in the RPC.

			logInsertIndex := args.PrevLogIndex + 1
			newEntriesIndex := uint64(1)

			for {
				if logInsertIndex > uint64(len(rn.log)) || newEntriesIndex > uint64(len(args.Entries)) {
					break
				}
				if rn.log[logInsertIndex-1].Term != args.Entries[newEntriesIndex-1].Term {
					break
				}
				logInsertIndex++
				newEntriesIndex++
			}

			/**
			* At the end of this  loop  (considering  1  based
			* indexing), the following will hold:
			*
			* [-] logInsertIndex points at the  end of the log
			*     or an index where the term  mismatches  with
			*     an entry from the leader
			*
			* [-] newEntriesIndex points at the end of Entries
			*     or an index where the term  mismatches  with
			*     the corresponding log entry
			 */

			if newEntriesIndex <= uint64(len(args.Entries)) {
				// If the newEntriesIndex is less than the length of the entries, append the entries
				rn.debug("Inserting entries %v from index %d", args.Entries[newEntriesIndex-1:], logInsertIndex)
				rn.log = append(rn.log[:logInsertIndex-1], args.Entries[newEntriesIndex-1:]...) //	Insert the new entries
				// Add the code to Update Config
				// Add code to establish/remove connections

				// loop over the new entries to check if any is for cluster change
				for _, entry := range args.Entries[newEntriesIndex-1:] {
					cmd := entry.Command
					switch v := cmd.(type) {
					case AddServers:
						for _, peerId := range v.ServerIds {
							if rn.id == uint64(peerId) {
								continue
							}
							rn.peerList.Add(uint64(peerId)) // add new server id to the peerList
						}
					case RemoveServers:
						for _, peerId := range v.ServerIds {
							rn.peerList.Remove(uint64(peerId)) // remove old server id from the peerList
						}
					}
				}

				rn.debug("Log is now: %v", rn.log)
			}

			// Set commit index, if the leader's commit index is greater than the length of the log, set it to the length of the log
			if args.LeaderCommit > rn.commitIndex {
				rn.commitIndex = uint64(math.Min(float64(args.LeaderCommit), float64(len(rn.log)))) //	Update the commit index
				rn.debug("Setting commitIndex=%d", rn.commitIndex)
				rn.newCommitReady <- struct{}{} //	Signal that a new commit index is ready
			}
		} else {
			// No match for PrevLogIndex or PrevLogTerm. Populate
			// ConflictIndex or ConflictTerm to help  the  leader
			// bring us up to date quickly. Success is  false  in
			// this case.

			if args.PrevLogIndex > uint64(len(rn.log)) {
				reply.ConflictIndex = uint64(len(rn.log)) + 1 //	If the PrevLogIndex is greater than the length of the log, set the conflict index to the length of the log
				reply.ConflictTerm = 0                        //	Set the conflict term to 0
			} else {
				// PrevLogIndex points within our log
				// but  PrevLogTerm  does  not  match
				// rn.log[PrevLogIndex-1].

				reply.ConflictTerm = rn.log[args.PrevLogIndex-1].Term

				var cfi uint64
				for cfi = args.PrevLogIndex - 1; cfi > 0; cfi-- {
					if rn.log[cfi-1].Term != reply.ConflictTerm {
						break //	Break out of the loop when the term mismatches
					}
				}
				reply.ConflictIndex = cfi + 1 //	Set the conflict index to the index of the first entry with a with the same term as the conflict term
			}
		}
	}

	reply.Term = rn.currentTerm //	Set the term in the reply to the current term
	rn.persistToStorage()       //	Persist the state to storage
	rn.debug("AppendEntries reply: %+v", *reply)
	return nil //	Return no error
}

// RequestVote Remote Procedure Call is invoked by  candidates
// to find out if they can win an  election.  The RPC  returns
// true if the candidate is running and has a higher term than
// the current term.

func (rn *RaftNode) RequestVote(args RequestVoteArgs, reply *RequestVoteReply) error {
	rn.mu.Lock()
	defer rn.mu.Unlock()

	if rn.state == Dead || !rn.peerList.Exists(args.CandidateId) { // If the node is dead, we don't need to process this request, since it is stale
		return nil
	}

	lastLogIndex, lastLogTerm := rn.lastLogIndexAndTerm() // Get the last log index and term
	rn.debug("RequestVote: %+v [currentTerm=%d, votedFor=%d, log index/term=(%d, %d)]", args, rn.currentTerm, rn.votedFor, lastLogIndex, lastLogTerm)

	if args.Term > rn.currentTerm {
		// If the term is out of date and becoming a follower
		// If it's already a follower, the state won't change
		// but the other state fields will reset

		rn.debug("Term out of date with term in RequestVote")
		rn.becomeFollower(args.Term)
	}

	/**
	 * If the candidate's log is at least as up-to-date as
	 * our last log entry, and the candidate's term is  at
	 * least as recent as ours, then we can grant the vote
	 */

	if rn.currentTerm == args.Term &&
		(rn.votedFor == -1 || rn.votedFor == int(args.CandidateId)) &&
		(args.LastLogTerm > lastLogTerm || (args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)) {

		// If the caller's term is aligned with ours and
		// we haven't voted for  another  candidate  yet
		// we'll grant the vote. We  never  grant a vote
		// for RPCs from older terms

		reply.VoteGranted = true            // Grant the vote to the candidate
		rn.votedFor = int(args.CandidateId) // Remember who we voted for
		rn.electionResetEvent = time.Now()  // Set the Reset Event to current time

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
	rn.votedFor = -1                   // Reset the votedFor to 0 [0 based index]
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

			// If the data is not found in the database
			log.Fatal("No data found for", data.name)

		}
	}
}

func (rn *RaftNode) readFromStorage(key string, reply interface{}) error {
	if value, found := rn.db.Get(key); found {
		// If the data is found in the database, decode it
		dec := gob.NewDecoder(bytes.NewBuffer(value)) // Create a new decoder
		if err := dec.Decode(reply); err != nil {     // Decode the data
			return err
		}
		return nil
	} else {
		err := fmt.Errorf("KeyNotFound:%v", key)
		return err
	}
}

// Submit submits a new command from the client to  the RaftNode.  This
// function doesn't block; clients read the commit  channel  passed  in
// the constructor to be notified of new committed entries. It  returns
// true iff this Raft Node is the leader - in which case the command is
// accepted.  If  false  is  returned,  the  client will have to find a
// different RaftNode to submit this command to.

func (rn *RaftNode) Submit(command interface{}) (bool, interface{}, error) {
	rn.mu.Lock() // Lock the mutex
	rn.debug("Submit received by %v: %v", rn.state, command)

	// Process the command only if the node is a leader
	if rn.state == Leader {
		switch v := command.(type) {
		case Read:
			key := v.Key
			var value int
			readErr := rn.readFromStorage(key, &value)
			rn.mu.Unlock()
			return true, value, readErr
		case AddServers:
			serverIds := v.ServerIds
			for i := 0; i < len(serverIds); i++ {
				if rn.peerList.Exists(uint64(serverIds[i])) {
					rn.mu.Unlock()
					return false, nil, errors.New("server with given serverID already exists")
				}
			}
			rn.log = append(rn.log, LogEntry{Command: command, Term: rn.currentTerm}) // Append the command to the log

			// Updating the configuration for this node. Raft Paper Section 6 mentions that
			// "Once a server adds the new configuration to its log, it uses that configuration
			// for all future decisions (regardless of whether it has been committed) "
			for i := 0; i < len(serverIds); i++ {
				rn.peerList.Add(uint64(serverIds[i]))
				rn.server.peerList.Add(uint64(serverIds[i]))
				rn.nextIndex[uint64(serverIds[i])] = uint64(len(rn.log)) + 1 // Initialize nextIndex for all peers with the last log index (leader) + 1
				rn.matchIndex[uint64(serverIds[i])] = 0                      // No match index yet
			}
			// Add code to establish connections
			rn.persistToStorage()      // Persist the log to storage
			rn.debug("log=%v", rn.log) // Debug the log state
			rn.mu.Unlock()             // Unlock the mutex before returning
			rn.trigger <- struct{}{}   // Trigger the event for append entries
			return true, nil, nil      // Return true since we are the leader
		case RemoveServers:
			serverIds := v.ServerIds
			for i := 0; i < len(serverIds); i++ {
				if !rn.peerList.Exists(uint64(serverIds[i])) && rn.id != uint64(serverIds[i]) {
					rn.mu.Unlock()
					return false, nil, errors.New("server with given serverID does not exist")
				}
			}
			rn.log = append(rn.log, LogEntry{Command: command, Term: rn.currentTerm}) // Append the command to the log

			// Updating the configuration for this node. Raft Paper Section 6 mentions that
			// "Once a server adds the new configuration to its log, it uses that configuration
			// for all future decisions (regardless of whether it has been committed) "
			for i := 0; i < len(serverIds); i++ {
				if rn.id != uint64(serverIds[i]) {
					rn.peerList.Remove(uint64(serverIds[i]))
					rn.server.peerList.Remove(uint64(serverIds[i]))
				}
			}
			// Add code to remove connections
			rn.persistToStorage()      // Persist the log to storage
			rn.debug("log=%v", rn.log) // Debug the log state
			rn.mu.Unlock()             // Unlock the mutex before returning
			rn.trigger <- struct{}{}   // Trigger the event for append entries
			return true, nil, nil      // Return true since we are the leader
		default:
			rn.log = append(rn.log, LogEntry{Command: command, Term: rn.currentTerm}) // Append the command to the log
			rn.persistToStorage()                                                     // Persist the log to storage
			rn.debug("log=%v", rn.log)                                                // Debug the log state
			rn.mu.Unlock()                                                            // Unlock the mutex before returning
			rn.trigger <- struct{}{}                                                  // Trigger the event for append entries
			return true, nil, nil                                                     // Return true since we are the leader
		}
	}

	rn.mu.Unlock() // Unlock the mutex
	return false, nil, nil
}

// Stop stops this RaftNode, cleaning up  its  state.  This  method
// returns quickly, but it may take a bit of time (up  to  election
// timeout) for all goroutines to exit and fully free its resources

func (rn *RaftNode) Stop() {
	rn.mu.Lock()         // Lock the mutex
	defer rn.mu.Unlock() // Unlock the mutex

	// Update the state to stopped
	rn.state = Dead          // Set the state to dead
	rn.debug("Becomes Dead") // Debug the state
	close(rn.newCommitReady) // Close the channel
}

// Report reports the current state of the RaftNode
// This function primarily  returns  the  following
// information:
// - The  identity  of  the  RaftNode
// - The current term of the RaftNode
// - The  boolean  indicating whether
//   this   RaftNode   is   a  leader

func (rn *RaftNode) Report() (id int, term int, isLeader bool) {
	rn.mu.Lock()         // Lock the mutex
	defer rn.mu.Unlock() // Unlock the mutex

	isLeader = rn.state == Leader                    // Set the leader flag to true if the node is a leader
	return int(rn.id), int(rn.currentTerm), isLeader // Return the id, term and leader flag
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
