package raft

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"sync"
)

type ServiceType int

// Server wraps a RPC Server
// It will also wrap Raft service object
type Server struct {
	mu        sync.Mutex          // mutual exclusion for accessing server members
	serverId  int                 // id of this server
	peerIds   []int               // peerIds that this server will connect as a client
	rpcServer *rpc.Server         // RPC Server
	listener  net.Listener        // listener to keep listening for incoming connections
	peers     map[int]*rpc.Client // maps peerId to corresponding peer
	quit      chan interface{}    // channel to indicate to stop listening for incoming connections
	wg        sync.WaitGroup      // waitgroup to wait for all connections to close before gracefully stopping
	service   *ServiceType        // DUMMY RPC SERVICE FOR TEST ONLY

	// cm *ConsensusModule
	// storage Storage
	// rpcProxy *RPCProxy
	// commitChan chan<- CommmitEntry
	//ready <-chan interface{}
}

//create a Server Instance with serverId and list of peerIds
func CreateServer(serverId int, peerIds []int /*,storage*/ /*,ready <-chan interface{}*/ /*commitChan*/) *Server {
	s := &Server{}
	s.serverId = serverId
	s.peerIds = peerIds
	s.peers = make(map[int]*rpc.Client)
	//server.ready = ready
	s.quit = make(chan interface{})
	return s
}

//keep listening for incoming connections in a loop
//on accepting a connection start a go routine to serve the connection
func (s *Server) ConnectionAccept() {
	defer s.wg.Done()

	for {
		fmt.Printf("%d Listening\n", s.serverId)
		listener, err := s.listener.Accept() // wait to accept an incoming connection
		if err != nil {
			select {
			case <-s.quit: // quit listening
				fmt.Println("No more accepting connections")
				return
			default:
				log.Fatal("accept error:", err)
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
func (s *Server) Serve(port string) {
	s.mu.Lock()

	s.service = new(ServiceType) //create a new service for which RPC is to be served
	dummy := ServiceType(1)
	s.service = &dummy
	s.rpcServer = rpc.NewServer() //create a new RPC Server for the new service

	s.rpcServer.RegisterName("ServiceType", s.service) //register the new service

	var err error
	s.listener, err = net.Listen("tcp", ":"+port) //get a listener to the tcp port
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("[%v] listening at %s", s.serverId, s.listener.Addr())
	s.mu.Unlock()

	s.wg.Add(1)

	go s.ConnectionAccept() //start listening for incoming connections
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
	//s.cm.Stop()
	close(s.quit)      // indicate the listener to stop listening
	s.listener.Close() // close the listener

	fmt.Println("Waiting for existing connections to close")
	s.wg.Wait() // wait for all existing connections to close

	fmt.Println("All connections closed")
}

func (s *Server) GetListenerAddr() net.Addr {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener.Addr()
}

//connect to a peer
func (s *Server) ConnectToPeer(peerId int, addr net.Addr) error {
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
func (s *Server) DisconnectPeer(peerId int) error {
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
func (s *Server) RPC(peerId int, rpcCall string, args interface{}, reply interface{}) error {
	s.mu.Lock()
	peer := s.peers[peerId] //obtain the peer client
	s.mu.Unlock()

	if peer == nil {
		return fmt.Errorf("RPC call to peer %d after it is closed", peerId)
	} else {
		// call RPC corresponding to the particular peer connection
		return peer.Call(rpcCall, args, reply)
	}
}

//A DUMMY RPC FUNCTION
func (s *ServiceType) DisplayMsg(args int, reply *int) error {
	fmt.Println(args)
	*reply = 2 * args
	return nil
}
