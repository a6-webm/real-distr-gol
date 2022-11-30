package main

import (
	"flag"
	"net"
	"net/rpc"

	"uk.ac.bris.cs/gameoflife/stubs"
)

type Broker struct{}

func (b *Broker) Process(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) Pause(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) Resume(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) Halt(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) GetNumAlive(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) GetState(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) GetTurn(req stubs.DisReq, res stubs.DisRes) {

}

func (b *Broker) Employ(req stubs.WkReq, res stubs.WkRes) {

}

func main() {
	pAddr := flag.String("port", "8060", "Port to listen on")
	flag.Parse()
	rpc.Register(&Broker{})
	listener, _ := net.Listen("tcp", ":"+*pAddr)
	defer listener.Close()
	rpc.Accept(listener)
}
