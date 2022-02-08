package raft

import (
	"log"
	"math/rand"
	"strconv"
	"sync"
	"testing"
	"time"
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

var ports map[int]string = map[int]string{}

func communicate(serverId int, numPeers int, s *Server) {
	defer wg.Done()
	for {
		peerId := rand.Intn(numPeers) + 1
		pause := rand.Intn(50)
		pauseTime := pause * int(time.Millisecond)
		time.Sleep(time.Duration(pauseTime))
		msg := rand.Intn(1000)
		if peerId == serverId {
			log.Println(serverId, "Shutting")
			s.DisconnectAll()
			s.Stop()
			log.Println(serverId, "Stopped")
			return
		} else {
			addr := Addr{"tcp", "[::]:" + ports[peerId]}
			err := s.ConnectToPeer(peerId, addr)
			var reply int
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
	var numPeers int = 5
	var port = 20000

	for i := 1; i <= numPeers; i++ {
		portStr := strconv.Itoa(port)
		ports[i] = portStr
		port++
	}

	var servers []*Server

	for i := 1; i <= numPeers; i++ {
		peerIds := make([]int, numPeers-1)
		j := 0
		for peerId := 1; peerId <= numPeers; peerId++ {
			if peerId != i {
				peerIds[j] = peerId
				j++
			}
		}
		s := CreateServer(i, peerIds)
		if s == nil {
			t.Errorf("ERROR: server could not be created")
		}
		servers = append(servers, s)
		s.Serve(ports[i])
	}

	for i := 1; i <= numPeers; i++ {
		wg.Add(1)
		go communicate(i, numPeers, servers[i-1])
	}
	wg.Wait()
}
