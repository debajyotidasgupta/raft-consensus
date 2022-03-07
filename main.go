package main

import (
	"bufio"
	"encoding/gob"
	"errors"
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

func CreateCluster(peers int) (*raft.ClusterSimulator, error) {
	if peers < 0 {
		return nil, errors.New("invalid number of peers")
	}
	cluster := raft.CreateNewCluster(nil, uint64(peers))
	if cluster != nil {
		return cluster, nil
	}
	return nil, errors.New("cluster could not be created")
}

func SetData(cluster *raft.ClusterSimulator, key string, val int, serverParam ...int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	commandToServer := raft.Write{Key: key, Val: val}
	serverId := 0
	if len(serverParam) >= 1 {
		serverId = serverParam[0]
	} else {
		serverId, _ = cluster.CheckUniqueLeader()
	}
	if serverId < 0 {
		return errors.New("unable to submit command to any server")
	}
	success := false
	if success, _ = cluster.SubmitToServer(serverId, commandToServer); success {
		return nil
	} else {
		return errors.New("command could not be submitted, try different server")
	}
}

func GetData(cluster *raft.ClusterSimulator, key string, serverParam ...int) (int, error) {
	if cluster == nil {
		return 0, errors.New("raft cluster not created")
	}
	commandToServer := raft.Read{Key: key}
	serverId := 0
	if len(serverParam) >= 1 {
		serverId = serverParam[0]
	} else {
		serverId, _ = cluster.CheckUniqueLeader()
	}
	if serverId < 0 {
		return 0, errors.New("unable to submit command to any server")
	}
	if success, reply := cluster.SubmitToServer(serverId, commandToServer); success {
		value, _ := reply.(int)
		return value, nil
	} else {
		return 0, errors.New("command could not be submitted, try different server")
	}
}

func DisconnectPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	cluster.DisconnectPeer(uint64(peerId))
	return nil
}

func ReconnectPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	cluster.ReconnectPeer(uint64(peerId))
	return nil
}

func CrashPeer(cluster *raft.ClusterSimulator, peerId int) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	if peerId < 0 {
		return errors.New("invalid peer id passed")
	}
	cluster.CrashPeer(uint64(peerId))
	return nil
}

func Shutdown(cluster *raft.ClusterSimulator) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	cluster.Shutdown()
	cluster = nil
	return nil
}

func CheckLeader(cluster *raft.ClusterSimulator) (int, int, error) {
	if cluster == nil {
		return -1, -1, errors.New("raft cluster not created")
	}
	leaderId, term := cluster.CheckUniqueLeader()
	if leaderId < 0 {
		return leaderId, -1, errors.New("no leader yet")
	} else {
		return leaderId, term, nil
	}
}

