package raft

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

type ServiceType uint64

// Server wraps a RPC Server
// It will also wrap Raft service object
type Server struct {
	mu        sync.Mutex             // mutual exclusion for accessing server members
	serverId  uint64                 // id of this server
	peerIds   []uint64               // peerIds that this server will connect as a client
	rpcServer *rpc.Server            // RPC Server
	listener  net.Listener           // listener to keep listening for incoming connections
	peers     map[uint64]*rpc.Client // maps peerId to corresponding peer
	quit      chan interface{}       // channel to indicate to stop listening for incoming connections
	wg        sync.WaitGroup         // waitgroup to wait for all connections to close before gracefully stopping

	rn         *RaftNode
	db         *Database
	rpcProxy   *RPCProxy
	commitChan chan CommitEntry
	ready      <-chan interface{}

	service *ServiceType // DUMMY SERVICE FOR TESTING PURPOSES
}

type RPCProxy struct {
	rn *RaftNode
}

//create a Server Instance with serverId and list of peerIds
func CreateServer(serverId uint64, peerIds []uint64, db *Database, ready <-chan interface{}, commitChan chan CommitEntry) *Server {
	s := new(Server)
	s.serverId = serverId
	s.peerIds = peerIds
	s.peers = make(map[uint64]*rpc.Client)
	s.db = db
	s.ready = ready
	s.commitChan = commitChan
	s.quit = make(chan interface{})
	return s
}

//keep listening for incoming connections in a loop
//on accepting a connection start a go routine to serve the connection
func (s *Server) ConnectionAccept() {
	defer s.wg.Done()

	for {
		fmt.Printf("[%d] Listening\n", s.serverId)
		listener, err := s.listener.Accept() // wait to accept an incoming connection
		if err != nil {
			select {
			case <-s.quit: // quit listening
				fmt.Printf("[%d] Accepting no more connections\n", s.serverId)
				return
			default:
				log.Fatalf("[%d] Error in accepting %v\n", s.serverId, err)
			}
		}
		s.wg.Add(1) // serve the new accepted connection in a separate go routine
		go func() {
			s.rpcServer.ServeConn(listener)
			s.wg.Done()
		}()
	}
}

//start a new service ->
//1. create the RPC Server
//2. register the service with RPC
//3. get a lister for TCP port passed as argument
//4. start listening for incoming connections
func (s *Server) Serve(port ...string) {
	s.mu.Lock()
	s.rn = NewRaftNode(s.serverId, s.peerIds, s, s.db, s.ready, s.commitChan)

	s.rpcServer = rpc.NewServer() //create a new RPC Server for the new service
	s.rpcProxy = &RPCProxy{rn: s.rn}
	//MIGHT ADD PROXY LATER
	//s.rpcServer.RegisterName("RaftNode", s.rpcProxy) //register the new service
	s.rpcServer.RegisterName("RaftNode", s.rn)

	var st ServiceType = ServiceType(1)
	s.service = &st
	s.rpcServer.RegisterName("ServiceType", s.service)

	var err error
	var tcpPort string = ":"
	if len(port) == 1 {
		tcpPort = tcpPort + port[0]
	} else {
		tcpPort = tcpPort + "0"
	}
	s.listener, err = net.Listen("tcp", tcpPort) //get a listener to the tcp port
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[%v] listening at %s\n", s.serverId, s.listener.Addr())
	s.mu.Unlock()

	s.wg.Add(1)

	go func() {
		defer s.wg.Done()

		for {
			conn, err := s.listener.Accept()
			if err != nil {
				select {
				case <-s.quit:
					return
				default:
					log.Fatal("accept error: ", err)
				}
			}
			s.wg.Add(1)
			go func() {
				s.rpcServer.ServeConn(conn)
				s.wg.Done()
			}()
		}
	}()
}

//close connections to all peers
func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id := range s.peers {
		if s.peers[id] != nil {
			s.peers[id].Close()
			s.peers[id] = nil
		}
	}
}

//stop the server
func (s *Server) Stop() {
	s.rn.Stop()
	close(s.quit)      // indicate the listener to stop listening
	s.listener.Close() // close the listener

	fmt.Printf("[%d] Waiting for existing connections to close\n", s.serverId)
	s.wg.Wait() // wait for all existing connections to close

	fmt.Printf("[%d] All connections closed. Stopping server\n", s.serverId)
}

func (s *Server) GetListenerAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener.Addr()
}

//connect to a peer
func (s *Server) ConnectToPeer(peerId uint64, addr net.Addr) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	// if not already connected to the peer
	if s.peers[peerId] == nil {
		peer, err := rpc.Dial(addr.Network(), addr.String()) // dial to eh network address of the peer server
		if err != nil {
			return err
		}
		s.peers[peerId] = peer // assign the peer client
	}
	return nil
}

//disconnect from a particular peer
func (s *Server) DisconnectPeer(peerId uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	peer := s.peers[peerId]
	if peer != nil {
		err := peer.Close()
		s.peers[peerId] = nil
		return err
	}
	return nil
}

//make an RPC call to the particular peer
func (s *Server) RPC(peerId uint64, rpcCall string, args interface{}, reply interface{}) error {
	s.mu.Lock()
	peer := s.peers[peerId] //obtain the peer client
	s.mu.Unlock()

	if peer == nil {
		return fmt.Errorf("[%d] RPC call to peer %d after it is closed", s.serverId, peerId)
	} else {
		// call RPC corresponding to the particular peer connection
		return peer.Call(rpcCall, args, reply)
	}
}

//A DUMMY RPC FUNCTION
func (s *ServiceType) DisplayMsg(args uint64, reply *uint64) error {
	fmt.Printf("received %d\n", args)
	*reply = 2 * args
	return nil
}
