#!/bin/bash

clear # Clear the screen

cd ../raft

# go test -v -race -run "$1"$ | grep "^.*\s\[[0-9]\]\s.*" | cat

go test -v -run TestElectionNormal$ > ../utils/logs

cd ../utils
go run viz.go < logs > viz.txt