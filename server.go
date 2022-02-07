package raft

import (
	//"fmt"
	//"log"
	//"math/rand"
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

func (s *Server) Serve() {
	/*
		s.mu.Lock()
		s.cm = NewConsensusModule(......)
	*/
	s.rpcServer = rpc.NewServer()
	//rpcProxy
	s.rpcServer.RegisterName("RaftModule")
}
