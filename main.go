package main

import (
	"fmt"
	"raft-consensus/raft"
)

var ports = map[int]string{
	1: "20000",
	2: "21000",
	3: "22000",
}

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

func main() {
	var serverId int
	fmt.Scanf("%d", &serverId)

	peerIds := []int{}
	for i := 1; i <= 3; i++ {
		if i != serverId {
			peerIds = append(peerIds, i)
		}
	}

	s := raft.CreateServer(serverId, peerIds)

	fmt.Println("Serving")

	s.Serve(ports[serverId])

	for {
		var peerId, msg int
		fmt.Scanf("%d %d", &peerId, &msg)
		if peerId == serverId || peerId < 1 || peerId > 3 {
			fmt.Println("Shutting")
			s.DisconnectAll()
			s.Shutdown()
			fmt.Println("Ended")
			return
		} else {
			addr := Addr{"tcp", "[::]:" + ports[peerId]}
			err := s.ConnectToPeer(peerId, addr)
			var reply int
			if err == nil {
				s.RPC(peerId, "ServiceType.DisplayMsg", msg, &reply)
			}
		}
	}

}
