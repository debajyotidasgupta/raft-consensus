package raft

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func init() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	seed := time.Now().UnixNano()
	//var seed = int64(1644671576249871733)
	fmt.Println("Seed: ", seed)
	rand.Seed(seed)
}

type ClusterSimulator struct {
	mu sync.Mutex

	raftCluster []*Server // all servers present in Cluster
	dbCluster   []*Database

	commitChans []chan CommitEntry // commit Channel for each Cluster

	commits [][]CommitEntry // commits[i] := sequence of commits by server i

	isConnected []bool // check if node i is connected to cluster

	isAlive []bool // check if node is alive

	n uint64 // number of servers
	t *testing.T
}

// Create a new ClusterSimulator
func CreateNewCluster(t *testing.T, n uint64) *ClusterSimulator {
	// initialising required fields of ClusterSimulator

	serverList := make([]*Server, n)
	isConnected := make([]bool, n)
	isAlive := make([]bool, n)
	commitChans := make([]chan CommitEntry, n)
	commits := make([][]CommitEntry, n)
	ready := make(chan interface{})
	storage := make([]*Database, n)

	// creating servers

	for i := uint64(0); i < n; i++ {
		peerIds := make([]uint64, 0)

		// get PeerIDs for server i
		for j := uint64(0); j < n; j++ {
			if i == j {
				continue
			} else {
				peerIds = append(peerIds, j)
			}
		}

		storage[i] = NewDatabase()
		commitChans[i] = make(chan CommitEntry)
		serverList[i] = CreateServer(i, peerIds, storage[i], ready, commitChans[i])

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

	close(ready)

	// create a new cluster
	newCluster := &ClusterSimulator{
		raftCluster: serverList,
		dbCluster:   storage,
		commitChans: commitChans,
		commits:     commits,
		isConnected: isConnected,
		isAlive:     isAlive,
		n:           n,
		t:           t,
	}

	for i := uint64(0); i < n; i++ {
		go newCluster.collectCommits(i)
	}

	return newCluster
}

// Shut down all servers in the cluster
func (nc *ClusterSimulator) Shutdown() {
	for i := uint64(0); i < nc.n; i++ {
		nc.raftCluster[i].DisconnectAll()
		nc.isConnected[i] = false
	}

	for i := uint64(0); i < nc.n; i++ {
		if nc.isAlive[i] {
			nc.isAlive[i] = false
			nc.raftCluster[i].Stop()
		}
	}

	for i := uint64(0); i < nc.n; i++ {
		close(nc.commitChans[i])
	}
}

// Reads channel and adds all received entries to the corresponding commits
func (nc *ClusterSimulator) collectCommits(i uint64) {
	for commit := range nc.commitChans[i] {
		nc.mu.Lock()
		logtest("collectCommits (%d) got %+v", i, commit)
		nc.commits[i] = append(nc.commits[i], commit)
		nc.mu.Unlock()
	}
}

// Disconnect a server from other servers
func (nc *ClusterSimulator) DisconnectPeer(id uint64) {
	logtest("Disconnect %d", id)

	nc.raftCluster[id].DisconnectAll()
	for i := uint64(0); i < nc.n; i++ {
		if i == id {
			continue
		} else {
			nc.raftCluster[i].DisconnectPeer(id)
		}
	}
	nc.isConnected[id] = false
}

// Reconnect a server to other servers
func (nc *ClusterSimulator) ReconnectPeer(id uint64) {
	logtest("Reconnect %d", id)

	for i := uint64(0); i < nc.n; i++ {
		if i != id && nc.isAlive[i] {
			err := nc.raftCluster[id].ConnectToPeer(i, nc.raftCluster[i].GetListenerAddr())
			if err != nil {
				nc.t.Fatal(err)
			}
			err = nc.raftCluster[i].ConnectToPeer(id, nc.raftCluster[id].GetListenerAddr())
			if err != nil {
				nc.t.Fatal(err)
			}
		}
	}

	nc.isConnected[id] = true
}

// Crash a server and shut it down
func (nc *ClusterSimulator) CrashPeer(id uint64) {
	logtest("Crash %d", id)

	nc.DisconnectPeer(id)
	nc.isAlive[id] = false
	nc.raftCluster[id].Stop()

	nc.mu.Lock()
	nc.commits[id] = nc.commits[id][:0]
	nc.mu.Unlock()
}

// Restart a server and reconnect to other peers
func (nc *ClusterSimulator) RestartPeer(id uint64) {
	if nc.isAlive[id] {
		log.Fatalf("Id %d alive in restart peer", id)
	}
	logtest("Restart ", id)

	peerIds := make([]uint64, 0)

	for i := uint64(0); i < nc.n; i++ {
		if id == i {
			continue
		} else {
			peerIds = append(peerIds, i)
		}
	}

	ready := make(chan interface{})

	nc.raftCluster[id] = CreateServer(id, peerIds, nc.dbCluster[id], ready, nc.commitChans[id])
	nc.raftCluster[id].Serve()
	nc.ReconnectPeer(id)

	close(ready)
	nc.isAlive[id] = true
	time.Sleep(time.Duration(20) * time.Millisecond)
}

// Ensure only a single leader
func (nc *ClusterSimulator) CheckUniqueLeader() (int, int) {
	for r := 0; r < 8; r++ {
		leaderId := -1
		leaderTerm := -1

		for i := uint64(0); i < nc.n; i++ {
			if nc.isConnected[i] {
				_, term, isLeader := nc.raftCluster[i].rn.Report()
				if isLeader {
					if leaderId < 0 {
						leaderId = int(i)
						leaderTerm = term
					} else {
						nc.t.Fatalf("2 ids: %d, %d think they are leaders", leaderId, i)
					}
				}
			}
		}
		if leaderId >= 0 {
			return leaderId, leaderTerm
		}
		time.Sleep(150 * time.Millisecond)
	}

	nc.t.Fatalf("no leader found")
	return -1, -1
}

// check if there are no leaders
func (nc *ClusterSimulator) CheckNoLeader() {

	for i := uint64(0); i < nc.n; i++ {
		if nc.isConnected[i] {
			if _, _, isLeader := nc.raftCluster[i].rn.Report(); isLeader {
				nc.t.Fatalf("%d is Leader, expected no leader", i)
			}
		}
	}
}

func logtest(logstr string, a ...interface{}) {
	logstr = "[TEST]" + logstr
	log.Printf(logstr, a...)
}
