package raft

import (
	"encoding/gob"
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
	var port = 2000

	for i := uint64(1); i <= numPeers; i++ {
		portStr := strconv.Itoa(port)
		ports[i] = portStr
		port++
	}

	var servers []*Server

	for i := uint64(1); i <= numPeers; i++ {
		peerList := makeSet()
		j := 0
		for peerId := uint64(1); peerId <= numPeers; peerId++ {
			if peerId != i {
				peerList.Add(uint64(j))
				j++
			}
		}

		db := NewDatabase()
		ready := make(chan interface{})
		commitChan := make(chan CommitEntry)

		s := CreateServer(i, peerList, db, ready, commitChan)
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

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()

	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm, _ := cs.CheckUniqueLeader()

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

	initialLeader, _, _ := cs.CheckUniqueLeader()
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

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm, _ := cs.CheckUniqueLeader()

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

	latestLeader, latestLeaderTerm, _ := cs.CheckUniqueLeader()

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

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(initialLeader))
	time.Sleep(300 * time.Millisecond)

	newLeader, newLeaderTerm, _ := cs.CheckUniqueLeader()

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

	latestLeader, latestLeaderTerm, _ := cs.CheckUniqueLeader()

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

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()
	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	time.Sleep(300 * time.Millisecond)

	cs.ReconnectPeer(uint64(follower))
	time.Sleep(100 * time.Millisecond)
	_, newLeaderTerm, _ := cs.CheckUniqueLeader()

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
		leader, newTerm, _ := cs.CheckUniqueLeader()

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

	_, newTerm, _ := cs.CheckUniqueLeader()

	if newTerm <= term {
		t.Errorf("new leader term expected to be > old leader term, new term=%d old term=%d", newTerm, term)
		return
	}
}

func TestElectionFollowerDisconnectReconnectAfterLong(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()

	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	time.Sleep(1200 * time.Millisecond)

	cs.ReconnectPeer(uint64(follower))

	time.Sleep(500 * time.Millisecond)
	newLeader, newLeaderTerm, _ := cs.CheckUniqueLeader()

	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
	}

	if newLeader != follower {
		t.Errorf("new leader expected to be %d, got %d", follower, newLeader)
	}
}

func TestCommitOneCommand(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()

	logtest(uint64(origLeaderId), "submitting 42 to %d", origLeaderId)
	isLeader, _, _ := cs.SubmitToServer(origLeaderId, 42)
	if !isLeader {
		t.Errorf("want id=%d leader, but it's not", origLeaderId)
	}

	time.Sleep(time.Duration(250) * time.Millisecond)
	num, _, _ := cs.CheckCommitted(42, 0)
	if num != 3 {
		t.Errorf("Not committed by 3 nodes")
	}
}

func TestElectionFollowerDisconnectReconnectAfterLongCommitDone(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	initialLeader, initialLeaderTerm, _ := cs.CheckUniqueLeader()

	follower := (initialLeader + 1) % 3
	cs.DisconnectPeer(uint64(follower))

	logtest(uint64(initialLeader), "submitting 42 to %d", initialLeader)
	isLeader, _, _ := cs.SubmitToServer(initialLeader, 42)
	if !isLeader {
		t.Errorf("want id=%d leader, but it's not", initialLeader)
	}

	time.Sleep(1200 * time.Millisecond)

	cs.ReconnectPeer(uint64(follower))

	time.Sleep(500 * time.Millisecond)
	newLeader, newLeaderTerm, _ := cs.CheckUniqueLeader()

	if newLeaderTerm <= initialLeaderTerm {
		t.Errorf("new leader term expected to be > initial leader term, new term=%d old term=%d", newLeaderTerm, initialLeaderTerm)
	}

	if newLeader == follower {
		t.Errorf("new leader not expected to be %d", follower)
	}
}

func TestTryCommitToNonLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	leaderId, _, _ := cs.CheckUniqueLeader()
	servingId := (leaderId + 1) % 3
	logtest(uint64(servingId), "submitting 42 to %d", servingId)
	isLeader, _, _ := cs.SubmitToServer(servingId, 42)
	if isLeader {
		t.Errorf("want id=%d to be non leader, but it is", servingId)
	}
	time.Sleep(time.Duration(20) * time.Millisecond)
}

func TestCommitThenLeaderDisconnect(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()

	logtest(uint64(origLeaderId), "submitting 42 to %d", origLeaderId)
	isLeader, _, _ := cs.SubmitToServer(origLeaderId, 42)
	if !isLeader {
		t.Errorf("want id=%d leader, but it's not", origLeaderId)
	}

	time.Sleep(time.Duration(250) * time.Millisecond)

	cs.DisconnectPeer(uint64(origLeaderId))
	time.Sleep(time.Duration(300) * time.Millisecond)

	num, _, _ := cs.CheckCommitted(42, 0)
	if num != 2 {
		t.Errorf("expected 2 commits found = %d", num)
	}

}

