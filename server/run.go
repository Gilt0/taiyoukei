package main

import (
	"errors"
	"flag"
	"log"
	"math"
	"net"
	"strconv"
	"sync"
	"time"

	pb "taiyoukei/proto"
	// protoc --go_out=. --go-grpc_out=. --proto_path=proto/ proto/celestial.proto

	"github.com/google/uuid"
	"google.golang.org/grpc"
)

const EMPTY_STR = ""
const UNDEFINED = "<undefined>"
const ZERO = 0
const ONE = 1

type argParser struct {
	port int
	n    int
}

func (parser *argParser) parse() {
	flag.IntVar(&parser.port, "server", 50051, "The port to use (in integer format)")
	flag.IntVar(&parser.n, "n", 3, "Number of Orbiting bodies: routine won't start unless that number is equal to the number of connections")
	flag.Parse()
}

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
	archive     pb.Data
	n           int
}

func (s *server) isReadyForBroadcast() bool {
	for _, c := range s.connections {
		if !c.isReadyForBroadcast() {
			return false
		}
	}
	return true
}

func (s *server) prepareBroadcastData(data *pb.Data) error {
	for _, c := range s.connections {
		if math.IsNaN(c.data.x) {
			return errors.New("x is nan")
		} else if math.IsNaN(c.data.y) {
			return errors.New("y is nan")
		} else if math.IsNaN(c.data.vx) {
			return errors.New("vx is nan")
		} else if math.IsNaN(c.data.vy) {
			return errors.New("vy is nan")
		}
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
	return nil
}

func (s *server) archiveBroadcastData(data *pb.Data) {
	s.mutex.Lock()
	s.archive.Success = data.Success
	for name, datum := range data.Content {
		s.archive.Content[name] = &pb.CelestialBody{
			Sequence: datum.Sequence,
			Name:     datum.Name,
			Mass:     datum.Mass,
			X:        datum.X,
			Y:        datum.Y,
			Vx:       datum.Vx,
			Vy:       datum.Vy,
		}
	}
	s.mutex.Unlock()
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

	stop := make(chan bool)

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
				if err := s.prepareBroadcastData(&data); err != nil {
					stop <- true
					break
				}
				// Keep an archive for frontend cilent through method CelestialBodiesPositions
				s.archiveBroadcastData(&data)
				// Resetting connection's data first to ensure that data acquisition
				// from clients can immediately resume
				s.resetConnectionsData()
				err := s.sendData(&data)
				if err != nil {
					stop <- true
					break
				}
			}
		}()
	}

	log.Printf("Initiating data reception for id %v", id)
forloop:
	for {
		select {
		case <-stop:
			break forloop
		default:
			data, err := stream.Recv()
			if err != nil {
				name := c.getName()
				log.Printf("Could not receive data stream from %v", name)
				s.mutex.Lock()
				delete(s.connections, id)
				s.mutex.Unlock()
				return err
			}
			s.mutex.Lock()
			c.setName(data.Name)
			c.setData(data)
			log.Printf("Received data from %v: %v", c.name, c.data)
			s.mutex.Unlock()

			for {
				if len(s.connections) == s.n {
					break
				}
			}
		}
	}
	return nil
}

func (s *server) CelestialBodiesPositions(req *pb.CelestialBodiesPositionRequest, stream pb.CelestialService_CelestialBodiesPositionsServer) error {
	log.Printf("Received CelestialBodiesPositions call")

	// Example of sending the latest data periodically
	// You can adjust this logic based on how your data is generated or stored
	ticker := time.NewTicker(time.Millisecond * 10)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mutex.Lock()
			if err := stream.Send(&s.archive); err != nil {
				log.Printf("Failed to send data: %v", err)
				s.mutex.Unlock()
				return err
			}
			s.mutex.Unlock()
		}
	}
}

func main() {
	parser := argParser{}
	parser.parse()
	port := parser.port
	n := parser.n
	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalf("Failed to start listener on port %v", port)
	}
	s := grpc.NewServer()
	pb.RegisterCelestialServiceServer(s, &server{
		connections: make(map[uuid.UUID]*connection),
		archive: pb.Data{
			Success: false,
			Content: make(map[string]*pb.CelestialBody),
		},
		n: n,
	})
	log.Printf("CelestialService started on port %v", port)
	if err = s.Serve(lis); err != nil {
		log.Fatalf("Failed to start grpc server on port %v", port)
	}
}
