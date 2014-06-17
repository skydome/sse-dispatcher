// Golang HTML5 Server Side Events Example
//
// Run this code like:
//  > go run server.go
//
// Then open up your browser to http://localhost:8000
// Your browser must support HTML5 SSE, of course.

package main

import (
	"encoding/hex"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	MQTT "git.eclipse.org/gitroot/paho/org.eclipse.paho.mqtt.golang.git"
)

// A single Broker will be created in this program. It is responsible
// for keeping a list of which clients (browsers) are currently attached
// and broadcasting events (messages) to those clients.
//
type Broker struct {

	// Create a map of clients, the keys of the map are the channels
	// over which we can push messages to attached clients.  (The values
	// are just booleans and are meaningless.)
	//
	clients map[chan Sensor]bool

	// Channel into which new clients can be pushed
	//
	newClients chan chan Sensor

	// Channel into which disconnected clients should be pushed
	//
	defunctClients chan chan Sensor

	// Channel into which messages are pushed to be broadcast out
	// to attahed clients.
	//
	messages chan Sensor
}

type Sensor struct {
	Holder   string
	Time     time.Time
	ServerID byte
	ID       byte
	Prefix   byte
	Code     byte
}

// This Broker method starts a new goroutine.  It handles
// the addition & removal of clients, as well as the broadcasting
// of messages out to clients that are currently attached.
//
func (b *Broker) Start() {
	go func() {

		for {

			select {

			case s := <-b.newClients:

				b.clients[s] = true
				log.Println("Added new client")

			case s := <-b.defunctClients:

				delete(b.clients, s)
				log.Println("Removed client")

			case msg := <-b.messages:

				for s, _ := range b.clients {
					s <- msg
				}
				log.Printf("Broadcast message to %d clients", len(b.clients))
			}
		}
	}()
}

func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan Sensor)

	b.newClients <- messageChan

	defer func() {
		b.defunctClients <- messageChan
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Don't close the connection, instead loop 10 times,
	// sending messages and flushing the response each time
	// there is a new message to send along.
	//
	// NOTE: we could loop endlessly; however, then you
	// could not easily detect clients that dettach and the
	// server would continue to send them messages long after
	// they're gone due to the "keep-alive" header.  One of
	// the nifty aspects of SSE is that clients automatically
	// reconnect when they lose their connection.
	//
	// A better way to do this is to use the CloseNotifier
	// interface that will appear in future releases of
	// Go (this is written as of 1.0.3):
	// https://code.google.com/p/go/source/detail?name=3292433291b2
	//
	for i := 0; i < 10; i++ {

		msg := <-messageChan

		fmt.Fprintf(w, "data: Message: %s\n\n", msg)
		f.Flush()
	}

	log.Println("Finished HTTP request at ", r.URL.Path)
}

func MainPageHandler(w http.ResponseWriter, r *http.Request) {

	if r.URL.Path != "/" {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	t, err := template.ParseFiles("templates/index.html")
	if err != nil {
		log.Fatal("WTF dude, error parsing your template.")

	}

	t.Execute(w, "Skydome")

	log.Println("Finished HTTP request at ", r.URL.Path)
}

var b *Broker

func main() {

	b = &Broker{
		make(map[chan Sensor]bool),
		make(chan (chan Sensor)),
		make(chan (chan Sensor)),
		make(chan Sensor),
	}

	b.Start()

	http.Handle("/events/", b)


	opts := MQTT.NewClientOptions().SetBroker("tcp://api.skydome.io:1883").SetClientId("sse-dispatcher")
	opts.SetTraceLevel(MQTT.Off)
	opts.SetDefaultPublishHandler(f)

	c := MQTT.NewClient(opts)
	_, err := c.Start()
	if err != nil {
		panic(err)
	}

	filter, _ := MQTT.NewTopicFilter("skydome/#", 0)
	if receipt, err := c.StartSubscription(nil, filter); err != nil {
		fmt.Println(err)
		os.Exit(1)
	} else {
		<-receipt
	}

	http.Handle("/", http.HandlerFunc(MainPageHandler))

	http.ListenAndServe(":8080", nil)
}

var f MQTT.MessageHandler = func(client *MQTT.MqttClient, msg MQTT.Message) {
	//fmt.Printf("TOPIC: %s\n", msg.Topic())
	fmt.Println("Size of array : ", len(msg.Payload()))
	fmt.Println("MSG: ", msg.Payload())

	sensor := Sensor{hex.EncodeToString(msg.Payload()[:6]), time.Now(), msg.Payload()[6], msg.Payload()[7], msg.Payload()[8], msg.Payload()[9]}
	fmt.Println("sensor : ", sensor)
	b.messages <- sensor
}
