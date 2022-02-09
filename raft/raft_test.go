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
		s := CreateServer(i, peerIds)
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
