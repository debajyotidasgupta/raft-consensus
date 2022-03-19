package raft

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
  
	seed := time.Now().UnixNano() // 1647347572251367891
	fmt.Println("Seed: ", seed)
  
	rand.Seed(seed)
}

type ClusterSimulator struct {
	mu sync.Mutex

	raftCluster map[uint64]*Server // all servers present in Cluster
	dbCluster   map[uint64]*Database

	commitChans map[uint64]chan CommitEntry // commit Channel for each Cluster

	commits map[uint64][]CommitEntry // commits[i] := sequence of commits by server i

	isConnected map[uint64]bool // check if node i is connected to cluster

	isAlive map[uint64]bool // check if node is alive
	/*
	*Now that servers can leave/enter, the cluster must have
	*info of all servers in the cluster, which is stored in
	*activeServers
	 */
	activeServers Set    // set of all servers currently in the cluster
	n             uint64 // number of servers
	t             *testing.T
}

type CommitFunctionType int

const (
	TestCommitFunction CommitFunctionType = iota
	TestNoCommitFunction
)

type Write struct {
	Key string
	Val int
}

type Read struct {
	Key string
}

type AddServers struct {
	ServerIds []int
}

type RemoveServers struct {
	ServerIds []int
}

// Create a new ClusterSimulator
func CreateNewCluster(t *testing.T, n uint64) *ClusterSimulator {
	// initialising required fields of ClusterSimulator

	serverList := make(map[uint64]*Server)
	isConnected := make(map[uint64]bool)
	isAlive := make(map[uint64]bool)
	commitChans := make(map[uint64]chan CommitEntry)
	commits := make(map[uint64][]CommitEntry)
	ready := make(chan interface{})
	storage := make(map[uint64]*Database)
	activeServers := makeSet()
	// creating servers

	for i := uint64(0); i < n; i++ {
		peerList := makeSet()

		// get PeerList for server i
		for j := uint64(0); j < n; j++ {
			if i == j {
				continue
			} else {
				peerList.Add(j)
			}
		}

		storage[i] = NewDatabase()
		commitChans[i] = make(chan CommitEntry)
		serverList[i] = CreateServer(i, peerList, storage[i], ready, commitChans[i])

		serverList[i].Serve()
		isAlive[i] = true
	}

	// Connecting peers to each other
	for i := uint64(0); i < n; i++ {
		for j := uint64(0); j < n; j++ {
			if i == j {
				continue
			}
			serverList[i].ConnectToPeer(j, serverList[j].GetListenerAddr())
		}
		isConnected[i] = true
	}

	for i := uint64(0); i < n; i++ {
		activeServers.Add(i)
	}
	close(ready)

	// create a new cluster
	newCluster := &ClusterSimulator{
		raftCluster:   serverList,
		dbCluster:     storage,
		commitChans:   commitChans,
		commits:       commits,
		isConnected:   isConnected,
		isAlive:       isAlive,
		n:             n,
		activeServers: activeServers,
		t:             t,
	}

	for i := uint64(0); i < n; i++ {
		go newCluster.collectCommits(i)
	}

	return newCluster
}

// Shut down all servers in the cluster
func (nc *ClusterSimulator) Shutdown() {

	for i := range nc.activeServers.peerSet {
		nc.raftCluster[i].DisconnectAll()
		nc.isConnected[i] = false
	}

	for i := range nc.activeServers.peerSet {
		if nc.isAlive[i] {
			nc.isAlive[i] = false
			nc.raftCluster[i].Stop()
		}
	}

	for i := range nc.activeServers.peerSet {
		close(nc.commitChans[i])
	}
}

// Reads channel and adds all received entries to the corresponding commits
func (nc *ClusterSimulator) collectCommits(i uint64) error {
	for commit := range nc.commitChans[i] {
		nc.mu.Lock()
		logtest(i, "collectCommits (%d) got %+v", i, commit)
		switch v := commit.Command.(type) {
		case Read:
			break
		case Write:
			var buf bytes.Buffer        // Buffer to hold the data
			enc := gob.NewEncoder(&buf) // Create a new encoder

			if err := enc.Encode(v.Val); err != nil { // Encode the data
				nc.mu.Unlock()
				return err
			}
			nc.dbCluster[i].Set(v.Key, buf.Bytes()) // Save the data to the database
		case RemoveServers:
			serverIds := v.ServerIds
			for i := uint64(0); i < uint64(len(serverIds)); i++ {
				if nc.activeServers.Exists(uint64(serverIds[i])) {
					// Cluster Modifications
					nc.DisconnectPeer(uint64(serverIds[i]))
					nc.isAlive[uint64(serverIds[i])] = false
					nc.raftCluster[uint64(serverIds[i])].Stop()
					nc.commits[uint64(serverIds[i])] = nc.commits[uint64(serverIds[i])][:0]
					close(nc.commitChans[uint64(serverIds[i])])

					// Removing traces of this server
					delete(nc.raftCluster, uint64(serverIds[i]))
					delete(nc.dbCluster, uint64(serverIds[i]))
					delete(nc.commitChans, uint64(serverIds[i]))
					delete(nc.commits, uint64(serverIds[i]))
					delete(nc.isAlive, uint64(serverIds[i]))
					delete(nc.isConnected, uint64(serverIds[i]))

					nc.activeServers.Remove(uint64(serverIds[i]))
				}
			}
		default:
			break
		}
		nc.commits[i] = append(nc.commits[i], commit)
		nc.mu.Unlock()
	}
	return nil
}

