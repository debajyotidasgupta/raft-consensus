package main

import (
	"bufio"
	"encoding/gob"
	"fmt"
	"os"
	"raft-consensus/raft"
	"strconv"
	"strings"
)

//USER COMMANDS					ARGUMENTS
//1-> create cluster			number of nodes
//2-> set data					key, value, [peerId]
//3-> get data					key, [peerId]
//4-> disconnect peer			peerId
//5-> reconnect peer			peerId
//6-> crash peer				peerId
//7-> shutdown					_
//8-> check leader				_
//9-> stop execution			_

func main() {
	var input string
	var cluster *raft.ClusterSimulator = nil
	var peers int = 0
	fmt.Println("MENU:")
	fmt.Println("USER COMMANDS					ARGUMENTS")
	fmt.Println("1-> create cluster				number of nodes")
	fmt.Println("2-> set data					key, value, [peerId]")
	fmt.Println("3-> get data					key, [peerId]")
	fmt.Println("4-> disconnect peer				peerId")
	fmt.Println("5-> reconnect peer				peerId")
	fmt.Println("6-> crash peer					peerId")
	fmt.Println("7-> shutdown					_")
	fmt.Println("8-> check leader				_")
	fmt.Println("9-> stop execution				_")
	gob.Register(raft.Write{})
	gob.Register(raft.Read{})
	for {
		fmt.Println("WAITING FOR INPUTS..")
		reader := bufio.NewReader(os.Stdin)
		input, _ = reader.ReadString('\n')
		tokens := strings.Fields(input)
		command, err := strconv.Atoi(tokens[0])
		if err != nil {
			fmt.Println("Wrong input")
			continue
		}
		switch command {
		case 1:
			if len(tokens) < 2 {
				fmt.Println("Number of peers not passed")
				break
			}
			peers, err = strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Println("Invalid number of peers")
				break
			}
			cluster = raft.CreateNewCluster(nil, uint64(peers))
			if cluster != nil {
				fmt.Printf("CLUSTER OF %d PEERS CREATED !!!\n", peers)
			}
		case 2:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			if len(tokens) < 3 {
				fmt.Println("Key or value not passed")
				break
			}
			val, err := strconv.Atoi(tokens[2])
			if err != nil {
				fmt.Println("Invalid value passed")
				break
			}
			commandToServer := raft.Write{Key: tokens[1], Val: val}
			serverId := 0
			if len(tokens) >= 4 {
				serverId, err = strconv.Atoi(tokens[3])
				if err != nil || serverId >= peers {
					fmt.Printf("Invalid server id %d passed\n", serverId)
					break
				}
			} else {
				serverId, _ = cluster.CheckUniqueLeader()
				if serverId < 0 {
					fmt.Println("Unable to submit command to any server")
					break
				}
			}
			if success, _ := cluster.SubmitToServer(serverId, commandToServer); success {
				fmt.Printf("WRITE TO KEY %s WITH VALUE %d SUCCESSFUL\n", tokens[1], val)
			} else {
				fmt.Println("Command could not be submitted. Try different server")
			}
		case 3:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			if len(tokens) < 2 {
				fmt.Println("Key not passed")
				break
			}
			commandToServer := raft.Read{Key: tokens[1]}
			serverId := 0
			if len(tokens) >= 3 {
				serverId, err = strconv.Atoi(tokens[2])
				if err != nil || serverId >= peers {
					fmt.Printf("Invalid server id %d passed\n", serverId)
					break
				}
			} else {
				serverId, _ = cluster.CheckUniqueLeader()
				if serverId < 0 {
					fmt.Println("Unable to submit command to any server")
					break
				}
			}
			if success, reply := cluster.SubmitToServer(serverId, commandToServer); success {
				value, _ := reply.(int)
				fmt.Printf("READ KEY %s VALUE %d\n", tokens[1], value)
			} else {
				fmt.Println("Command could not be submitted. Try different server")
			}
		case 4:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			if len(tokens) < 2 {
				fmt.Println("Peer ID not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("Invalid server id %d passed\n", peer)
				break
			}
			cluster.DisconnectPeer(uint64(peer))
			fmt.Printf("PEER %d DISCONNECTED\n", peer)
		case 5:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			if len(tokens) < 2 {
				fmt.Println("Peer ID not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("Invalid server id %d passed\n", peer)
				break
			}
			cluster.ReconnectPeer(uint64(peer))
			fmt.Printf("PEER %d RECONNECTED\n", peer)
		case 6:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			if len(tokens) < 2 {
				fmt.Println("Peer ID not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("Invalid server id %d passed\n", peer)
				break
			}
			cluster.CrashPeer(uint64(peer))
			fmt.Printf("PEER %d CRASHED\n", peer)
		case 7:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			cluster.Shutdown()
			fmt.Println("ALL SERVERS STOPPED AND RAFT SERVICE STOPPED")
			cluster = nil
		case 8:
			if cluster == nil {
				fmt.Println("Raft cluster not created")
				break
			}
			leaderId, term := cluster.CheckUniqueLeader()
			if leaderId < 0 {
				fmt.Println("No leader yet :(")
			} else {
				fmt.Printf("LEADER ID: %d, TERM: %d\n", leaderId, term)
			}
		case 9:
			if cluster != nil {
				cluster.Shutdown()
			}
			fmt.Println("STOPPING EXECUTION, NO INPUTS WILL BE TAKEN FURTHER")
			return
		default:
			fmt.Println("Invalid Command")
		}
	}
}