func Stop(cluster *raft.ClusterSimulator) error {
	if cluster == nil {
		return errors.New("raft cluster not created")
	}
	cluster.Shutdown()
	cluster = nil
	return nil
}

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
		command, err0 := strconv.Atoi(tokens[0])
		if err0 != nil {
			fmt.Println("Wrong input")
			continue
		}
		switch command {
		case 1:
			if len(tokens) < 2 {
				fmt.Println("number of peers not passed")
				break
			}
			var err error
			peers, err = strconv.Atoi(tokens[1])
			if err != nil {
				fmt.Println("invalid number of peers")
				break
			}
			cluster, err = CreateCluster(peers)
			if err == nil {
				fmt.Printf("CLUSTER OF %d PEERS CREATED !!!\n", peers)
			} else {
				fmt.Printf("err: %v\n", err)
				cluster = nil
			}
		case 2:
			if len(tokens) < 3 {
				fmt.Println("key or value not passed")
				break
			}
			val, err := strconv.Atoi(tokens[2])
			if err != nil {
				fmt.Println("invalid value passed")
				break
			}
			serverId := 0
			if len(tokens) >= 4 {
				serverId, err = strconv.Atoi(tokens[3])
				if err != nil || serverId >= peers {
					fmt.Printf("invalid server id %d passed\n", serverId)
					break
				}
				err = SetData(cluster, tokens[1], val, serverId)
			} else {
				err = SetData(cluster, tokens[1], val)
			}
			if err == nil {
				fmt.Printf("WRITE TO KEY %s WITH VALUE %d SUCCESSFUL\n", tokens[1], val)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 3:
			if len(tokens) < 2 {
				fmt.Println("key not passed")
				break
			}
			var err error
			var val int
			serverId := 0
			if len(tokens) >= 3 {
				serverId, err = strconv.Atoi(tokens[2])
				if err != nil || serverId >= peers {
					fmt.Printf("invalid server id %d passed\n", serverId)
					break
				}
				val, err = GetData(cluster, tokens[1], serverId)
			} else {
				val, err = GetData(cluster, tokens[1])
			}
			if err == nil {
				fmt.Printf("READ KEY %s VALUE %d\n", tokens[1], val)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 4:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}

			err = DisconnectPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d DISCONNECTED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 5:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = ReconnectPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d DISCONNECTED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 6:
			if len(tokens) < 2 {
				fmt.Println("peer id not passed")
				break
			}
			peer, err := strconv.Atoi(tokens[1])
			if err != nil || peer >= peers {
				fmt.Printf("invalid server id %d passed\n", peer)
				break
			}
			err = CrashPeer(cluster, peer)
			if err == nil {
				fmt.Printf("PEER %d CRASHED\n", peer)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 7:
			err := Shutdown(cluster)
			if err == nil {
				fmt.Println("ALL SERVERS STOPPED AND RAFT SERVICE STOPPED")
			} else {
				fmt.Printf("%v\n", err)
			}
			cluster = nil
		case 8:
			leaderId, term, err := CheckLeader(cluster)
			if err == nil {
				fmt.Printf("LEADER ID: %d, TERM: %d\n", leaderId, term)
			} else {
				fmt.Printf("%v\n", err)
			}
		case 9:
			err := Stop(cluster)
			if err == nil {
				fmt.Println("STOPPING EXECUTION, NO INPUTS WILL BE TAKEN FURTHER")
				cluster = nil
				return
			} else {
				fmt.Printf("%v\n", err)
			}
		default:
			fmt.Println("Invalid Command")
		}
	}
}