// Disconnect a server from other servers
func (nc *ClusterSimulator) DisconnectPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Disconnect %d", id)

	nc.raftCluster[id].DisconnectAll()
	for i := range nc.activeServers.peerSet {
		if i == id {
			continue
		} else {
			nc.raftCluster[i].DisconnectPeer(id)
		}
	}
	nc.isConnected[id] = false
	return nil
}

// Reconnect a server to other servers
func (nc *ClusterSimulator) ReconnectPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Reconnect %d", id)

	for i := range nc.activeServers.peerSet {
		if i != id && nc.isAlive[i] {
			err := nc.raftCluster[id].ConnectToPeer(i, nc.raftCluster[i].GetListenerAddr())
			if err != nil {
				if nc.t != nil {
					nc.t.Fatal(err)
				} else {
					return err
				}
			}
			err = nc.raftCluster[i].ConnectToPeer(id, nc.raftCluster[id].GetListenerAddr())
			if err != nil {
				if nc.t != nil {
					nc.t.Fatal(err)
				} else {
					return err
				}
			}
		}
	}

	nc.isConnected[id] = true
	return nil
}

// Crash a server and shut it down
func (nc *ClusterSimulator) CrashPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	logtest(id, "Crash %d", id)

	nc.DisconnectPeer(id)

	nc.isAlive[id] = false
	nc.raftCluster[id].Stop()

	nc.mu.Lock()
	nc.commits[id] = nc.commits[id][:0]
	nc.mu.Unlock()
	return nil
}

// Restart a server and reconnect to other peers
func (nc *ClusterSimulator) RestartPeer(id uint64) error {
	if !nc.activeServers.Exists(uint64(id)) {
		return fmt.Errorf("invalid server id passed")
	}
	if nc.isAlive[id] {
		if nc.t != nil {
			log.Fatalf("Id %d alive in restart peer", id)
		} else {
			return fmt.Errorf("id %d alive in restart peer", id)
		}
	}
	logtest(id, "Restart ", id, id)

	peerList := makeSet()
	for i := range nc.activeServers.peerSet {
		if id == i {
			continue
		} else {
			peerList.Add(i)
		}
	}

	ready := make(chan interface{})

	nc.raftCluster[id] = CreateServer(id, peerList, nc.dbCluster[id], ready, nc.commitChans[id])
	nc.raftCluster[id].Serve()
	nc.ReconnectPeer(id)

	close(ready)
	nc.isAlive[id] = true
	time.Sleep(time.Duration(20) * time.Millisecond)
	return nil
}

