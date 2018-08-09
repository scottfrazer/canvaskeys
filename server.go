package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/rakyll/portmidi"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}

var addr = flag.String("addr", "localhost:8080", "http service address")

var upgrader = websocket.Upgrader{} // use default options

var listeners map[*http.Request]chan portmidi.Event = make(map[*http.Request]chan portmidi.Event)

func subscribe(r *http.Request) chan portmidi.Event {
	if channel, ok := listeners[r]; ok {
		return channel
	}
	channel := make(chan portmidi.Event, 10000)
	listeners[r] = channel
	return channel
}

func unsubscribe(r *http.Request) {
	if _, ok := listeners[r]; ok {
		delete(listeners, r)
	}
}

func publish(event portmidi.Event) {
	fmt.Printf("midi: %v\n", event)
	for _, channel := range listeners {
		channel <- event
	}
}

func midi(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("WebSocket connection from %s\n", r.RemoteAddr)
	kill := make(chan bool)

	events := subscribe(r)
	defer unsubscribe(r)

	// WebSocket setup
	c, err := upgrader.Upgrade(w, r, nil)
	check(err)
	defer c.Close()

	go func(conn *websocket.Conn) {
		for {
			mt, bytes, err := conn.ReadMessage()
			if err != nil {
				fmt.Printf("recv: error: %s\n", err)
				kill <- true
				return
			}
			fmt.Printf("recv: mt=%d, bytes=%v\n", mt, bytes)
		}
	}(c)

EVENTLOOP:
	for {
		select {
		case event := <-events:
			serialized, err := json.Marshal(event)
			check(err)
			err = c.WriteMessage(1, serialized)
			if err != nil {
				fmt.Errorf("write", err)
				break EVENTLOOP
			}
		case <-kill:
			fmt.Printf("midi: message received on kill chan\n")
			break EVENTLOOP
		}
	}

	fmt.Printf("WebSocket connection from %s: EXIT\n", r.RemoteAddr)
}

func home(w http.ResponseWriter, r *http.Request) {
	contents, err := ioutil.ReadFile("main.html")
	check(err)
	homeTemplate := template.Must(template.New("").Parse(string(contents)))
	homeTemplate.Execute(w, "ws://"+r.Host+"/midi")
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	fmt.Printf("Initializing MIDI... ")
	portmidi.Initialize()
	fmt.Printf("DONE (Devices: %d; default=%d)\n", portmidi.CountDevices(), portmidi.DefaultInputDeviceID())
	defer portmidi.Terminate()

	midiStream, err := portmidi.NewInputStream(portmidi.DefaultInputDeviceID(), 1024)
	check(err)
	defer midiStream.Close()
	events := midiStream.Listen()

	go func(events <-chan portmidi.Event) {
		for {
			select {
			case event := <-events:
				publish(event)
			}
		}
	}(events)

	http.HandleFunc("/midi", midi)
	http.HandleFunc("/", home)

	fmt.Printf("Listening on %s...\n", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

var homeTemplate = template.Must(template.New("").Parse(``))
