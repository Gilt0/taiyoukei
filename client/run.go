package main

import (
	"context"
	"flag"
	"log"
	"time"

	pb "taiyoukei/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func parseFlags() (server, name string, mass, x, y, vx, vy float64) {
	flag.StringVar(&server, "server", ":50051", "The server address in the format of host:port")
	flag.StringVar(&name, "name", "", "Name of the celestial body")
	flag.Float64Var(&mass, "mass", 0, "Mass of the celestial body")
	flag.Float64Var(&x, "x", 0, "Initial x-position")
	flag.Float64Var(&y, "y", 0, "Initial y-position")
	flag.Float64Var(&vx, "vx", 0, "Initial x-speed")
	flag.Float64Var(&vy, "vy", 0, "Initial y-speed")
	flag.Parse()
	return
}

func main() {

	server, name, mass, x, y, vx, vy := parseFlags()

	conn, err := grpc.Dial(server, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewCelestialServiceClient(conn)
	stream, err := client.CelestialUpdate(context.Background())
	if err != nil {
		conn.Close()
		log.Fatalf("did not create stream: %v", err)
	}
	data := pb.CelestialBody{
		Name: name,
		Mass: mass,
		X:    x,
		Y:    y,
		Vx:   vx,
		Vy:   vy,
	}
	err = stream.Send(&data)
	if err != nil {
		conn.Close()
		log.Fatalf("could not send initial request: %v", err)
	}
	log.Printf("Sending initial data %v\n", &data)
	n_sequence := 1

	for {
		broadcast, err := stream.Recv()
		if err != nil {
			log.Fatalf("Failed to receive broadcast: %v", err)
		}

		log.Printf("Received broadcast update %v\n", broadcast)

		// Increment values
		data.Sequence += 1.0
		data.X += 1.0
		data.Y += 1.0
		data.Vx += 1.0
		data.Vy += 1.0

		// Send updated values
		err = stream.Send(&data)
		if err != nil {
			conn.Close()
			log.Fatalf("failed to send update: %v", err)
		}
		log.Printf("Sending update %v\n", &data)
		n_sequence += 1

		time.Sleep(1000000000)

	}

}
