package main

import (
	"bytes"
	"context"
	term "github.com/nsf/termbox-go"
	"go.bug.st/serial"
	"log"
	"os"
	"time"
)

type TickerAction int

const (
	STEPUP TickerAction = iota
	STEPDOWN
	DEFAULT
)

func main() {

	port, err := serial.Open("/dev/ttyS4", &serial.Mode{})
	if err != nil {
		log.Printf("Main: Error opening port: %s", err)
		os.Exit(1)
	}
	defer port.Close()

	err = port.SetMode(&serial.Mode{
		BaudRate: 115200,
		Parity:   serial.EvenParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	})
	if err != nil {
		log.Printf("Main: Error setting port mode: %s", err)
		os.Exit(1)
	}

	log.Println("Main: Serial port ready")

	ticks := make(chan struct{}, 5)
	ticker_actions := make(chan TickerAction, 5)
	message_forward := make(chan []byte, 5)
	done := make(chan struct{}, 5)

	ctx := context.Background()
	ctx = context.WithValue(ctx, "ticker_default_interval_ms", 1000)
	ctx = context.WithValue(ctx, "ticker_interval_step_ms", 50)
	ctx = context.WithValue(ctx, "ticker_min_interval_ms", 100)
	ctx, stopAll := context.WithCancel(ctx)

	go readerActor(port, message_forward, done, ctx)
	go parserActor(message_forward, done, ctx)
	go writerActor(port, done, ticks, ctx)
	go tickerActor(ticker_actions, done, ticks, ctx)
	coroutine_count := 4

	for {
		switch e := term.PollEvent(); e.Type {
		case term.EventKey:
			switch e.Key {
			case term.KeyArrowUp:
				ticker_actions <- STEPUP
				term.Sync()
			case term.KeyArrowDown:
				ticker_actions <- STEPDOWN
				term.Sync()
      case term.KeyCtrlP:
        ticker_actions <- DEFAULT
        term.Sync()
      case term.KeyCtrlX:
        term.Sync()
        break
			}
		case term.EventInterrupt:
      log.Println("Main: user interrupt")
			break
    case term.EventError:
      log.Printf("Main: terminal event error: %s\n", e.Err)
      break
		}
		break
	}

	stopAll()

	for i := 0; i < coroutine_count; i++ {
		<-done
	}

  log.Printf("Main: all coroutines stopped successfully")
}

// Reads from serial hw buffer until it encounter a message delimeter byte, then forwards the message byte slice to the next actor in the pipeline
func readerActor(port serial.Port, message_forward chan []byte, done chan struct{}, ctx context.Context) {

	defer func() {
		log.Println("Reader: Stopped successfully")
		done <- struct{}{}
	}()

	buf := new(bytes.Buffer)

	for {
		select {
		case <-ctx.Done():
			return
		default:

			recieved_bytes := make([]byte, 1024)

			bytes_read, err := port.Read(recieved_bytes)
			if err != nil {
				log.Printf("Reader: Error while reading serial port data: %s", err)
				continue
			}

			for _, recieved_byte := range recieved_bytes[0:bytes_read] {

				if recieved_byte == '\n' {
					message := make([]byte, buf.Len())
					buf.Read(message)
					message_forward <- message
					continue
				}

				buf.WriteByte(recieved_byte)
			}
		}
	}
}

// recieves byte slice messages
func parserActor(message_forward chan []byte, done chan struct{}, ctx context.Context) {

	defer func() {
		log.Println("Parser: Stopped successfully")
		done <- struct{}{}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case message := <-message_forward:
			// TODO: count the messages, check that all the messages arrived successfully
			log.Printf("Parser: Recieved '%s'\n", string(message))
		}
	}
}

// emits dynamically timed ticks, recieves commands modifying the tick interval
func tickerActor(ticker_actions chan TickerAction, done chan struct{}, ticks chan struct{}, ctx context.Context) {

	defer func() {
		if r := recover(); r != nil {
			log.Printf("Ticker: panic with reason %s\n", r)
		} else {
			log.Println("Ticker: Stopped successfully")
		}
		done <- struct{}{}
	}()

	default_interval := ctx.Value("ticker_default_interval_ms").(int)
	interval := default_interval
	step := ctx.Value("ticker_interval_step_ms").(int)
	min_interval := ctx.Value("ticker_min_interval_ms").(int)

	ticker := time.NewTicker(time.Duration(interval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case action := <-ticker_actions:

			ticker.Stop()

			switch action {
			case STEPUP:
				interval += step
			case STEPDOWN:
				interval -= step
				if interval < min_interval {
					interval = min_interval
				}
			case DEFAULT:
				interval = default_interval
			}

			ticker = time.NewTicker(time.Millisecond * time.Duration(interval))

			log.Printf("Ticker: interval set to %d ms\n", interval)

		case <-ticker.C:
			ticks <- struct{}{}
		}
	}

}

// writes to serial on tick
func writerActor(port serial.Port, done chan struct{}, ticks chan struct{}, ctx context.Context) {

	defer func() {
		log.Println("Writer: Stopped successfully")
		done <- struct{}{}
	}()

	// TODO: send json messages with pid values to test for lost messages
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			port.Write([]byte("hello world!\n"))
		}
	}
}
