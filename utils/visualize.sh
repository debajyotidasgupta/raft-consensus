#!/bin/bash
# Example:   ./visualize.sh -t TestElectionNormal
clear # Clear the screen
echo "Ensure that debug is enabled in the raft/raft.go file"

# Get Args
while getopts ":t:" opt; do
  case $opt in
    t)
      TEST_NAME=$OPTARG
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      ;;
  esac
done

cd ../raft # Go to the raft directory
go test -v -run "$TEST_NAME$" -timeout=10m > ../utils/logs.txt # Run the test

cd ../utils
go run viz.go < logs.txt > output.txt

echo "Output saved to output.txt"