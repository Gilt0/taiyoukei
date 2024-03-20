package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/signal"
	"syscall"

	pb "taiyoukei/proto"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/vg"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const DATA_DIR = "data"
const PLOT_FREQUENCY = 1000
const CENTER = "E"
const BODY = "M"

type Position struct {
	X float64
	Y float64
}

func saveToFile(name string, datum *pb.CelestialBody) {
	fileName := fmt.Sprintf("%s/%s_data.txt", DATA_DIR, name)
	file, err := os.OpenFile(fileName, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	_, err = file.WriteString(fmt.Sprintf("%v\n", datum))
	if err != nil {
		log.Fatalf("error writing to file: %v", err)
	}
	file.Close()
}

func plotData(positionData map[string][]Position) {
	p := plot.New()
	p.Title.Text = "Celestial Body Positions"
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y"

	colorIndex := 0
	colors := []color.Color{
		color.RGBA{R: 89, G: 114, B: 191, A: 255},
		color.RGBA{R: 89, G: 191, B: 114, A: 255},
		color.RGBA{R: 191, G: 114, B: 89, A: 255},
	}

	for name, positions := range positionData {
		pts := make(plotter.XYs, len(positions))
		for i, pos := range positions {
			pts[i].X = pos.X
			pts[i].Y = pos.Y
		}

		line, err := plotter.NewLine(pts)
		if err != nil {
			log.Fatal(err)
		}
		line.LineStyle.Width = vg.Points(2)
		line.Color = colors[colorIndex%len(colors)]
		colorIndex++

		p.Add(line)
		p.Legend.Add(name, line)
	}

	if err := p.Save(10*vg.Inch, 10*vg.Inch, "data/celestial_positions.png"); err != nil {
		log.Fatal(err)
	}
}

func plotDataCenteredOn(positionData map[string][]Position, center string, body string) {
	p := plot.New()
	p.Title.Text = fmt.Sprintf("Relative position of %s to %s", body, center)
	p.X.Label.Text = "X"
	p.Y.Label.Text = "Y"

	colorIndex := 0
	colors := []color.Color{
		color.RGBA{R: 89, G: 114, B: 191, A: 255},
		color.RGBA{R: 89, G: 191, B: 114, A: 255},
		color.RGBA{R: 191, G: 114, B: 89, A: 255},
	}

	bodyPosition, bodyExists := positionData[body]
	centerPosition, centerExists := positionData[center]

	if !bodyExists || !centerExists {
		log.Printf("Earth or Moon data not found")
		return
	}

	// Ensure there is data to plot
	if len(bodyPosition) == 0 || len(centerPosition) == 0 {
		log.Printf("No data available for Earth or Moon")
		return
	}

	pts := make(plotter.XYs, len(bodyPosition))
	for i, pos := range bodyPosition {
		pts[i].X = pos.X - centerPosition[i].X
		pts[i].Y = pos.Y - centerPosition[i].Y
	}

	line, err := plotter.NewLine(pts)
	if err != nil {
		log.Fatal(err)
	}
	line.LineStyle.Width = vg.Points(2)
	line.Color = colors[colorIndex%len(colors)]
	colorIndex++

	p.Add(line)
	p.Legend.Add(body, line)

	if err := p.Save(10*vg.Inch, 10*vg.Inch, fmt.Sprintf("data/relative_positions_%s_to_%s.png", body, center)); err != nil {
		log.Fatal(err)
	}
}

func light_grpc_client() {
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

	dataDir := "data"
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		log.Fatalf("error creating data directory: %v", err)
	}

	positionData := make(map[string][]Position)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	counter := 1

outer_loop:
	for {
		select {
		case <-c:
			plotData(positionData)
			break outer_loop
		default:
			update, err := stream.Recv()
			if err != nil {
				log.Printf("error on receiving update: %v", err)
				break
			}
			for name, datum := range update.Content {
				saveToFile(name, datum)
				positionData[name] = append(positionData[name], Position{X: datum.X, Y: datum.Y})
				log.Printf("name = %v datum = %v", name, datum)
			}
			if counter%PLOT_FREQUENCY == 0 {
				plotData(positionData)
				plotDataCenteredOn(positionData, CENTER, BODY)
			}
			counter++
		}
	}
}

func main() {
	light_grpc_client()
}
