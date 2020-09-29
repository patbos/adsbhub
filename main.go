package main

import (
	"bufio"
	"flag"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var (
	messagesReceived = promauto.NewCounter(prometheus.CounterOpts{
		Name: "adsb_received_messages",
		Help: "The total number of received ADS-B messages",
	})
	messagesSent = promauto.NewCounter(prometheus.CounterOpts{
		Name: "adsb_sent_messages",
		Help: "The total number of sent ADS-B messages",
	})
)

func sender(host string, port int, messages chan string) {
	for {
		var address = host + ":" + strconv.Itoa(port)
		log.Printf("Trying to connect sender to %s", address)
		conn, err := net.Dial("tcp", address)
		writer := bufio.NewWriter(conn)
		if err == nil {
			log.Printf("Connected sender to %s", address)
			for {
				message := <-messages
				_, writeErr := writer.WriteString(message + "\n")
				flushErr := writer.Flush()
				if writeErr == nil && flushErr == nil {
					messagesSent.Inc()
				} else {
					messages <- message
					log.Printf("Could not send message! %v %v", writeErr, flushErr)
					break
				}
			}
			closeErr := conn.Close()
			if closeErr != nil {
				log.Printf("Could not cole connection %v", closeErr)
			}
		} else {
			log.Printf("Could not connect sender to %s", address)
		}
		time.Sleep(10 * time.Second)
	}
}

func receiver(host string, port int, messages chan string) {
	for {
		var address = host + ":" + strconv.Itoa(port)
		log.Printf("Trying to connect receiver to %s", address)
		conn, err := net.Dial("tcp", address)
		if err == nil {
			log.Printf("Connected receiver to %s", address)
			reader := bufio.NewReader(conn)
			for {
				receivedMessage, readErr := reader.ReadString('\n')
				if readErr == nil {
					message := strings.TrimSpace(receivedMessage)
					if message != "" {
						messages <- message
						messagesReceived.Inc()
					} else {
						log.Print("Empty message received")
					}
				} else {
					break
				}
			}
			closeErr := conn.Close()
			if closeErr != nil {
				log.Printf("Could not cole connection %v", closeErr)
			}
		} else {
			log.Printf("Connection failed to receiver %s", address)
		}
		time.Sleep(10 * time.Second)
	}
}

func prom() {
	http.Handle("/metrics", promhttp.Handler())
	err := http.ListenAndServe(":9180", nil)
	if err != nil {
		log.Printf("Could not start metric endpoint %v", err)
	}
}

func main() {
	sourceHost := flag.String("source-host", "localhost", "Receiver hostname")
	sourcePort := flag.Int("source-port", 30003, "Receiver port")
	destHost := flag.String("dest-host", "data.adsbhub.org", "Destination hostname")
	destPort := flag.Int("dest-port", 5001, "Destination port")

	flag.Parse()

	var messages = make(chan string, 100)
	go sender(*destHost, *destPort, messages)
	go prom()
	receiver(*sourceHost, *sourcePort, messages)

}
