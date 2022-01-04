package main

import(
	"flag"
	"log"
	"context"
	"google.golang.org/grpc"
	proto "passive-replication/proto"
)

var (
	serverAddr = flag.String("serverAddr", "localhost:5001", "Server to connect to")
)

func main(){
	flag.Parse()

	conn, err := grpc.Dial(*serverAddr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		log.Printf("Could not connect: %v\n", err)
	}
	defer conn.Close()

	c := proto.NewReplicationClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inc(c, ctx)
	
}

func inc(c proto.ReplicationClient, ctx context.Context) (int32,error) {
	reply, err := c.Increment(ctx, &proto.Empty{})

	if err != nil {
		log.Printf("Failed to increment: %v\n", err)
		return 0,err
	}

	return reply.Value, nil
}