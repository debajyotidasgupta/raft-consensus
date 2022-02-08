package raft

import (
	"fmt"
	"log"
	"math/rand"
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
		peerId := rand.Intn(numPeers)
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

func TestServerClient(t *testing.T) {
	var numPeers int = 3
	//fmt.Scanf("%d", &numPeers)
	var port = 20000

	for i := 1; i <= numPeers; i++ {
		portStr := fmt.Sprintf("%d", port)
		ports[i] = portStr
		port++
	}

	var servers []*Server

	for i := 1; i <= numPeers; i++ {
		peerIds := []int{}
		for peerId := 1; peerId <= numPeers; i++ {
			if peerId != i {
				peerIds = append(peerIds, peerId)
			}
		}
		s := CreateServer(i, peerIds)
		servers = append(servers, s)
		s.Serve(ports[i])
	}

	for i := 1; i <= numPeers; i++ {
		wg.Add(1)
		go communicate(i, numPeers, servers[i-1])
	}
	wg.Wait()
}
