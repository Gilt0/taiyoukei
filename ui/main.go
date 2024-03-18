package main

import (
	"context"
	"image/color"
	"log"

	pb "taiyoukei/proto"

	"github.com/hajimehoshi/ebiten/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type lightCelestialBody struct {
	sequence uint64
	name     string
	mass     float64
	x        float64
	y        float64
	vx       float64
	vy       float64
}

func createCelestialBodyImage() *ebiten.Image {
	const size = 5 // Reduced size of the image for smaller points
	img := ebiten.NewImage(size, size)
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			dx, dy := float64(x-size/2), float64(y-size/2)
			if dx*dx+dy*dy <= float64(size/2)*(float64(size/2)) {
				img.Set(x, y, color.RGBA{0, 0, 0, 255}) // Black circle
			}
		}
	}
	return img
}

type Game struct {
	stream_update chan *pb.Data
	data          map[string][]lightCelestialBody
	bodyImage     *ebiten.Image
}

func (g *Game) Update() error {
	update := <-g.stream_update
	log.Printf("update received: %v", update)
	for name, rawdata := range update.Content {
		_, ok := g.data[name]
		if !ok {
			g.data[name] = make([]lightCelestialBody, 0)
		}
		g.data[name] = append(
			g.data[name],
			lightCelestialBody{
				sequence: rawdata.Sequence,
				name:     rawdata.Name,
				mass:     rawdata.Mass,
				x:        rawdata.X,
				y:        rawdata.Y,
				vx:       rawdata.Vx,
				vy:       rawdata.Vy,
			},
		)
		if len(g.data[name]) > 1000 {
			g.data[name] = g.data[name][len(g.data[name])-1000:]
		}
	}
	return nil
}

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(color.White)

	screenWidth, screenHeight := screen.Size()
	for _, data := range g.data {
		n := len(data)
		if n == 0 {
			continue
		}
		for i, datum := range data {
			op := &ebiten.DrawImageOptions{}
			x := (datum.x + 1) * float64(screenWidth) / 3
			y := (datum.y + 1) * float64(screenHeight) / 3

			op.GeoM.Translate(x, y)

			// Calculate opacity
			opacity := float32(i) / float32(n)
			op.ColorScale.Scale(1, 1, 1, opacity)

			screen.DrawImage(g.bodyImage, op)
		}
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 320, 240
}

func light_grpc_client(stream_update chan *pb.Data, stop chan bool) {
	conn, err := grpc.Dial(":50051", grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithBlock())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()
	client := pb.NewCelestialServiceClient(conn)

	stream, err := client.CelestialBodiesPositions(context.Background(), &pb.CelestialBodiesPositionRequest{})
	if err != nil {
		log.Fatalf("error on getting celestial bodies positions: %v", err)
	}

	for {
		update, err := stream.Recv()
		if err != nil {
			log.Printf("error on receiving update: %v", err)
			break
		}

		stream_update <- update

	}
	stop <- true
}

func main() {

	stream_update := make(chan *pb.Data)
	stop_stream := make(chan bool)
	go light_grpc_client(stream_update, stop_stream)

	ebiten.SetWindowSize(640, 480)
	ebiten.SetWindowTitle("Your Game Title")

	game := &Game{
		stream_update: stream_update,
		data:          make(map[string][]lightCelestialBody),
		bodyImage:     createCelestialBodyImage(),
	}
	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}

	<-stop_stream

}