// func main() {
// 	var input string
// 	var cluster *raft.ClusterSimulator = nil
// 	var peers int = 0
// 	fmt.Println("MENU:")
// 	fmt.Println("USER COMMANDS					ARGUMENTS")
// 	fmt.Println("1-> create cluster				number of nodes")
// 	fmt.Println("2-> set data					key, value, [peerId]")
// 	fmt.Println("3-> get data					key, [peerId]")
// 	fmt.Println("4-> disconnect peer				peerId")
// 	fmt.Println("5-> reconnect peer				peerId")
// 	fmt.Println("6-> crash peer					peerId")
// 	fmt.Println("7-> shutdown					_")
// 	fmt.Println("8-> check leader				_")
// 	fmt.Println("9-> stop execution				_")
// 	gob.Register(raft.Write{})
// 	gob.Register(raft.Read{})
// 	for {
// 		fmt.Println("WAITING FOR INPUTS..")
// 		reader := bufio.NewReader(os.Stdin)
// 		input, _ = reader.ReadString('\n')
// 		tokens := strings.Fields(input)
// 		command, err := strconv.Atoi(tokens[0])
// 		if err != nil {
// 			fmt.Println("Wrong input")
// 			continue
// 		}
// 		switch command {
// 		case 1:
// 			if len(tokens) < 2 {
// 				fmt.Println("Number of peers not passed")
// 				break
// 			}
// 			peers, err = strconv.Atoi(tokens[1])
// 			if err != nil {
// 				fmt.Println("Invalid number of peers")
// 				break
// 			}
// 			cluster = raft.CreateNewCluster(nil, uint64(peers))
// 			if cluster != nil {
// 				fmt.Printf("CLUSTER OF %d PEERS CREATED !!!\n", peers)
// 			}
// 		case 2:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			if len(tokens) < 3 {
// 				fmt.Println("Key or value not passed")
// 				break
// 			}
// 			val, err := strconv.Atoi(tokens[2])
// 			if err != nil {
// 				fmt.Println("Invalid value passed")
// 				break
// 			}
// 			commandToServer := raft.Write{Key: tokens[1], Val: val}
// 			serverId := 0
// 			if len(tokens) >= 4 {
// 				serverId, err = strconv.Atoi(tokens[3])
// 				if err != nil || serverId >= peers {
// 					fmt.Printf("Invalid server id %d passed\n", serverId)
// 					break
// 				}
// 			} else {
// 				serverId, _ = cluster.CheckUniqueLeader()
// 				if serverId < 0 {
// 					fmt.Println("Unable to submit command to any server")
// 					break
// 				}
// 			}
// 			if success, _ := cluster.SubmitToServer(serverId, commandToServer); success {
// 				fmt.Printf("WRITE TO KEY %s WITH VALUE %d SUCCESSFUL\n", tokens[1], val)
// 			} else {
// 				fmt.Println("Command could not be submitted. Try different server")
// 			}
// 		case 3:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			if len(tokens) < 2 {
// 				fmt.Println("Key not passed")
// 				break
// 			}
// 			commandToServer := raft.Read{Key: tokens[1]}
// 			serverId := 0
// 			if len(tokens) >= 3 {
// 				serverId, err = strconv.Atoi(tokens[2])
// 				if err != nil || serverId >= peers {
// 					fmt.Printf("Invalid server id %d passed\n", serverId)
// 					break
// 				}
// 			} else {
// 				serverId, _ = cluster.CheckUniqueLeader()
// 				if serverId < 0 {
// 					fmt.Println("Unable to submit command to any server")
// 					break
// 				}
// 			}
// 			if success, reply := cluster.SubmitToServer(serverId, commandToServer); success {
// 				value, _ := reply.(int)
// 				fmt.Printf("READ KEY %s VALUE %d\n", tokens[1], value)
// 			} else {
// 				fmt.Println("Command could not be submitted. Try different server")
// 			}
// 		case 4:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			if len(tokens) < 2 {
// 				fmt.Println("Peer ID not passed")
// 				break
// 			}
// 			peer, err := strconv.Atoi(tokens[1])
// 			if err != nil || peer >= peers {
// 				fmt.Printf("Invalid server id %d passed\n", peer)
// 				break
// 			}
// 			cluster.DisconnectPeer(uint64(peer))
// 			fmt.Printf("PEER %d DISCONNECTED\n", peer)
// 		case 5:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			if len(tokens) < 2 {
// 				fmt.Println("Peer ID not passed")
// 				break
// 			}
// 			peer, err := strconv.Atoi(tokens[1])
// 			if err != nil || peer >= peers {
// 				fmt.Printf("Invalid server id %d passed\n", peer)
// 				break
// 			}
// 			cluster.ReconnectPeer(uint64(peer))
// 			fmt.Printf("PEER %d RECONNECTED\n", peer)
// 		case 6:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			if len(tokens) < 2 {
// 				fmt.Println("Peer ID not passed")
// 				break
// 			}
// 			peer, err := strconv.Atoi(tokens[1])
// 			if err != nil || peer >= peers {
// 				fmt.Printf("Invalid server id %d passed\n", peer)
// 				break
// 			}
// 			cluster.CrashPeer(uint64(peer))
// 			fmt.Printf("PEER %d CRASHED\n", peer)
// 		case 7:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			cluster.Shutdown()
// 			fmt.Println("ALL SERVERS STOPPED AND RAFT SERVICE STOPPED")
// 			cluster = nil
// 		case 8:
// 			if cluster == nil {
// 				fmt.Println("Raft cluster not created")
// 				break
// 			}
// 			leaderId, term := cluster.CheckUniqueLeader()
// 			if leaderId < 0 {
// 				fmt.Println("No leader yet :(")
// 			} else {
// 				fmt.Printf("LEADER ID: %d, TERM: %d\n", leaderId, term)
// 			}
// 		case 9:
// 			if cluster != nil {
// 				cluster.Shutdown()
// 			}
// 			fmt.Println("STOPPING EXECUTION, NO INPUTS WILL BE TAKEN FURTHER")
// 			return
// 		default:
// 			fmt.Println("Invalid Command")
// 		}
// 	}
// }