func TestCommitMultipleCommands(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()

	values := []int{42, 55, 81}
	for _, v := range values {
		logtest(uint64(origLeaderId), "submitting %d to %d", v, origLeaderId)
		isLeader, _, _ := cs.SubmitToServer(origLeaderId, v)
		if !isLeader {
			t.Errorf("want id=%d leader, but it's not", origLeaderId)
		}
		time.Sleep(time.Duration(100) * time.Millisecond)
	}

	time.Sleep(time.Duration(300) * time.Millisecond)
	com1, i1, _ := cs.CheckCommitted(42, 0)
	com2, i2, _ := cs.CheckCommitted(55, 0)
	com3, i3, _ := cs.CheckCommitted(81, 0)

	if com1 != 3 || com2 != 3 || com3 != 3 {
		t.Errorf("expected com1 = com2 = com3 = 3 found com1 = %d com2 = %d com3 = %d", com1, com2, com3)
	}

	if i1 != 1 || i2 != 2 || i3 != 3 {
		t.Errorf("expected i1 = 1 i2 = 2 i3 = 3 found i1 = %d i2 = %d i3 = %d", i1, i2, i3)
	}
}

func TestCommitWithDisconnectionAndRecover(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	// Submit a couple of values to a fully connected cluster.
	origLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(origLeaderId, 5)
	cs.SubmitToServer(origLeaderId, 6)

	time.Sleep(time.Duration(500) * time.Millisecond)
	num, _, _ := cs.CheckCommitted(6, 0)
	if num != 3 {
		t.Errorf("expected 3 commits found = %d", num)
	}

	dPeerId := (origLeaderId + 1) % 3
	cs.DisconnectPeer(uint64(dPeerId))
	time.Sleep(time.Duration(500) * time.Millisecond)

	// Submit a new command; it will be committed but only to two servers.
	cs.SubmitToServer(origLeaderId, 7)
	time.Sleep(time.Duration(300) * time.Millisecond)
	num, _, _ = cs.CheckCommitted(7, 0)
	if num != 2 {
		t.Errorf("expected 2 commits found = %d", num)
	}
	// Now reconnect dPeerId and wait a bit; it should find the new command too.
	cs.ReconnectPeer(uint64(dPeerId))
	time.Sleep(time.Duration(500) * time.Millisecond)

	time.Sleep(time.Duration(500) * time.Millisecond)
	num, _, _ = cs.CheckCommitted(7, 0)
	if num != 3 {
		t.Errorf("expected 3 commits found = %d", num)
	}
}

func TestTryCommitMajorityFailure(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 3)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(origLeaderId, 5)

	time.Sleep(time.Duration(300) * time.Millisecond)
	num, _, _ := cs.CheckCommitted(5, 0)

	if num != 3 {
		t.Errorf("expected 3 commits found = %d", num)
	}

	dPeer1 := (origLeaderId + 1) % 3
	dPeer2 := (origLeaderId + 2) % 3

	cs.DisconnectPeer(uint64(dPeer1))
	cs.DisconnectPeer(uint64(dPeer2))
	time.Sleep(time.Duration(300) * time.Millisecond)

	cs.SubmitToServer(origLeaderId, 6)
	time.Sleep(time.Duration(300) * time.Millisecond)
	numC, _, _ := cs.CheckCommitted(6, 1)

	if numC != 0 {
		t.Errorf("expected 0 commits found = %d", numC)
	}

	cs.ReconnectPeer(uint64(dPeer1))
	cs.ReconnectPeer(uint64(dPeer2))
	time.Sleep(time.Duration(600) * time.Millisecond)

	newLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(newLeaderId, 8)
	time.Sleep(time.Duration(300) * time.Millisecond)

	numF, _, _ := cs.CheckCommitted(8, 1)
	if numF != 3 {
		t.Errorf("expected 3 commits found = %d", numF)
	}

}

func TestTryCommitToDCLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(origLeaderId, 5)
	time.Sleep(time.Duration(300) * time.Millisecond)

	cs.DisconnectPeer(uint64(origLeaderId))
	time.Sleep(time.Duration(200) * time.Millisecond)
	cs.SubmitToServer(origLeaderId, 6)
	time.Sleep(time.Duration(200) * time.Millisecond)

	num1, _, _ := cs.CheckCommitted(6, 1)
	if num1 != 0 {
		t.Errorf("expected 0 commits found = %d", num1)
	}

	newLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(newLeaderId, 7)
	time.Sleep(time.Duration(300) * time.Millisecond)
	num2, _, _ := cs.CheckCommitted(7, 0)

	if num2 != 4 {
		t.Errorf("expected 4 commits found = %d", num2)
	}

	cs.ReconnectPeer(uint64(origLeaderId))
	time.Sleep(time.Duration(300) * time.Millisecond)

	newLeaderId, _, _ = cs.CheckUniqueLeader()
	cs.SubmitToServer(newLeaderId, 8)
	time.Sleep(time.Duration(300) * time.Millisecond)
	num3, _, _ := cs.CheckCommitted(7, 0)
	num4, _, _ := cs.CheckCommitted(8, 0)
	num5, _, _ := cs.CheckCommitted(6, 1)
	if num3 != 5 || num4 != 5 || num5 != 0 {
		t.Errorf("expected num3 = num4 = 5 and num5 = 0 found num3= %d num4 = %d num5 = %d", num3, num4, num5)
	}
}

