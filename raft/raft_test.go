package raft

import (
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
)

type Addr struct {
	network string
	address string
}

func (a Addr) Network() string {
	return a.network
}

func (a Addr) String() string {
	return a.address
}

var wg = sync.WaitGroup{}

var ports map[uint64]string = map[uint64]string{}

func communicate(serverId uint64, numPeers uint64, s *Server) {
	defer wg.Done()
	for {
		peerId := uint64(rand.Intn(int(numPeers)) + 1)
		pause := uint64(rand.Intn(50))
		pauseTime := pause * uint64(time.Millisecond)

		time.Sleep(time.Duration(pauseTime))
		msg := uint64(rand.Intn(1000))
		if peerId == serverId {
			log.Println(serverId, "Shutting")
			s.DisconnectAll()
			s.Stop()
			log.Println(serverId, "Stopped")
			return
		} else {
			addr := Addr{"tcp", "[::]:" + ports[peerId]}
			err := s.ConnectToPeer(peerId, addr)
			var reply uint64
			if err == nil {
				log.Printf("[%d] sending %d to [%d]\n", serverId, msg, peerId)
				s.RPC(peerId, "ServiceType.DisplayMsg", msg, &reply)
				if reply != 2*msg {
					s.DisconnectAll()
					s.Stop()
					log.Fatalf("[%d] returned %d expected %d\n", peerId, reply, 2*msg)
				}
			}
		}
	}
}

// Change the numPeers to test with different number of peers
func TestServerClient(t *testing.T) {
	var numPeers uint64 = 5
	var port = 20000

	for i := uint64(1); i <= numPeers; i++ {
		portStr := strconv.Itoa(port)
		ports[i] = portStr
		port++
	}

	var servers []*Server

	for i := uint64(1); i <= numPeers; i++ {
		peerIds := make([]uint64, numPeers-1)
		j := 0
		for peerId := uint64(1); peerId <= numPeers; peerId++ {
			if peerId != i {
				peerIds[j] = peerId
				j++
			}
		}

		db := NewDatabase()
		ready := make(chan interface{})
		commitChan := make(chan CommitEntry)

		s := CreateServer(i, peerIds, db, ready, commitChan)
		if s == nil {
			t.Errorf("ERROR: server could not be created")
		}
		servers = append(servers, s)
		s.Serve(ports[i])
	}

	for i := uint64(1); i <= numPeers; i++ {
		wg.Add(1)
		go communicate(i, numPeers, servers[i-1])
	}

	wg.Wait()
}

//REAL RAFT TESTS START HERE

func TestElectionNormal(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()
	cs.CheckUniqueLeader()
}

func TestElectionLeaderDisconnect(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm := cs.CheckUniqueLeader()

	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm := cs.CheckUniqueLeader()

	if newLeader == initialLeader {
		t.Errorf("new leader expected to be different from initial leader")
	}
	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
	}
}

func TestElectionLeaderAndFollowerDisconnect(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, _ := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(initialLeader))
	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	time.Sleep(300 * time.Millisecond)
	cs.CheckNoLeader()

	cs.ReconnectPeer(uint64(initialLeader))
	time.Sleep(100 * time.Millisecond)
	cs.CheckUniqueLeader()

}

func TestElectionLeaderDisconnectAndReconnect(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm := cs.CheckUniqueLeader()

	if newLeader == initialLeader {
		t.Errorf("new leader expected to be different from initial leader")
		return
	}
	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
		return
	}

	cs.ReconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	latestLeader, latestLeaderTerm := cs.CheckUniqueLeader()

	if latestLeader != newLeader {
		t.Errorf("latest leader expected to be %d, got %d", newLeader, latestLeader)
	}
	if latestLeaderTerm != newLeaderTerm {
		t.Errorf("latest leader term expected to be %d got %d", newLeaderTerm, latestLeaderTerm)
	}
}

func TestElectionDisconnectAllAndReconnectAll(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	time.Sleep(300 * time.Millisecond)

	for i := 0; i < 3; i++ {
		cs.DisconnectPeer(uint64(i))
	}
	time.Sleep(300 * time.Millisecond)
	cs.CheckNoLeader()

	for i := 0; i < 3; i++ {
		cs.ReconnectPeer(uint64(i))
	}

	time.Sleep(300 * time.Millisecond)
	cs.CheckUniqueLeader()
}

func TestElectionLeaderDisconnectAndReconnect5Nodes(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm := cs.CheckUniqueLeader()

	if newLeader == initialLeader {
		t.Errorf("new leader expected to be different from initial leader")
		return
	}
	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
		return
	}

	cs.ReconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	latestLeader, latestLeaderTerm := cs.CheckUniqueLeader()

	if latestLeader != newLeader {
		t.Errorf("latest leader expected to be %d, got %d", newLeader, latestLeader)
	}
	if latestLeaderTerm != newLeaderTerm {
		t.Errorf("latest leader term expected to be %d got %d", newLeaderTerm, latestLeaderTerm)
	}
}

func TestElectionFollowerDisconnectAndReconnect(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm := cs.CheckUniqueLeader()
	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	time.Sleep(300 * time.Millisecond)

	cs.ReconnectPeer(uint64(follower))
	time.Sleep(100 * time.Millisecond)
	_, newLeaderTerm := cs.CheckUniqueLeader()

	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
	}
}

func TestElectionDisconnectReconnectLoop(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	var term = 0

	for i := 0; i < 6; i++ {
		leader, newTerm := cs.CheckUniqueLeader()

		if newTerm <= term {
			t.Errorf("new leader term expected to be > old leader term, new term=%d old term=%d", newTerm, term)
			return
		}

		cs.DisconnectPeer(uint64(leader))
		follower := (leader + 1) % 3
		cs.DisconnectPeer(uint64(follower))
		time.Sleep(300 * time.Millisecond)
		cs.CheckNoLeader()

		cs.ReconnectPeer(uint64(follower))
		cs.ReconnectPeer(uint64(leader))

		time.Sleep(100 * time.Millisecond)
	}

	_, newTerm := cs.CheckUniqueLeader()

	if newTerm <= term {
		t.Errorf("new leader term expected to be > old leader term, new term=%d old term=%d", newTerm, term)
		return
	}
}

func TestElectionFollowerDisconnectReconnectAfterLong(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm := cs.CheckUniqueLeader()

	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	time.Sleep(1200 * time.Millisecond)

	cs.ReconnectPeer(uint64(follower))

	time.Sleep(500 * time.Millisecond)
	newLeader, newLeaderTerm := cs.CheckUniqueLeader()

	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
	}

	if newLeader != follower {
		t.Errorf("new leader expected to be %d, got %d", follower, newLeader)
	}
}
