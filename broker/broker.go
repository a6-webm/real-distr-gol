package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct{}

var jobQueue []stubs.Job
var jobQmx sync.RWMutex

var employeeQueue chan string

var workerClients map[string]*rpc.Client
var wCmx sync.RWMutex

var nextSpace [][]bool
var nextSpacemx sync.RWMutex

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

func replaceSection(in [][]bool, with stubs.Result) [][]bool {
	return append(append(in[:with.GlobalStart], with.Section[1:len(with.Section)-1]...), in[with.GlobalEnd:]...)
}

func resetNextSpace() {
	nextSpace = make([][]bool, len(space))
	for i := 0; i < len(nextSpace); i++ {
		nextSpace[i] = make([]bool, len(space[0]))
	}
	nextSpace = make([][]bool, len(space))
}

func handleEmployee(ip string) {
	var err error
	var worker *rpc.Client
	wCmx.Lock()
	if rpcC, ok := workerClients[ip]; ok {
		worker = rpcC
	} else {
		worker, err = rpc.Dial("tcp", ip)
		if err != nil {
			wCmx.Unlock()
			return
		}
		workerClients[ip] = worker
	}
	wCmx.Unlock()

	jobQmx.Lock()
	job := jobQueue[len(jobQueue)-1]
	jobQueue = jobQueue[:len(jobQueue)-1]
	jobQmx.Unlock()

	res := new(stubs.BrRes)
	err = worker.Call(stubs.WkGOL, stubs.BrReq{Jb: job}, res)
	if err != nil {
		jobQmx.Lock()
		jobQueue = append([]stubs.Job{job}, jobQueue...)
		jobQmx.Unlock()
		return
	}
	nextSpacemx.Lock()
	nextSpace = replaceSection(nextSpace, res.Done)
	nextSpacemx.Unlock()
	worker.Call(stubs.WkDone, stubs.BrReq{}, res)
}

func processLoop(req stubs.DisReq, res *stubs.DisRes) {
	for {
		jobQueue = jobsFromSpace(space, req.NumJobs)
		for {
			select {
			case <-pauseCh:
				<-resumeCh
			case <-haltCh:
				fmt.Println("shoulda halted ig")
			case ip := <-employeeQueue:
				go handleEmployee(ip)
			}
			if len(jobQueue) == 0 {
				nextSpacemx.Lock()
				spacemx.Lock()
				space = nextSpace
				resetNextSpace()
				spacemx.Unlock()
				nextSpacemx.Unlock()
				return
			}
		}
	}
}

func (b *Broker) Process(req stubs.DisReq, res *stubs.DisRes) error { // TODO come back and analyse race conditions
	nextSpacemx.Lock()
	spacemx.Lock()
	space = req.Space
	resetNextSpace()
	spacemx.Unlock()
	nextSpacemx.Unlock()
	for {
		turnmx.RLock()
		if turn >= req.ForTurns {
			turnmx.RUnlock()
			break
		}
		turnmx.RUnlock()

		processLoop(req, res)
	}

	return nil
}

func callAllWorkers(serviceMethod string) {
	wCmx.Lock()
	done := make(chan *rpc.Call, len(workerClients))
	var brRes stubs.BrRes
	for ip, rpcC := range workerClients {
		rpcC.Go(serviceMethod, stubs.BrReq{Ip: ip}, brRes, done)
	}
	for i := 0; i < len(workerClients); i++ {
		d := <-done
		if d.Error != nil {
			delete(workerClients, d.Args.(stubs.BrReq).Ip)
		}
	}
	wCmx.Unlock()
}

func (b *Broker) Pause(req stubs.DisReq, res *stubs.DisRes) error {
	pauseCh <- true
	callAllWorkers(stubs.WkPause)
	return nil
}

func (b *Broker) Resume(req stubs.DisReq, res *stubs.DisRes) error {
	resumeCh <- true
	callAllWorkers(stubs.WkResume)
	return nil
}

func (b *Broker) Halt(req stubs.DisReq, res *stubs.DisRes) error {
	haltCh <- true
	callAllWorkers(stubs.WkHalt)
	return nil
}

func (b *Broker) GetNumAlive(req stubs.DisReq, res *stubs.DisRes) error {
	spacemx.RLock()
	res.IntRes = countAlive(space)
	spacemx.RUnlock()
	return nil
}

func (b *Broker) GetState(req stubs.DisReq, res *stubs.DisRes) error {
	spacemx.RLock()
	res.Space = space
	spacemx.RUnlock()
	turnmx.RLock()
	res.Turn = turn
	turnmx.RUnlock()
	return nil
}

func (b *Broker) GetTurn(req stubs.DisReq, res *stubs.DisRes) error {
	turnmx.RLock()
	res.Turn = turn
	turnmx.RUnlock()
	return nil
}

func (b *Broker) Employ(req stubs.WkReq, res *stubs.WkRes) error {
	employeeQueue <- req.Ip
	return nil
}

func main() {
	pAddr := flag.String("port", "8081", "Port to listen on")
	flag.Parse()
	rpc.Register(&Broker{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}
