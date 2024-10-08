package gosse

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

type GoSSE struct {
	clients    map[chan string]bool
	add        chan chan string
	remove     chan chan string
	messages   chan string
	timeout_ms time.Duration
}

func NewSSE(timeout_ms time.Duration) *GoSSE {
	sse := &GoSSE{
		make(map[chan string]bool),
		make(chan (chan string)),
		make(chan (chan string)),
		make(chan string),
		timeout_ms,
	}

	go func() {
		for {
			select {
			case c := <-sse.add:
				sse.clients[c] = true
			case c := <-sse.remove:
				delete(sse.clients, c)
				close(c)
			case msg := <-sse.messages:
				for c := range sse.clients {
					select {
					case c <- msg:
					case <-time.After(timeout_ms * time.Millisecond):
						continue
					}
				}
			}
		}
	}()

	return sse
}

func (sse *GoSSE) Publish(msg string) error {
	select {
	case sse.messages <- msg:
		return nil
	case <-time.After(sse.timeout_ms * time.Millisecond):
		return errors.New("timetou")
	}
}

func (sse *GoSSE) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	message := make(chan string)

	sse.add <- message

	go func() {
		<-r.Context().Done()
		sse.remove <- message
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	for {
		msg, open := <-message
		if !open {
			break
		}
		fmt.Fprintf(w, "data: %s\n\n", msg)
		f.Flush()
	}
}
