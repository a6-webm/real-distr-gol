package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

var broker *rpc.Client

var numThreads int

var pauseCh chan bool

var resumeCh chan bool

var haltCh chan bool

var ip string

func getOutboundIP() string {
	conn, _ := net.Dial("udp", "8.8.8.8:80")
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr).IP.String()
	return localAddr
}

func aliveCellsNearby(x int, y int, sec [][]bool) uint8 {
	var sum uint8 = 0
	if sec[y-1][(x-1+len(sec[0]))%len(sec[0])] {
		sum++
	}
	if sec[y-1][x] {
		sum++
	}
	if sec[y-1][(x+1)%len(sec[0])] {
		sum++
	}
	if sec[y][(x-1+len(sec[0]))%len(sec[0])] {
		sum++
	}
	if sec[y][(x+1)%len(sec[0])] {
		sum++
	}
	if sec[y+1][(x-1+len(sec[0]))%len(sec[0])] {
		sum++
	}
	if sec[y+1][x] {
		sum++
	}
	if sec[y+1][(x+1)%len(sec[0])] {
		sum++
	}
	return sum
}

func gameOfLife(sec [][]bool) [][]bool {
	newSec := make([][]bool, len(sec))
	for i := range newSec {
		newSec[i] = make([]bool, len(sec[i]))
	}

	for y := 1; y < len(sec)-1; y++ {
		for x := 0; x < len(sec[0]); x++ {
			surround := aliveCellsNearby(x, y, sec)
			if !sec[y][x] {
				if surround == 3 {
					newSec[y][x] = true
				} else {
					newSec[y][x] = sec[y][x]
				}
			} else if surround != 2 && surround != 3 {
				newSec[y][x] = false
			} else {
				newSec[y][x] = sec[y][x]
			}
			if possiblePauseMayExit() {
				return nil
			}
		}
	}
	return newSec
}

func possiblePauseMayExit() bool {
	select {
	case <-pauseCh:
		select {
		case <-resumeCh:
			return false
		case <-haltCh:
			return true
		}
	default:
		return false
	}
}

type Worker struct{}

func (w *Worker) Process(req stubs.BrReq, res *stubs.BrRes) error {
	sec := gameOfLife(req.Jb.Section)
	if sec == nil {
		return nil
	}
	res.Done = stubs.Result{
		GlobalStart: req.Jb.GlobalStart,
		GlobalEnd:   req.Jb.GlobalEnd,
		Section:     sec,
	}
	return nil
}

func (w *Worker) Pause(req stubs.BrReq, res *stubs.BrRes) error {
	for i := 0; i < numThreads; i++ {
		pauseCh <- true
	}
	return nil
}

func (w *Worker) Resume(req stubs.BrReq, res *stubs.BrRes) error {
	for i := 0; i < numThreads; i++ {
		resumeCh <- true
	}
	return nil
}

func (w *Worker) Halt(req stubs.BrReq, res *stubs.BrRes) error {
	for i := 0; i < numThreads+1; i++ {
		haltCh <- true
	}
	return nil
}

func (w *Worker) Layoff(req stubs.BrReq, res *stubs.BrRes) error {
	var ress *stubs.WkRes
	broker.Go(stubs.BrEmploy, stubs.WkReq{Ip: ip}, ress, nil)
	return nil
}

func main() {
	pAddr := flag.String("port", "8050", "Port to listen on")
	numTh := flag.Int("threads", 1, "Number of goroutines worker should use")
	brokerAddr := flag.String("broker", "127.0.0.1:8030", "Address of broker instance")
	flag.Parse()

	numThreads = *numTh
	pauseCh = make(chan bool, numThreads)
	resumeCh = make(chan bool, numThreads)
	haltCh = make(chan bool, numThreads+1)

	var err error
	for {
		broker, err = rpc.Dial("tcp", *brokerAddr)
		if err != nil {
			fmt.Println("WORKER::! Error connecting to broker, retrying in 5 seconds")
			time.Sleep(5 * time.Second)
		} else {
			fmt.Println("WORKER:: Connected to broker")
			break
		}
	}
	rpc.Register(&Worker{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	go rpc.Accept(listener)
	fmt.Println("WORKER:: Setup complete")
	ip = getOutboundIP() + *pAddr
	var res *stubs.WkRes
	for i := 0; i < numThreads; i++ {
		broker.Go(stubs.BrEmploy, stubs.WkReq{Ip: ip}, res, nil)
	}
	<-haltCh
}
