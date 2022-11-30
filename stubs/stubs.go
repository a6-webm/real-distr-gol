package stubs

var DiTurn = "Distributor.TurnCompleted"

var BrGOL = "Broker.Process"
var BrPause = "Broker.Pause"
var BrResume = "Broker.Resume"
var BrHalt = "Broker.Halt"
var BrNumAlive = "Broker.GetNumAlive"
var BrGetState = "Broker.GetState"
var BrGetTurn = "Broker.GetTurn"

var BrEmploy = "Broker.Employ"
var WkGOL = "Worker.Process"
var WkPause = "Worker.Pause"
var WkResume = "Worker.Resume"
var WkHalt = "Worker.Halt"

type Job struct {
	GlobalStart int
	GlobalEnd   int
	Section     [][]bool
}

type DisReq struct {
	Space   [][]bool
	NumJobs int
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
}

type BrRes struct{}
