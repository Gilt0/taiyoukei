package main

import (
	"context"
	"flag"
	"log"
	"math"

	pb "taiyoukei/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// -------------------------------------------------------------------------------------
// Argument parser

type argParser struct {
	server string
	name   string
	mass   float64
	x      float64
	y      float64
	vx     float64
	vy     float64
}

func (parser *argParser) parse() {
	flag.StringVar(&parser.server, "server", ":50051", "The server address in the format of host:port")
	flag.StringVar(&parser.name, "name", "", "Name of the celestial body")
	flag.Float64Var(&parser.mass, "mass", 1, "Mass of the celestial body")
	flag.Float64Var(&parser.x, "x", 1, "Initial x-position")
	flag.Float64Var(&parser.y, "y", 1, "Initial y-position")
	flag.Float64Var(&parser.vx, "vx", 1, "Initial x-speed")
	flag.Float64Var(&parser.vy, "vy", 1, "Initial y-speed")
	flag.Parse()
}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Light weight struct to carry around the fundalental necessary data

type lightCelestialBody struct {
	sequence uint64
	name     string
	mass     float64
	x        float64
	y        float64
	vx       float64
	vy       float64
}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Builder for a connection object

type celestialConnectionBuilder struct {
	server    string
	solver    Solver
	fh        celestialRateFunctionHandler
	init_data lightCelestialBody
}

func (builder *celestialConnectionBuilder) set_server(server string) {
	builder.server = server
}

func (builder *celestialConnectionBuilder) set_solver(solver Solver) {
	builder.solver = solver
}

func (builder *celestialConnectionBuilder) set_rateFunctionHandler(fh celestialRateFunctionHandler) {
	builder.fh = fh
}

func (builder *celestialConnectionBuilder) set_initial_data(init_data lightCelestialBody) {
	builder.init_data = init_data
}

func (builder *celestialConnectionBuilder) build() (celestialConnection, error) {
	conn, err := grpc.Dial(builder.server, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		conn.Close()
		log.Printf("did not connect to server: %v", err)
		return celestialConnection{}, err
	}
	client := pb.NewCelestialServiceClient(conn)
	stream, err := client.CelestialUpdate(context.Background())
	if err != nil {
		conn.Close()
		log.Printf("did not create stream: %v", err)
		return celestialConnection{}, err
	}
	c := celestialConnection{
		conn:   conn,
		stream: stream,
		data:   builder.init_data,
		solver: builder.solver,
		fh:     builder.fh,
	}
	return c, nil
}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Connection object

type celestialConnection struct {
	conn   *grpc.ClientConn
	stream pb.CelestialService_CelestialUpdateClient
	data   lightCelestialBody
	solver Solver
	fh     celestialRateFunctionHandler
}

func (c *celestialConnection) sendUpdate() error {
	data := pb.CelestialBody{
		Sequence: c.data.sequence,
		Name:     c.data.name,
		Mass:     c.data.mass,
		X:        c.data.x,
		Y:        c.data.y,
		Vx:       c.data.vx,
		Vy:       c.data.vy,
	}

	err := c.stream.Send(&data)
	if err != nil {
		log.Printf("could not send update: %v", err)
		return err
	}
	log.Printf("Sending update %v\n", &data)
	return nil
}

func (c *celestialConnection) closeConnection() {
	c.conn.Close()
}

func (c *celestialConnection) parseBroadcastData(broadcast *pb.Data) (self lightCelestialBody, others []lightCelestialBody) {
	self = lightCelestialBody{}
	others = make([]lightCelestialBody, 0)
	for name, body := range broadcast.Content {
		if name == c.data.name {
			self.sequence = body.Sequence
			if self.sequence != c.data.sequence {
				log.Printf("Warning -- sequence is not respected -- expected sequence number: %v -- received sequence number: %v", c.data.sequence, self.sequence)
			}
			self.name = body.Name
			self.mass = body.Mass
			self.x = body.X
			self.y = body.Y
			self.vx = body.Vx
			self.vy = body.Vy
		} else {
			others = append(
				others,
				lightCelestialBody{
					sequence: body.Sequence,
					name:     body.Name,
					mass:     body.Mass,
					x:        body.X,
					y:        body.Y,
					vx:       body.Vx,
					vy:       body.Vy,
				},
			)
		}
	}
	return
}

func (c *celestialConnection) udpateData(broadcast *pb.Data, dt float64) {
	self, others := c.parseBroadcastData(broadcast)
	c.fh.update(others)
	sigma := hamiltonVector{x: self.x, y: self.y, vx: self.vx, vy: self.vy}
	log.Printf("broadcast = %v sigma = %v", broadcast, sigma)
	sigma = c.solver.step(sigma, c.fh.evaluate, dt)
	c.data.sequence = c.data.sequence + 1
	c.data.x = sigma.x
	c.data.y = sigma.y
	c.data.vx = sigma.vx
	c.data.vy = sigma.vy
}

func (c *celestialConnection) run() error {
	if err := c.sendUpdate(); err != nil {
		c.closeConnection()
		log.Printf("Could not send initial data")
		return err
	}

	for {
		broadcast, err := c.stream.Recv()
		if err != nil {
			c.closeConnection()
			log.Printf("Failed to receive broadcast: %v", err)
			return err
		}

		log.Printf("Received broadcast update %v\n", broadcast)

		// Increment values
		c.udpateData(broadcast, 0.00001)

		// Send updated values
		err = c.sendUpdate()
		if err != nil {
			c.conn.Close()
			log.Fatalf("failed to send update: %v", err)
			return err
		}
		log.Printf("Sending update %v\n", &c.data)

		// time.Sleep(1000)

	}

}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Vector for hamiltonian calculations

type hamiltonVector struct {
	x  float64
	y  float64
	vx float64
	vy float64
}

func (sigma1 *hamiltonVector) Add(sigma2 hamiltonVector) hamiltonVector {
	return hamiltonVector{
		x:  sigma1.x + sigma2.x,
		y:  sigma1.y + sigma2.y,
		vx: sigma1.vx + sigma2.vx,
		vy: sigma1.vy + sigma2.vy,
	}
}

func (sigma1 *hamiltonVector) scalarMultiply(lambda float64) hamiltonVector {
	return hamiltonVector{
		x:  sigma1.x * lambda,
		y:  sigma1.y * lambda,
		vx: sigma1.vx * lambda,
		vy: sigma1.vy * lambda,
	}
}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Function type to pass the ODE rate function as arument of solver

type rateFunction func(hamiltonVector) hamiltonVector

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Interface to abstract solver properties

type Solver interface {
	step(sigma hamiltonVector, f rateFunction, dt float64) hamiltonVector
}

// -------------------------------------------------------------------------------------

// -------------------------------------------------------------------------------------
// Order 4 Runge-Kutta solver

type RungeKutta4Solver struct {
}

func (rks *RungeKutta4Solver) step(sigma hamiltonVector, f rateFunction, dt float64) hamiltonVector {
	F1 := f(sigma)
	F2 := f(sigma.Add(F1.scalarMultiply(dt / 2.0)))
	F3 := f(sigma.Add(F2.scalarMultiply(dt / 2.0)))
	F4 := f(sigma.Add(F3.scalarMultiply(dt)))
	sum_F := F1.Add(F2.scalarMultiply(2.0))
	sum_F = sum_F.Add(F3.scalarMultiply(2.0))
	sum_F = sum_F.Add(F4)
	return sigma.Add(sum_F.scalarMultiply(dt / 6.0))
}

type celestialRateFunctionHandler struct {
	bodies []hamiltonVector
	masses []float64
}

func (fh *celestialRateFunctionHandler) evaluate(sigmai hamiltonVector) hamiltonVector {
	rate := hamiltonVector{
		x:  sigmai.vx,
		y:  sigmai.vy,
		vx: 0.,
		vy: 0.,
	}
	for j, sigmaj := range fh.bodies {
		muj := fh.masses[j]
		d_ij := math.Sqrt((sigmaj.x-sigmai.x)*(sigmaj.x-sigmai.x) + (sigmaj.y-sigmai.y)*(sigmaj.y-sigmai.y))
		Fx := muj * (sigmaj.x - sigmai.x) / (d_ij * d_ij * d_ij)
		Fy := muj * (sigmaj.y - sigmai.y) / (d_ij * d_ij * d_ij)
		rate.vx += Fx
		rate.vy += Fy
		log.Printf("mu = %v d = %v, F = %v", muj, d_ij, math.Sqrt(Fx*Fx+Fy*Fy))
	}
	return rate
}

func (fh *celestialRateFunctionHandler) update(bodies []lightCelestialBody) {
	// Creating copies as the number of bodies may vary
	fh.bodies = make([]hamiltonVector, 0)
	fh.masses = make([]float64, 0)
	for _, body := range bodies {
		fh.bodies = append(fh.bodies, hamiltonVector{x: body.x, y: body.y, vx: body.vx, vy: body.vy})
		fh.masses = append(fh.masses, body.mass)
	}
}

// -------------------------------------------------------------------------------------

func main() {

	parser := argParser{}
	parser.parse()

	server := parser.server
	name := parser.name
	mass := parser.mass
	x := parser.x
	y := parser.y
	vx := parser.vx
	vy := parser.vy

	solver := RungeKutta4Solver{}
	fh := celestialRateFunctionHandler{}

	builder := celestialConnectionBuilder{}
	builder.set_server(server)
	builder.set_solver(&solver)
	builder.set_rateFunctionHandler(fh)
	builder.set_initial_data(lightCelestialBody{
		sequence: 1,
		name:     name,
		mass:     mass,
		x:        x,
		y:        y,
		vx:       vx,
		vy:       vy,
	})
	c, err := builder.build()
	if err != nil {
		log.Fatalf("could not build celestialConnection: %v", err)
	}
	if err := c.run(); err != nil {
		log.Fatalf("client shut down: %v", err)
	}

}
