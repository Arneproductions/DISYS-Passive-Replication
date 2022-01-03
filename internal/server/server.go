package server

import (
	"context"
	"errors"
	"log"
	"net"
	grpcUtil "passive-replication/internal/grpc"
	proto "passive-replication/proto"
	"sync"

	"google.golang.org/grpc"
)

type NodeStatus int32

const (
	Leader          NodeStatus = 0
	Replica         NodeStatus = 1
	WaitingVotes    NodeStatus = 2
	ElectionOngoing NodeStatus = 3
)

type Node struct {
	status      NodeStatus
	ip          string
	leaderIp    string
	value       int32
	replicas    []string
	statusMutex sync.RWMutex
	valueMutex  sync.RWMutex
	proto.UnimplementedElectionServer
	proto.UnimplementedReplicationServer
}

func (n *Node) StartServer() {
	net.Listen("tcp", n.ip)
}

func (n *Node) Increment(ctx context.Context, message *proto.Empty) (*proto.ReplicaReply, error) {
	if n.HasStatus(Leader) {

		n.valueMutex.Lock()
		n.value++
		log.Printf("Value is being incremented (as a leader) to: %v", n.value)
		n.valueMutex.Unlock()

		// Update replicas
		for idx, v := range n.replicas {

			if len(v) == 0 {
				continue
			}

			if _, err := n.SendIncrement(v); err != nil {
				// Replica does not respond and is now declared dead
				n.replicas[idx] = ""
			}
		}

		return &proto.ReplicaReply{Value: n.value}, nil
	} else if n.HasStatus(Replica) {

		if n.requestIsFromLeader(ctx) {

			n.valueMutex.Lock()
			n.value++
			log.Printf("Value is being incremented to: %v", n.value)
			n.valueMutex.Unlock()
		} else {

			// Send to leader so it can handle it
			return n.SendIncrement(n.leaderIp)
		}
	}

	// If the node is not a Leader or a replica then return error
	return nil, errors.New("election is ongoing")
}

func (n *Node) SendIncrement(ip string) (*proto.ReplicaReply, error) {
	conn, err := grpc.Dial(ip, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Printf("Could not connect: %v\n", err)
	}
	defer conn.Close()

	c := proto.NewReplicationClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if resp, err := c.Increment(ctx, &proto.Empty{}); err != nil {
		return nil, err
	} else {
		return resp, nil
	}
}

func (n *Node) HasStatus(status NodeStatus) bool {
	n.statusMutex.RLock()
	defer n.statusMutex.RUnlock()

	return n.status == status
}

func (n *Node) requestIsFromLeader(ctx context.Context) bool {
	callerIp := grpcUtil.GetClientIpAddress(ctx)

	return n.leaderIp == callerIp
}
