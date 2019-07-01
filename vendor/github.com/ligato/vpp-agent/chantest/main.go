package main

import (
	"os"
	"os/signal"
	"fmt"
	"time"
)

type plugin struct {
	ch  chan int
}

func (p *plugin) watcher() {
	for {
		select {
		case msg := <-p.ch:
			fmt.Println(msg)
		}
	}
}

func (p *plugin) publisher() {
	for i := 0; i < 10; i++ {
		p.ch <- i
	}
}

func main() {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt)

	p := &plugin{}

	go p.watcher()
	time.Sleep(10*time.Nanosecond)
	p.ch = make(chan int, 1000)
	go p.publisher()

	<-signalCh
}
