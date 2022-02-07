package raft

import (
	//"fmt"
	//"log"
	//"math/rand"
	"log"
	"net"
	"net/rpc"

	//"os"
	"sync"
	//"time"
)

// Server wraps a CM and RPC SERVER
type Server struct {
	mu sync.Mutex

	serverId int
	peerIds  []int

	// cm *ConsensusModule
	// storage Storage
	// rpcProxy *RPCProxy

	rpcServer *rpc.Server
	listener  net.Listener

	// commitChan chan<- CommmitEntry
	peerClients map[int]*rpc.Client

	//ready <-chan interface{}
	//quit  chan interface{}
	wg sync.WaitGroup
}

func CreateServer(serverId int, peerIds []int /*,storage*/ /*,ready <-chan interface{}*/ /*commitChan*/) *Server {
	s := new(Server)
	s.serverId = serverId
	s.peerIds = peerIds
	s.peerClients = make(map[int]*rpc.Client)
	//server.ready = ready
	//server.quit = make(chan interface{})
	return s
}

func (s *Server) connectionAccept() {
	defer s.wg.Done()

	for {
		listener, err := s.listener.Accept()
		if err != nil {
			//quit case
			log.Fatal("accept error:", err)
		}
		s.wg.Add(1)
		go func() {
			s.rpcServer.ServeConn(listener)
			s.wg.Done()
		}()
	}
}

func (s *Server) Serve() {
	s.mu.Lock()
	//s.cm = NewConsensusModule(......)

	s.rpcServer = rpc.NewServer()
	//rpcProxy
	//s.rpcServer.RegisterName("RaftModule",s.rpcProxy)

	var err error
	s.listener, err = net.Listen("tcp", "0")
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[%v] listening at %s", s.serverId, s.listener.Addr())
	s.mu.Unlock()

	s.wg.Add(1)

	go s.connectionAccept()
}

func (s *Server) disconnectPeer(peerId int) {
	if s.peerClients[peerId] != nil {
		s.peerClients[peerId].Close()
		s.peerClients[peerId] = nil
	}
}

func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.peerClients {
		s.disconnectPeer(id)
	}
}

func (s *Server) Shutdown() {
	//s.cm.Stop()
	//close(s.quit)
	s.listener.Close()
	s.wg.Wait()
}
