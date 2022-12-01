package stubs

var DiTurn = "Distributor.TurnCompleted"

var BrGOL = "Broker.Process"
var BrPause = "Broker.Pause"
var BrResume = "Broker.Resume"
var BrHalt = "Broker.Halt"
var BrNumAlive = "Broker.GetNumAlive"
var BrGetState = "Broker.GetState"
var BrGetTurn = "Broker.GetTurn"
var BrSetDist = "Broker.SetDistributor"

var BrEmploy = "Broker.Employ"
var WkGOL = "Worker.Process"
var WkPause = "Worker.Pause"
var WkResume = "Worker.Resume"
var WkHalt = "Worker.Halt"
var WkDone = "Worker.Layoff"

type Job struct {
	GlobalStart int
	GlobalEnd   int
	Section     [][]bool
}

type Result struct {
	GlobalStart int
	GlobalEnd   int
	Section     [][]bool
}

type DisReq struct {
	Space    [][]bool
	NumJobs  int
	ForTurns int
	Ip       string
	C        any
}

type DisRes struct {
	IntRes int
	Space  [][]bool
	Turn   int
}

type WkReq struct {
	Ip string
}

type WkRes struct{}

type BrReq struct {
	Turn int
	Jb   Job
	Ip   string
	C    any
}

type BrRes struct {
	Done Result
}