func TestTryCommitLeaderDisconnectsShortTime(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.DisconnectPeer(uint64(origLeaderId))
	cs.SubmitToServer(origLeaderId, 5)
	time.Sleep(time.Duration(10) * time.Millisecond)
	cs.ReconnectPeer(uint64(origLeaderId))
	time.Sleep(time.Duration(300) * time.Millisecond)

	num, _, _ := cs.CheckCommitted(5, 0)

	if num != 5 {
		t.Errorf("expected commits = 5 found = %d", num)
	}
}

func TestCrashFollowerThenLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(origLeaderId, 25)
	cs.SubmitToServer(origLeaderId, 26)
	cs.SubmitToServer(origLeaderId, 27)
	time.Sleep(time.Duration(250) * time.Millisecond)

	dPeerID1 := (origLeaderId + 1) % 5
	dPeerID2 := (origLeaderId + 2) % 5
	cs.CrashPeer(uint64(dPeerID1))
	cs.CrashPeer(uint64(dPeerID2))
	time.Sleep(time.Duration(250) * time.Millisecond)

	num1, _, _ := cs.CheckCommitted(25, 0)
	num2, _, _ := cs.CheckCommitted(26, 0)
	num3, _, _ := cs.CheckCommitted(27, 0)

	if num1 != 3 || num2 != 3 || num3 != 3 {
		t.Errorf("expected num1 = num2 = num3 = 3 got num1 = %d num2 = %d num3 = %d", num1, num2, num3)
	}

	cs.RestartPeer(uint64(dPeerID1))
	time.Sleep(time.Duration(250) * time.Millisecond)
	cs.CrashPeer(uint64(origLeaderId))
	time.Sleep(time.Duration(250) * time.Millisecond)

	newLeaderId, _, _ := cs.CheckUniqueLeader()
	cs.SubmitToServer(newLeaderId, 29)
	time.Sleep(time.Duration(250) * time.Millisecond)
	num4, _, _ := cs.CheckCommitted(29, 0)

	if num4 != 3 {
		t.Errorf("expected commit number = 3 found = %d", num4)
	}

}

func TestAddNewServer(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	gob.Register(Write{})
	gob.Register(Read{})
	gob.Register(AddServers{})
	gob.Register(RemoveServers{})

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	serverIds := []int{5, 6}
	commandToServer := AddServers{ServerIds: serverIds}

	if success, _, err := cs.SubmitToServer(origLeaderId, commandToServer); success {
		if err != nil {
			t.Errorf("Could not submit command")
		}
	} else {
		t.Errorf("Could not submit command")
	}

	time.Sleep(time.Duration(500) * time.Millisecond)
	numServer := cs.activeServers.Size()
	if numServer != 7 {
		t.Errorf("Add Servers could not be completed expected 7 servers, found %d", numServer)
	}
}

func TestRemoveServerNonLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	gob.Register(Write{})
	gob.Register(Read{})
	gob.Register(AddServers{})
	gob.Register(RemoveServers{})

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	rem1 := (origLeaderId + 1) % 5
	rem2 := (origLeaderId + 2) % 5
	serverIds := []int{rem1, rem2}
	commandToServer := RemoveServers{ServerIds: serverIds}

	if success, _, err := cs.SubmitToServer(origLeaderId, commandToServer); success {
		if err != nil {
			t.Errorf("Could not submit command")
		}
	} else {
		t.Errorf("Could not submit command")
	}

	time.Sleep(time.Duration(1000) * time.Millisecond)
	numServer := cs.activeServers.Size()
	if numServer != 3 {
		t.Errorf("Remove Servers could not be completed expected 3 servers, found %d", numServer)
	}
}

func TestRemoveLeader(t *testing.T) {
	defer leaktest.CheckTimeout(t, 100*time.Millisecond)()

	cs := CreateNewCluster(t, 5)
	defer cs.Shutdown()

	gob.Register(Write{})
	gob.Register(Read{})
	gob.Register(AddServers{})
	gob.Register(RemoveServers{})

	origLeaderId, _, _ := cs.CheckUniqueLeader()
	serverIds := []int{origLeaderId}
	commandToServer := RemoveServers{ServerIds: serverIds}

	if success, _, err := cs.SubmitToServer(origLeaderId, commandToServer); success {
		if err != nil {
			t.Errorf("Could not submit command")
		}
	} else {
		t.Errorf("Could not submit command")
	}

	time.Sleep(time.Duration(1000) * time.Millisecond)
	numServer := cs.activeServers.Size()
	if numServer != 4 {
		t.Errorf("Remove Servers could not be completed expected 3 servers, found %d", numServer)
	}

	newLeaderId, _, _ := cs.CheckUniqueLeader()
	if origLeaderId == newLeaderId {
		t.Errorf("Expected New Leader to be different, Found Same ")
	}
}
