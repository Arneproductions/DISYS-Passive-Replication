package main

import (
	"flag"
	"passive-replication/internal/net"
	"passive-replication/internal/server"
	"strings"
)



func main() {

	replicasFlag := flag.String("replicas", "127.0.0.1:5001", "Replica ip addresses")
	leaderFlag := flag.Bool("leader", false, "Indicates whether it should be considered a leader to begin with")
	flag.Parse()
	
	replicas := strings.Split(*replicasFlag, ",")
	node := server.CreateNewNode(net.GetOutboundIp("172"), ParseLeaderFlag(*leaderFlag), "127.0.0.1:5001", replicas)
	
	node.StartServer()
}

func ParseLeaderFlag(isLeader bool) server.NodeStatus {
	if isLeader {
		return server.Leader
	} else {
		return server.Replica
	}
}