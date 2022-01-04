package server

import (
	"context"
	"math/rand"
	"errors"
	"log"
	"net"
	"os"
	grpcUtil "passive-replication/internal/grpc"
	proto "passive-replication/proto"
	"strings"
	"sync"
	"time"

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
	id          int32
	ip          string
	status      NodeStatus
	leaderIp    string
	value       int32
	replicas    []string
	electionTime time.Time
	statusMutex sync.RWMutex
	valueMutex  sync.RWMutex
	proto.UnimplementedElectionServer
	proto.UnimplementedReplicationServer
}

func CreateNewNode(ip string, status NodeStatus, leaderIp string, replicas []string) Node {

	log.Printf("Creating node with following parameters, IP: %s, Status: %v, Replica Endpoints: %v", ip, status, replicas)

	return Node{
		id:          int32(os.Getpid()),
		ip:          ip,
		status:      status,
		leaderIp:    leaderIp,
		value:       0,
		replicas:    replicas,
		electionTime: time.Now(),
		statusMutex: sync.RWMutex{},
		valueMutex:  sync.RWMutex{},
	}
}

func (n *Node) StartServer() {
	log.Printf("Starting server...")

	lis, err := net.Listen("tcp", ":5001")
	if err != nil {
		log.Printf("Failed to listen: %v", err)
	}

	s := grpc.NewServer()
	proto.RegisterReplicationServer(s, n)
	proto.RegisterElectionServer(s, n)

	log.Printf("Server listening on %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Printf("Failed to serve: %v", err)
	}
}

