package main

import (
	"errors"
	"log"
	"net"
	"strconv"
	"sync"

	pb "taiyoukei/proto"

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const EMPTY_STR = ""
const UNDEFINED = "<undefined>"
const ZERO = 0
const ONE = 1

type lightCelestialBody struct {
	sequence uint64
	name     string
	mass     float64
	x        float64
	y        float64
	vx       float64
	vy       float64
}

type connection struct {
	id     uuid.UUID
	name   string
	data   lightCelestialBody
	stream pb.CelestialService_CelestialUpdateServer
}

func (c *connection) setName(name string) {
	c.name = name
}

func (c *connection) setData(data *pb.CelestialBody) error {
	if c.data != (lightCelestialBody{}) {
		return errors.New("data is already set")
	}
	c.data = lightCelestialBody{
		sequence: data.Sequence,
		name:     data.Name,
		mass:     data.Mass,
		x:        data.X,
		y:        data.Y,
		vx:       data.Vx,
		vy:       data.Vy,
	}
	return nil
}

func (c *connection) resetData() {
	c.data = lightCelestialBody{}
}

func (c *connection) getName() string {
	name := c.name
	if name == EMPTY_STR {
		return UNDEFINED
	}
	return name
}

func (c *connection) isReadyForBroadcast() bool {
	return c.data != lightCelestialBody{}
}

type server struct {
	pb.UnimplementedCelestialServiceServer
	mutex       sync.Mutex
	connections map[uuid.UUID]*connection
}

func (s *server) isReadyForBroadcast() bool {
	for _, c := range s.connections {
		if !c.isReadyForBroadcast() {
			return false
		}
	}
	return true
}

func (s *server) prepareBroadcastData(data *pb.Data) {
	for _, c := range s.connections {
		data.Content[c.data.name] = &pb.CelestialBody{
			Sequence: c.data.sequence,
			Name:     c.data.name,
			Mass:     c.data.mass,
			X:        c.data.x,
			Y:        c.data.y,
			Vx:       c.data.vx,
			Vy:       c.data.vy,
		}
	}
}

func (s *server) resetConnectionsData() {
	for _, c := range s.connections {
		c.resetData()
	}
}

func (s *server) sendData(data *pb.Data) error {
	for _, c := range s.connections {
		err := c.stream.Send(data)
		log.Printf("Sent data to %v: %v", c.name, data)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *server) CelestialUpdate(stream pb.CelestialService_CelestialUpdateServer) error {

	log.Printf("New connection")

	id, err := uuid.NewUUID()
	if err != nil {
		log.Printf("Error generating uuid %v", err)
		return err
	}
	c := connection{
		id:     id,
		name:   "",
		data:   lightCelestialBody{},
		stream: stream,
	}

	s.mutex.Lock()
	s.connections[id] = &c
	n_connections := len(s.connections)
	s.mutex.Unlock()

	if n_connections == ONE {
		go func() {
			for {
				s.mutex.Lock()
				isReadyForBroadcast := s.isReadyForBroadcast()
				n_connections := len(s.connections)
				s.mutex.Unlock()
				if n_connections == ZERO {
					break // Cancel goroutine if there are no clients
				}
				if !isReadyForBroadcast {
					continue
				}
				// No mutex necessary as the clients are not sending anything
				// (they are waiting for the broadcast!)
				data := pb.Data{
					Success: true,
					Content: make(map[string]*pb.CelestialBody),
				}
				s.prepareBroadcastData(&data)
				// Resetting connection's data first to ensure that data acquisition
				// from clients can immediately resume
				s.resetConnectionsData()
				err := s.sendData(&data)
				if err != nil {
					break
				}
			}
		}()
	}

	log.Printf("Initiating data reception for id %v", id)
	for {
		data, err := stream.Recv()
		if err != nil {
			name := c.getName()
			log.Printf("Could not receive data stream from %v", name)
			s.mutex.Lock()
			delete(s.connections, id)
			s.mutex.Unlock()
			return err
		}
		c.setName(data.Name)
		c.setData(data)
		log.Printf("Received data from %v: %v", c.name, c.data)
	}
}

func main() {
	port := 50051
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalf("Failed to start listener on port %v", port)
	}
	s := grpc.NewServer()
	pb.RegisterCelestialServiceServer(s, &server{
		connections: make(map[uuid.UUID]*connection),
	})
	log.Printf("CelestialService started on port %v", port)
	if err = s.Serve(lis); err != nil {
		log.Fatalf("Failed to start grpc server on port %v", port)
	}
}
