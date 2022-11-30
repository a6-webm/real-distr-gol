package main

import (
	"flag"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct{}

var jobQueue []stubs.Job

var employeeQueue chan string

var space [][]bool
var spacemx sync.RWMutex

var turn int
var turnmx sync.RWMutex

var haltCh chan bool
var pauseCh chan bool
var resumeCh chan bool

func countAlive(space [][]bool) int {
	count := 0
	for _, r := range space {
		for _, cell := range r {
			if cell {
				count++
			}
		}
	}
	return count
}

func min(x int, y int) int {
	if x < y {
		return x
	}
	return y
}

func jobsFromSpace(space [][]bool, numJobs int) []stubs.Job {
	//Split matrix into overlapping segments
	height := len(space)
	numJobs = min(numJobs, height) // cannot use more than one thread per row
	split := height / numJobs
	var jobs []stubs.Job
	for t := 0; t < numJobs-1; t++ {
		y := t * split
		job := stubs.Job{Section: space[y : y+split+1]}                            // Section is now some number of rows plus 1 for overlapping
		job.Section = append([][]bool{space[(y-1+height)%height]}, job.Section...) // Add previous row for overlapping
		job.GlobalStart = y
		job.GlobalEnd = y + split
		jobs = append(jobs, job)
	}
	y := (numJobs - 1) * split
	job := stubs.Job{Section: space[y:]} // Create final job
	job.Section = append(job.Section, space[0])
	job.Section = append([][]bool{space[(y-1+height)%height]}, job.Section...) // Add previous row for overlapping
	jobs = append(jobs, job)
	return jobs
}

func (b *Broker) Process(req stubs.DisReq, res stubs.DisRes) error {

}

func (b *Broker) Pause(req stubs.DisReq, res stubs.DisRes) error {
	pauseCh <- true
}

func (b *Broker) Resume(req stubs.DisReq, res stubs.DisRes) error {
	resumeCh <- true
}

func (b *Broker) Halt(req stubs.DisReq, res stubs.DisRes) error {
	haltCh <- true
}

func (b *Broker) GetNumAlive(req stubs.DisReq, res stubs.DisRes) error {
	spacemx.RLock()
	res.IntRes = countAlive(space)
	spacemx.RUnlock()
	return nil
}

func (b *Broker) GetState(req stubs.DisReq, res stubs.DisRes) error {
	spacemx.RLock()
	res.Space = space
	spacemx.RUnlock()
	turnmx.RLock()
	res.Turn = turn
	turnmx.RUnlock()
	return nil
}

func (b *Broker) GetTurn(req stubs.DisReq, res stubs.DisRes) error {
	turnmx.RLock()
	res.Turn = turn
	turnmx.RUnlock()
	return nil
}

func (b *Broker) Employ(req stubs.WkReq, res stubs.WkRes) error {
	employeeQueue <- req.Ip
}

func main() {
	pAddr := flag.String("port", "8060", "Port to listen on")
	flag.Parse()
	rpc.Register(&Broker{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}
