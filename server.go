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

func echo(w http.ResponseWriter, r *http.Request) {
	kill := make(chan bool)

	// WebSocket setup
	c, err := upgrader.Upgrade(w, r, nil)
	check(err)
	defer c.Close()

	// MIDI setup
	midiStream, err := portmidi.NewInputStream(portmidi.DefaultInputDeviceID(), 1024)
	check(err)

	go func(conn *websocket.Conn) {
		for {
			mt, bytes, err := conn.ReadMessage()
			if err != nil {
				fmt.Printf("recv: error: %s\n", err)
				midiStream.Close()
				kill <- true
				return
			}
			fmt.Printf("recv: mt=%d, bytes=%v\n", mt, bytes)
		}
	}(c)

	events := midiStream.Listen()
	for {
		select {
		case event := <-events:
			serialized, err := json.Marshal(event)
			check(err)
			fmt.Printf("midi: %v\n", event)
			err = c.WriteMessage(1, serialized)
			if err != nil {
				fmt.Errorf("write", err)
				break
			}
		case <-kill:
			fmt.Printf("midi: message received on kill chan\n")
			break
		}
	}

	fmt.Println("EXIT")
}

func home(w http.ResponseWriter, r *http.Request) {
	contents, err := ioutil.ReadFile("main.html")
	check(err)
	homeTemplate := template.Must(template.New("").Parse(string(contents)))
	homeTemplate.Execute(w, "ws://"+r.Host+"/echo")
}

func main() {
	flag.Parse()
	log.SetFlags(0)

	portmidi.Initialize()
	fmt.Printf("Devices: %d (default=%d)\n", portmidi.CountDevices(), portmidi.DefaultInputDeviceID())
	defer portmidi.Terminate()

	http.HandleFunc("/echo", echo)
	http.HandleFunc("/", home)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

var homeTemplate = template.Must(template.New("").Parse(`
<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<script>
window.addEventListener("load", function(evt) {

    var output = document.getElementById("output");
    var input = document.getElementById("input");
    var ws;

    document.getElementById("open").onclick = function(evt) {
        if (ws) {
            return false;
        }
        ws = new WebSocket("{{.}}");
        ws.onopen = function(evt) {
					console.log("OPEN");
        }
        ws.onclose = function(evt) {
					console.log("CLOSE");
					ws = null;
        }
        ws.onmessage = function(evt) {
          console.log("RESPONSE: " + evt.data);
        }
        ws.onerror = function(evt) {
          console.log("ERROR: " + evt.data);
        }
        return false;
    };

    document.getElementById("send").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        console.log("SEND: " + input.value);
        ws.send(input.value);
        return false;
    };

    document.getElementById("close").onclick = function(evt) {
        if (!ws) {
            return false;
        }
        ws.close();
        return false;
    };

		var key_width = 25
		var c = document.getElementById("myCanvas");
		var ctx = c.getContext("2d");

		ctx.fillStyle = 'rgb(200, 0, 0)';
		ctx.strokeRect(1,1,25,25);
		for (i = 0; i < c.width; i += key_width) {
			console.log(i,0)
			//ctx.strokeRect(i, 0, key_width-1, 100-1);
		}
});
</script>
</head>
<body>
	<form>
		<button id="open">Open</button>
		<button id="close">Close</button>
		<input id="input" type="text" value="Hello world!">
		<button id="send">Send</button>
	</form>
	<canvas id="myCanvas" style="width: 100%; height: 100px; border:1px solid #000000;"></canvas>
</body>
</html>
`))
