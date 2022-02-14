#!/bin/bash

clear # Clear the screen

# go test -v -race -run "$1"$ | grep "^.*\s\[[0-9]\]\s.*" | cat

go test -v -race -run TestElectionLeaderDisconnectAndReconnect$ | grep -E "^(.*\s\[[0-9]\]\s|===|---).*" | cat