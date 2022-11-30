package gol

import (
	"fmt"
	"net"
	"net/rpc"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

func loadImage(p Params, c distributorChannels) [][]bool {
	c.ioCommand <- ioInput
	c.ioFilename <- fmt.Sprint(p.ImageWidth, "x", p.ImageHeight)
	space := make([][]bool, p.ImageHeight)
	for i := range space {
		space[i] = make([]bool, p.ImageWidth)
	}

	for y := 0; y < p.ImageHeight; y++ {
		for x := 0; x < p.ImageWidth; x++ {
			p := <-c.ioInput
			if p != 0 {
				space[y][x] = true
			} else {
				space[y][x] = false
			}
		}
	}
	return space
}

func sendStateToOutput(p Params, c distributorChannels, space [][]bool, turn int) {
	c.ioCommand <- ioOutput
	c.ioFilename <- fmt.Sprint(p.ImageWidth, "x", p.ImageHeight, "x", turn)
	for _, r := range space {
		for _, cell := range r {
			if cell {
				c.ioOutput <- 255
			} else {
				c.ioOutput <- 0
			}
		}
	}
}

func toListOfAlive(space [][]bool) []util.Cell {
	var alive []util.Cell
	for y, r := range space {
		for x, c := range r {
			if c {
				alive = append(alive, util.Cell{X: x, Y: y})
			}
		}
	}
	return alive
}

func mainLoop(p Params, c distributorChannels, broker *rpc.Client, initialSpace [][]bool) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	res := new(stubs.DisRes)
	paused := false
	var finalTurnCh chan *rpc.Call
	broker.Go(stubs.BrGOL, stubs.DisReq{Space: initialSpace}, res, finalTurnCh)
	for {
		select {
		case key := <-c.keyPresses:
			if key == 's' {
				broker.Call(stubs.BrGetState, stubs.DisReq{}, res)
				sendStateToOutput(p, c, res.Space, res.Turn)
			}
			if key == 'q' {
				broker.Call(stubs.BrGetState, stubs.DisReq{}, res)
				sendStateToOutput(p, c, res.Space, res.Turn)
				return
			}
			if key == 'k' {
				broker.Call(stubs.BrGetState, stubs.DisReq{}, res)
				sendStateToOutput(p, c, res.Space, res.Turn)
				broker.Call(stubs.BrHalt, stubs.DisReq{}, res)
				return
			}
			if key == 'p' {
				if paused {
					broker.Call(stubs.BrGetTurn, stubs.DisReq{}, res)
					broker.Call(stubs.BrPause, stubs.DisReq{}, res)
					c.events <- StateChange{
						res.Turn, Paused,
					}
					fmt.Println(res.Turn)
				} else {
					broker.Call(stubs.BrGetTurn, stubs.DisReq{}, res)
					broker.Call(stubs.BrResume, stubs.DisReq{}, res)
					c.events <- StateChange{
						res.Turn, Executing,
					}
					fmt.Println("Continuing")
				}
				paused = !paused
			}
		case <-ticker.C:
			broker.Call(stubs.BrNumAlive, stubs.DisReq{}, res)
			c.events <- AliveCellsCount{
				CompletedTurns: res.Turn,
				CellsCount:     res.IntRes,
			}
		case <-finalTurnCh:
			broker.Call(stubs.BrGetState, stubs.DisReq{}, res)
			sendStateToOutput(p, c, res.Space, res.Turn)
			c.events <- FinalTurnComplete{CompletedTurns: p.Turns, Alive: toListOfAlive(res.Space)}
			return
		}
	}
}

type Distributor struct {
	c distributorChannels // TODO if this doesn't work, try passing c to Broker.Process in the req
}

func (d *Distributor) TurnCompleted(req stubs.BrReq, res *stubs.BrRes) error {
	d.c.events <- TurnComplete{CompletedTurns: req.Turn + 1}
	return nil
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {
	var broker *rpc.Client
	var err error
	for {
		broker, err = rpc.Dial("tcp", p.BrokerAddr)
		if err != nil {
			fmt.Println("DIST::! Error connecting to broker, retrying in 5 seconds")
			time.Sleep(5 * time.Second)
		} else {
			defer broker.Close()
			break
		}
	}
	listener, _ := net.Listen("tcp", ":"+p.RPCPort)
	defer listener.Close()
	rpc.Register(&Distributor{c: c})
	go rpc.Accept(listener)

	turn := 0
	space := loadImage(p, c)
	for y, r := range space {
		for x, cell := range r {
			if cell {
				c.events <- CellFlipped{
					CompletedTurns: 0,
					Cell:           util.Cell{X: x, Y: y},
				}
			}
		}
	}
	fmt.Println("DIST:: Loaded ", p.ImageWidth, "x", p.ImageHeight, ".pgm")

	mainLoop(p, c, broker, space)

	// Make sure that the Io has finished any output before exiting.
	c.ioCommand <- ioCheckIdle
	<-c.ioIdle

	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