func (n *Node) Increment(ctx context.Context, message *proto.Empty) (*proto.ReplicaReply, error) {
	if n.HasStatus(Leader) {

		n.valueMutex.Lock()
		n.value++
		log.Printf("Value is being incremented (as a leader) to: %v", n.value)
		n.valueMutex.Unlock()

		// Update replicas
		for idx, v := range n.replicas {

			// Skip replica that is either declared dead or is itself
			if strings.HasPrefix(v, n.ip) || len(v) == 0 {
				continue
			}

			if _, err := n.SendIncrement(v); err != nil {
				// Replica does not respond and is now declared dead
				n.declareReplicaDead(idx)
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

func (n *Node) Election(ctx context.Context, in *proto.Empty) (*proto.ElectionMessage, error) {

	n.SetStatus(ElectionOngoing)

	return &proto.ElectionMessage{Value: n.value, ProcessId: n.id}, nil
}

func (n *Node) Elected(ctx context.Context, in *proto.ElectedMessage) (*proto.Empty, error) {

	n.leaderIp = in.GetLeaderIp()

	if strings.HasPrefix(n.leaderIp, n.ip) {
		n.SetStatus(Leader)
		go n.runHeartbeatProcess()
	} else {
		n.SetStatus(Replica)
		n.electionTime = time.Now()

		go n.runElectionProcess()
	}

	return &proto.Empty{}, nil
}

func (n *Node) Heartbeat(ctx context.Context, in *proto.HeartbeatMessage) (*proto.Empty, error) {
	n.replicas = in.GetReplicas() // Update list of replicas
	n.electionTime = time.Now()

	return &proto.Empty{}, nil
}

func (n *Node) SendElection() {

	n.SetStatus(WaitingVotes)

	currentHighestValue := n.value
	currentHighestIp := n.ip
	currentHighestId := n.id

	for idx, replicaIp := range n.replicas {

		// Do not perform heartbeats on nodes that are dead or to itself
		if strings.HasPrefix(replicaIp, n.ip) || len(replicaIp) == 0 {
			continue
		}

		conn, err := grpc.Dial(replicaIp, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Printf("Could not connect: %v\n", err)
		}
		defer conn.Close()

		c := proto.NewElectionClient(conn)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		voteReply, err := c.Election(ctx, &proto.Empty{})
		if err != nil {
			defer n.declareReplicaDead(idx)
		}

		if (voteReply.Value > currentHighestValue) || (voteReply.Value == currentHighestValue && voteReply.ProcessId > currentHighestId) {
			currentHighestId = voteReply.ProcessId
			currentHighestValue = voteReply.Value
			currentHighestIp = replicaIp
		}
	}

	n.SendElected(currentHighestIp)

	if strings.HasPrefix(currentHighestIp, n.ip) {
		n.SetStatus(Leader)
		go n.runHeartbeatProcess()
	} else {
		n.SetStatus(Replica)
		go n.runElectionProcess()
	}
}

func (n *Node) SendElected(electedIp string) {
	for idx, replicaIp := range n.replicas {

		// Do not send Elected to nodes that are dead or to itself
		if strings.HasPrefix(replicaIp, n.ip) || len(replicaIp) == 0 {
			continue
		}

		conn, err := grpc.Dial(replicaIp, grpc.WithInsecure(), grpc.WithBlock())
		if err != nil {
			log.Printf("Could not connect: %v\n", err)
		}
		defer conn.Close()

		c := proto.NewElectionClient(conn)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if _, err := c.Elected(ctx, &proto.ElectedMessage{LeaderIp: electedIp}); err != nil {

			defer n.declareReplicaDead(idx)
		}
	}
}

func (n *Node) SendHeartbeat() {

	for idx, replicaIp := range n.replicas {

		// Do not perform heartbeats on nodes that are dead or to itself
		if strings.HasPrefix(replicaIp, n.ip) || len(replicaIp) == 0 {
			continue
		}
		go func (idx int, ip string) {

			conn, err := grpc.Dial(ip, grpc.WithInsecure(), grpc.WithBlock())
			if err != nil {
				log.Printf("Could not connect: %v\n", err)
			}
			defer conn.Close()

			c := proto.NewElectionClient(conn)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			if _, err := c.Heartbeat(ctx, &proto.HeartbeatMessage{Replicas: n.replicas}); err != nil {

				defer n.declareReplicaDead(idx)
			}
		}(idx, replicaIp)
	}
}

func (n *Node) declareReplicaDead(index int) {
	n.replicas[index] = ""
}

func (n *Node) SetStatus(status NodeStatus) {
	n.statusMutex.Lock()
	defer n.statusMutex.Unlock()

	n.status = status
}

func (n *Node) HasStatus(status NodeStatus) bool {
	n.statusMutex.RLock()
	defer n.statusMutex.RUnlock()

	return n.status == status
}

func (n *Node) requestIsFromLeader(ctx context.Context) bool {
	callerIp := grpcUtil.GetClientIpAddress(ctx)

	return strings.HasPrefix(n.leaderIp, callerIp)
}

/*
	Is a process that keeps track of when to do an election. 
	It does not try to run an election if the process is in state 'ElectionOngoing' and the time since the 
*/
func (n *Node) runElectionProcess() {
	var t *time.Ticker = time.NewTicker(10 * time.Millisecond)
	randVal := rand.Intn(500) + 400
	var electionTimeDuration time.Duration = time.Millisecond * time.Duration(randVal) // interval between 400 and 900

	for {
		<-t.C

		if !n.HasStatus(Replica) {
			return // Do nothing... it will either elect someone soon or it is the leader
		} 

		if elapsed := time.Since(n.electionTime); elapsed >= electionTimeDuration {
			n.SendElection()
			return
		}
	}
}

func (n *Node) runHeartbeatProcess() {
	var t *time.Ticker = time.NewTicker(200 * time.Millisecond) // Triggers heartbeats after 200 miliseconds

	for {
		<-t.C
		
		if !n.HasStatus(Leader) {
			// Process is not the leader and should therefor not send heartbeats. Go back to electionTimer

			go n.runElectionProcess()
			return
		}

		n.SendHeartbeat()
	}
}
