package server

import (
	"context"
	"net"
	proto "passive-replication/proto"
	"sync"
)

type NodeStatus int32

const (
	Leader          NodeStatus = 0
	Replica         NodeStatus = 1
	WaitingVotes    NodeStatus = 2
	ElectionOngoing NodeStatus = 3
)

type Node struct {
	id          int32
	statusMutex sync.RWMutex
	status      NodeStatus
	ip          string
	proto.UnimplementedElectionServer
	proto.UnimplementedReplicationServer
}

func (n *Node) StartServer() {
	net.Listen("tcp", n.ip)
}

func (n *Node) HelloWorld(context.Context, *proto.ReplicationMessage) (*proto.Empty, error) {
	if n.HasStatus(Leader) {
		// TODO: Perform action

	} else if n.HasStatus(Replica) {
		// TODO: Reroute to leader?
	} else {
		// TODO: Return error because of ongoing leader election
	}

}

func (n *Node) HasStatus(status NodeStatus) bool {
	n.statusMutex.RLock()
	defer n.statusMutex.RUnlock()

	return n.status == status
}