// Ensure only a single leader
func (nc *ClusterSimulator) CheckUniqueLeader() (int, int, error) {
	for r := 0; r < 8; r++ {
		leaderId := -1
		leaderTerm := -1

		for i := range nc.activeServers.peerSet {
			if nc.isConnected[i] {
				_, term, isLeader := nc.raftCluster[i].rn.Report()
				if isLeader {
					if leaderId < 0 {
						leaderId = int(i)
						leaderTerm = term
					} else {
						if nc.t != nil {
							nc.t.Fatalf("2 ids: %d, %d think they are leaders", leaderId, i)
						} else {
							return -1, -1, fmt.Errorf("2 ids: %d, %d think they are leaders", leaderId, i)
						}
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm, nil
		}
		time.Sleep(150 * time.Millisecond)
	}

	if nc.t != nil {
		nc.t.Fatalf("no leader found")
	}
	return -1, -1, fmt.Errorf("no leader found")
}

// check if there are no leaders
func (nc *ClusterSimulator) CheckNoLeader() error {

	for i := range nc.activeServers.peerSet {
		if nc.isConnected[i] {
			if _, _, isLeader := nc.raftCluster[i].rn.Report(); isLeader {
				if nc.t != nil {
					nc.t.Fatalf("%d is Leader, expected no leader", i)
				} else {
					return fmt.Errorf("%d is Leader, expected no leader", i)
				}
			}
		}
	}
	return nil
}

func (nc *ClusterSimulator) CheckCommitted(cmd int, choice CommitFunctionType) (num int, index int, err error) {
	nc.mu.Lock()
	defer nc.mu.Unlock()

	err = nil

	// Find the length of the commits slice for connected servers.
	commitsLen := -1
	for i := range nc.activeServers.peerSet {
		if nc.isConnected[i] {
			if commitsLen >= 0 {
				// If this was set already, expect the new length to be the same.
				if len(nc.commits[i]) != commitsLen {
					if nc.t != nil {
						nc.t.Fatalf("commits[%d] = %d, commitsLen = %d", i, nc.commits[i], commitsLen)
					} else {
						err = fmt.Errorf("commits[%d] = %d, commitsLen = %d", i, nc.commits[i], commitsLen)
						return -1, -1, err
					}
				}
			} else {
				commitsLen = len(nc.commits[i])
			}
		}
	}

	// Check consistency of commits from the start and to the command we're asked
	// about. This loop will return once a command=cmd is found.
	for c := 0; c < commitsLen; c++ {
		cmdAtC := -1
		for i := uint64(0); i < nc.n; i++ {
			if nc.isConnected[i] {
				cmdOfN := nc.commits[i][c].Command.(int)
				if cmdAtC >= 0 {
					if cmdOfN != cmdAtC {
						if nc.t != nil {
							nc.t.Errorf("got %d, want %d at nc.commits[%d][%d]", cmdOfN, cmdAtC, i, c)
						} else {
							err = fmt.Errorf("got %d, want %d at nc.commits[%d][%d]", cmdOfN, cmdAtC, i, c)
						}
					}
				} else {
					cmdAtC = cmdOfN
				}
			}
		}
		if cmdAtC == cmd {
			// Check consistency of Index.
			index := -1
			num := 0
			for i := uint64(0); i < nc.n; i++ {
				if nc.isConnected[i] {
					if index >= 0 && int(nc.commits[i][c].Index) != index {
						if nc.t != nil {
							nc.t.Errorf("got Index=%d, want %d at h.commits[%d][%d]", nc.commits[i][c].Index, index, i, c)
						} else {
							err = fmt.Errorf("got Index=%d, want %d at h.commits[%d][%d]", nc.commits[i][c].Index, index, i, c)
						}
					} else {
						index = int(nc.commits[i][c].Index)
					}
					num++
				}
			}
			return num, index, err
		}
	}

	// If there's no early return, we haven't found the command we were looking for

	if choice == TestCommitFunction {
		if nc.t != nil {
			nc.t.Errorf("cmd = %d not found in commits", cmd)
		} else {
			err = fmt.Errorf("cmd = %d not found in commits", cmd)
		}
		return 0, -1, err
	} else {
		return 0, -1, err
	}

}

func (nc *ClusterSimulator) SubmitToServer(serverId int, cmd interface{}) (bool, interface{}, error) {
	if !nc.activeServers.Exists(uint64(serverId)) {
		return false, nil, fmt.Errorf("invalid server id passed")
	}
	switch v := cmd.(type) {
	case AddServers:
		nc.mu.Lock()
		// Cluster modifications
		serverIds := v.ServerIds
		for i := 0; i < len(serverIds); i++ {
			nc.activeServers.Add(uint64(serverIds[i]))
		}
		ready := make(chan interface{})

		// creating the new servers to be added
		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			peerList := makeSet()

			// get PeerList for server i
			for j := range nc.activeServers.peerSet {
				if uint64(serverIds[i]) == j {
					continue
				} else {
					peerList.Add(j)
				}
			}

			nc.dbCluster[uint64(serverIds[i])] = NewDatabase()
			nc.commitChans[uint64(serverIds[i])] = make(chan CommitEntry)
			nc.raftCluster[uint64(serverIds[i])] = CreateServer(uint64(serverIds[i]), peerList, nc.dbCluster[uint64(serverIds[i])], ready, nc.commitChans[uint64(serverIds[i])])

			nc.raftCluster[uint64(serverIds[i])].Serve()
			nc.isAlive[uint64(serverIds[i])] = true
		}

		// Connecting peers to each other
		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			for j := range nc.activeServers.peerSet {
				if uint64(serverIds[i]) == j {
					continue
				}
				nc.raftCluster[uint64(serverIds[i])].ConnectToPeer(j, nc.raftCluster[j].GetListenerAddr())
				nc.raftCluster[j].ConnectToPeer(uint64(serverIds[i]), nc.raftCluster[uint64(serverIds[i])].GetListenerAddr())
			}
			nc.isConnected[uint64(serverIds[i])] = true
		}

		for i := uint64(0); i < uint64(len(serverIds)); i++ {
			go nc.collectCommits(uint64(serverIds[i]))
		}

		close(ready)

		nc.mu.Unlock()
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	case RemoveServers:
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	default:
		return nc.raftCluster[uint64(serverId)].rn.Submit(cmd)
	}
}

func logtest(id uint64, logstr string, a ...interface{}) {
	if DEBUG > 0 {
		logstr = "[" + strconv.Itoa(int(id)) + "] " + "[TEST]" + logstr
		log.Printf(logstr, a...)
	}
}
