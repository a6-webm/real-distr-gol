package main

import (
	"flag"
	"fmt"
	"net"
	"net/rpc"
	"sync"
	"time"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct{}

var distributor *rpc.Client
var distmx sync.RWMutex

var runningWorkers int
var runQmx sync.RWMutex

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
	job.GlobalStart = y
	job.GlobalEnd = y + split
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
		fmt.Println("BROKER:: rpcClient for", ip, "already stored")
		worker = rpcC
	} else {
		fmt.Println("BROKER:: rpcClient for", ip, "unknown, dialing...")
		worker, err = rpc.Dial("tcp", ip)
		if err != nil {
			wCmx.Unlock()
			fmt.Println("BROKER::! Couldn't dial", ip)
			return
		}
		workerClients[ip] = worker
	}
	wCmx.Unlock()
	fmt.Println("BROKER:: Retrieved rpcClient")

	res := new(stubs.BrRes)
	jobQmx.RLock()
	if len(jobQueue) == 0 {
		jobQmx.RUnlock()
		fmt.Println("BROKER:: no work for employee, handle closed")
		worker.Call(stubs.WkDone, stubs.BrReq{}, res)
		return
	}
	jobQmx.RUnlock()
	runQmx.Lock()
	jobQmx.Lock()
	job := jobQueue[len(jobQueue)-1]
	jobQueue = jobQueue[:len(jobQueue)-1]
	runningWorkers++
	jobQmx.Unlock()
	runQmx.Unlock()
	fmt.Println("BROKER:: Selected job [", job.GlobalStart, ",", job.GlobalEnd, "]")

	err = worker.Call(stubs.WkGOL, stubs.BrReq{Jb: job}, res)
	if err != nil {
		fmt.Println("BROKER::! Worker errored '", err, "', removing worker and adding job back to queue")
		runQmx.Lock()
		jobQmx.Lock()
		wCmx.Lock()
		delete(workerClients, ip)
		jobQueue = append([]stubs.Job{job}, jobQueue...)
		runningWorkers--
		wCmx.Unlock()
		jobQmx.Unlock()
		runQmx.Unlock()
		return
	}
	nextSpacemx.Lock()
	nextSpace = replaceSection(nextSpace, res.Done)
	nextSpacemx.Unlock()
	worker.Call(stubs.WkDone, stubs.BrReq{}, res)
	runQmx.Lock()
	runningWorkers--
	runQmx.Unlock()
	fmt.Println("BROKER:: handleEmployee() closed")
}

func processLoop(req stubs.DisReq, res *stubs.DisRes) {
	jobQueue = jobsFromSpace(space, req.NumJobs)
	for {
		select {
		case <-pauseCh:
			fmt.Println("BROKER:: Paused")
			<-resumeCh
			fmt.Println("BROKER:: Resumed")
		case <-haltCh:
			fmt.Println("shoulda halted ig")
		case ip := <-employeeQueue:
			fmt.Println("BROKER:: Employing", ip)
			go handleEmployee(ip)
		}
		if len(jobQueue) == 0 && runningWorkers == 0 {
			fmt.Println("BROKER:: jobQueue empty, turn ended")
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

func (b *Broker) Process(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("BROKER:: Process started")
	nextSpacemx.Lock()
	spacemx.Lock()
	space = req.Space
	resetNextSpace()
	spacemx.Unlock()
	nextSpacemx.Unlock()
	ress := new(stubs.BrRes)
	turnmx.RLock()
	if turn >= req.ForTurns {
		fmt.Println("BROKER:: Last turn reached")
		spacemx.RLock()
		res.Space = space
		res.Turn = turn
		spacemx.RUnlock()
		turnmx.RUnlock()
		return nil
	}
	turnmx.RUnlock()
	for { // Turn loop
		processLoop(req, res)

		turnmx.Lock()
		if turn >= req.ForTurns {
			fmt.Println("BROKER:: Last turn reached")
			turnmx.Unlock()
			break
		}
		turn++
		temp := turn
		turnmx.Unlock()

		distmx.RLock()
		distributor.Call(stubs.DiTurn, stubs.BrReq{Turn: temp, C: req.C}, ress)
		distmx.RUnlock()
	}

	spacemx.RLock()
	turnmx.RLock()
	res.Space = space
	res.Turn = turn
	spacemx.RUnlock()
	turnmx.RUnlock()

	return nil
}

func callAllWorkers(serviceMethod string) {
	wCmx.Lock()
	done := make(chan *rpc.Call, len(workerClients))
	brRes := new(stubs.BrRes)
	for ip, rpcC := range workerClients {
		rpcC.Go(serviceMethod, stubs.BrReq{Ip: ip}, brRes, done)
	}
	for i := 0; i < len(workerClients); i++ {
		d := <-done
		if d.Error != nil {
			fmt.Print("BROKER::! Worker", d.Args.(stubs.BrReq).Ip, "no longer responsive, removing from list")
			delete(workerClients, d.Args.(stubs.BrReq).Ip)
		}
	}
	wCmx.Unlock()
}

func (b *Broker) SetDistributor(req stubs.DisReq, res *stubs.DisRes) error {
	var err error
	distmx.Lock()
	if distributor != nil {
		distributor.Close()
	}
	for {
		distributor, err = rpc.Dial("tcp", req.Ip)
		if err != nil {
			fmt.Println("BROKER::! Error connecting to distributor, retrying in 1 second")
			time.Sleep(1 * time.Second)
		} else {
			fmt.Println("BROKER:: Connected to distributor")
			break
		}
	}
	distmx.Unlock()
	return nil
}

func (b *Broker) Pause(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: Pause signal")
	pauseCh <- true
	callAllWorkers(stubs.WkPause)
	return nil
}

func (b *Broker) Resume(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: Resume signal")
	resumeCh <- true
	callAllWorkers(stubs.WkResume)
	return nil
}

func (b *Broker) Halt(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: Halt signal")
	haltCh <- true
	callAllWorkers(stubs.WkHalt)
	return nil
}

func (b *Broker) GetNumAlive(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: GetNumAlive")
	turnmx.RLock()
	spacemx.RLock()
	res.IntRes = countAlive(space)
	res.Turn = turn
	spacemx.RUnlock()
	turnmx.RUnlock()
	return nil
}

func (b *Broker) GetState(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: GetState")
	spacemx.RLock()
	turnmx.RLock()
	res.Space = space
	res.Turn = turn
	spacemx.RUnlock()
	turnmx.RUnlock()
	return nil
}

func (b *Broker) GetTurn(req stubs.DisReq, res *stubs.DisRes) error {
	fmt.Println("DIST:: GetTurn")
	turnmx.RLock()
	res.Turn = turn
	turnmx.RUnlock()
	return nil
}

func (b *Broker) Employ(req stubs.WkReq, res *stubs.WkRes) error {
	fmt.Println(req.Ip, ":: Employ request")
	employeeQueue <- req.Ip
	return nil
}

func main() {
	pAddr := flag.String("port", "8081", "Port to listen on")
	bufsize := flag.Int("bufsize", 64, "Size of message buffers, set to at least the number of threads in use")
	flag.Parse()
	workerClients = make(map[string]*rpc.Client)
	employeeQueue = make(chan string, *bufsize)
	haltCh = make(chan bool, *bufsize)
	pauseCh = make(chan bool, *bufsize)
	resumeCh = make(chan bool, *bufsize)
	rpc.Register(&Broker{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	fmt.Println("BROKER:: Setup complete")
	rpc.Accept(listener)
}
